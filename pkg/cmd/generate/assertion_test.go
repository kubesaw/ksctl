package generate

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	commonidentity "github.com/codeready-toolchain/toolchain-common/pkg/identity"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/kustomize/api/types"

	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/utils"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// storage assertions

type storageAssertion interface {
	assertObject(namespace, name string, object runtimeclient.Object, contentAssertions ...func())
	listObjects(dirName, kind string, object runtimeclient.Object) ([]runtimeclient.Object, error)
}

type storageAssertionImpl struct {
	storageAssertion
	out         string
	rootDirName string
	t           *testing.T
}

// cache based structure related assertions

type objectsCacheAssertion struct {
	*storageAssertionImpl
	cache objectsCache
}

func inObjectCache(t *testing.T, out, rootDirName string, cache objectsCache) *objectsCacheAssertion {
	cacheAssertion := &objectsCacheAssertion{
		storageAssertionImpl: &storageAssertionImpl{
			out:         out,
			t:           t,
			rootDirName: rootDirName,
		},
		cache: cache,
	}
	cacheAssertion.storageAssertion = cacheAssertion
	return cacheAssertion
}

func (a *objectsCacheAssertion) assertObjectDoesNotExist(namespace, name string, object runtimeclient.Object) *objectsCacheAssertion { //nolint unparam
	gvks, _, err := scheme.Scheme.ObjectKinds(object)
	require.NoError(a.t, err)
	require.Len(a.t, gvks, 1)
	plural, _ := meta.UnsafeGuessKindToResource(gvks[0])
	filePath := getFilePath(a.out, a.rootDirName, namespace, plural.Resource, name)
	assert.Nil(a.t, a.cache[filePath])
	return a
}

func (a *objectsCacheAssertion) listObjects(dirName, kind string, _ runtimeclient.Object) ([]runtimeclient.Object, error) {
	var objects []runtimeclient.Object
	prefix := filepath.Join(a.out, a.rootDirName)
	for path, obj := range a.cache {
		if strings.HasPrefix(path, prefix) && filepath.Base(filepath.Dir(path)) == dirName && obj.GetObjectKind().GroupVersionKind().Kind == kind {
			object := obj
			objects = append(objects, object)
		}
	}
	return objects, nil
}

func (a *objectsCacheAssertion) assertObject(namespace, name string, object runtimeclient.Object, contentAssertions ...func()) {
	gvks, _, err := scheme.Scheme.ObjectKinds(object)
	require.NoError(a.t, err)
	require.Len(a.t, gvks, 1)
	plural, _ := meta.UnsafeGuessKindToResource(gvks[0])
	filePath := getFilePath(a.out, a.rootDirName, namespace, plural.Resource, name)
	require.Contains(a.t, a.cache, filePath)
	obj := a.cache[filePath]
	bytes, err := json.Marshal(obj)
	require.NoError(a.t, err)
	err = json.Unmarshal(bytes, object)
	require.NoError(a.t, err)
	genericObjectAssertion(a.t, name, namespace, gvks[0], object)
	for _, assertContent := range contentAssertions {
		assertContent()
	}
}

func genericObjectAssertion(t *testing.T, name, namespace string, gvk schema.GroupVersionKind, object runtimeclient.Object) {
	assert.Equal(t, name, object.GetName())
	assert.Equal(t, namespace, object.GetNamespace())
	assert.Equal(t, gvk, object.GetObjectKind().GroupVersionKind())
}

// file based and kustomize structure related assertions

func assertNoClusterTypeEntry(t *testing.T, out string, clusterType configuration.ClusterType) {
	_, err := os.Stat(filepath.Join(out, clusterType.String()))
	require.True(t, os.IsNotExist(err))
}

type kStructureAssertion struct {
	*storageAssertionImpl
}

func inKStructure(t *testing.T, out, rootDirName string) *kStructureAssertion {
	kAssertion := &kStructureAssertion{
		storageAssertionImpl: &storageAssertionImpl{
			out:         out,
			t:           t,
			rootDirName: rootDirName,
		},
	}
	kAssertion.storageAssertion = kAssertion
	return kAssertion
}

func (a *kStructureAssertion) listObjects(dirName, kind string, object runtimeclient.Object) ([]runtimeclient.Object, error) {
	var objects []runtimeclient.Object

	err := filepath.WalkDir(filepath.Join(a.out, a.rootDirName), func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry != nil && !dirEntry.IsDir() && (filepath.Base(filepath.Dir(path)) == dirName || dirName == "") && dirEntry.Name() != "kustomization.yaml" {
			obj := object.DeepCopyObject()
			objFile, err := os.ReadFile(path)
			require.NoError(a.t, err)
			err = yaml.Unmarshal(objFile, obj)
			require.NoError(a.t, err)
			if obj.GetObjectKind().GroupVersionKind().Kind == kind {
				objects = append(objects, obj.(runtimeclient.Object))
				assertKustomizationFiles(a.t, a.out, a.rootDirName, path)
			}
		}
		return nil
	})
	return objects, err
}

func (a *kStructureAssertion) assertObject(namespace, name string, object runtimeclient.Object, contentAssertions ...func()) {
	gvks, _, err := scheme.Scheme.ObjectKinds(object)
	require.NoError(a.t, err)
	require.Len(a.t, gvks, 1)
	plural, _ := meta.UnsafeGuessKindToResource(gvks[0])
	filePath := getFilePath(a.out, a.rootDirName, namespace, plural.Resource, name)
	assertObjectAsFile(a.t, filePath, namespace, name, object, contentAssertions...)
	assertKustomizationFiles(a.t, a.out, a.rootDirName, filePath)
}

func assertObjectAsFile(t *testing.T, filePath, namespace, name string, object runtimeclient.Object, contentAssertions ...func()) {
	file, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(file), header), "every file generated by cli should container a header")
	assert.NotContains(t, string(file), "creationTimestamp")
	assert.NotContains(t, string(file), "user: {}")
	err = yaml.Unmarshal(file, object)
	require.NoError(t, err)

	gvks, _, err := scheme.Scheme.ObjectKinds(object)
	require.NoError(t, err)
	require.Len(t, gvks, 1)
	genericObjectAssertion(t, name, namespace, gvks[0], object)

	for _, assertContent := range contentAssertions {
		assertContent()
	}
}

func assertKustomizationFiles(t *testing.T, out, rootDirName string, filePath string) {
	kFilePath := filepath.Join(filepath.Dir(filePath), "kustomization.yaml")
	assertKustomizationFile(t, kFilePath, filepath.Base(filePath), true)

	if filepath.Join(out, rootDirName) != filepath.Dir(filePath) {
		assertKustomizationFiles(t, out, rootDirName, filepath.Dir(kFilePath))
	} else {
		if rootDirName == "" {
			assertKustomizationFile(t, filepath.Join(out, "kustomization.yaml"), filepath.Base(filePath), true)
			return
		}
		kFilePathBase := filepath.Join(baseDirectory(out), "kustomization.yaml")
		basePresent := false
		if _, err := os.Stat(kFilePathBase); err != nil && !os.IsNotExist(err) {
			require.NoError(t, err)
		} else if err == nil {
			basePresent = true
			assertKustomizationFile(t, kFilePathBase, "../base", false)
		}
		switch rootDirName {
		case "member":
			assertKustomizationFile(t, kFilePath, "../base", basePresent)
		case "host":
			assertKustomizationFile(t, kFilePath, "../base", false)
		}
	}
}

func assertKustomizationFile(t *testing.T, kFilePath, item string, shouldBePresent bool) {
	file, err := os.ReadFile(kFilePath)
	require.NoError(t, err)

	kustomization := &types.Kustomization{}
	err = yaml.Unmarshal(file, kustomization)
	require.NoError(t, err)

	assert.Equal(t, types.KustomizationVersion, kustomization.APIVersion)
	assert.Equal(t, types.KustomizationKind, kustomization.Kind)
	if shouldBePresent {
		assert.Contains(t, kustomization.Resources, item)
	} else {
		assert.NotContains(t, kustomization.Resources, item)
	}
}

func getDirPath(out, rootDir, namespace, kind string) string {
	rootDirPath := filepath.Join(out, rootDir)
	dirPath := filepath.Join(rootDirPath, "namespace-scoped", namespace, strings.ToLower(kind))
	if namespace == "" {
		dirPath = filepath.Join(rootDirPath, "cluster-scoped", strings.ToLower(kind))
	}
	return dirPath
}
func getFilePath(out, rootDir, namespace, resource, objName string) string {
	path := filepath.Join(getDirPath(out, rootDir, namespace, resource), fmt.Sprintf("%s.yaml", objName))
	path = strings.ReplaceAll(path, ":", "-")
	return path
}

// permission assertions

type permissionAssertion struct {
	*storageAssertionImpl
	subject   rbacv1.Subject
	expLabels map[string]string
}

func newPermissionAssertion(storageAssertion *storageAssertionImpl, subjNamespace, subjName, subjKind string) permissionAssertion {
	return permissionAssertion{
		storageAssertionImpl: storageAssertion,
		subject: rbacv1.Subject{
			Kind:      subjKind,
			Name:      subjName,
			Namespace: subjNamespace,
		},
		expLabels: map[string]string{
			"provider": "sandbox-sre",
		},
	}
}

func (a *storageAssertionImpl) assertSa(namespace, name string) permissionAssertion {
	splitName := strings.Split(name, "-")

	sa := &corev1.ServiceAccount{}
	a.assertObject(namespace, name, sa, func() {
		expLabels := map[string]string{
			"provider": "sandbox-sre",
			"username": splitName[len(splitName)-1],
		}
		assert.Equal(a.t, expLabels, sa.Labels)
		assert.Equal(a.t, ignoreExtraneousAnnotations, sa.Annotations)
	})
	return newPermissionAssertion(a, namespace, name, "ServiceAccount")
}

type userAssertion struct {
	permissionAssertion
	expLabels map[string]string
	userName  string
	outDir    string
}

func (a *storageAssertionImpl) assertUser(name string) userAssertion {
	expLabels := map[string]string{
		"provider": "sandbox-sre",
		"username": name,
	}

	userObj := &userv1.User{}
	a.assertObject("", name, userObj, func() {
		assert.Equal(a.t, expLabels, userObj.Labels)
		assert.Equal(a.t, ignoreExtraneousAnnotations, userObj.Annotations)
	})

	return userAssertion{
		permissionAssertion: newPermissionAssertion(a, "", name, "User"),
		expLabels:           expLabels,
		userName:            name,
		outDir:              a.out,
	}
}

func (a userAssertion) hasIdentity(ID string) userAssertion {
	ins := commonidentity.NewIdentityNamingStandard(ID, "DevSandbox")
	src := &userv1.Identity{}
	ins.ApplyToIdentity(src)

	identity := &userv1.Identity{}
	a.assertObject("", ins.IdentityName(), identity, func() {
		assert.Equal(a.t, a.expLabels, identity.Labels)
		assert.Equal(a.t, src.ProviderName, identity.ProviderName)
		assert.Equal(a.t, src.ProviderUserName, identity.ProviderUserName)
		assert.Equal(a.t, ignoreExtraneousAnnotations, identity.Annotations)
	})

	return a
}

type groupsUserBelongsTo []string
type extraGroupsPresentInCluster []string

func groups(groups ...string) groupsUserBelongsTo {
	return groupsUserBelongsTo(groups)
}

func extraGroupsUserIsNotPartOf(groups ...string) extraGroupsPresentInCluster {
	return extraGroupsPresentInCluster(groups)
}

func (a userAssertion) belongsToGroups(groups groupsUserBelongsTo, extraGroups extraGroupsPresentInCluster) userAssertion {
	presentGroups, err := a.listObjects("groups", "Group", &userv1.Group{})
	require.NoError(a.t, err)
	allGroupsExpected := append(extraGroups, groups...)
	require.Len(a.t, presentGroups, len(allGroupsExpected))
	for _, group := range presentGroups {
		assert.Contains(a.t, allGroupsExpected, group.GetName())
	}

	for _, groupObj := range presentGroups {
		expLabels := map[string]string{
			"provider": "sandbox-sre",
		}
		assert.Equal(a.t, expLabels, groupObj.GetLabels())
		group := groupObj.(*userv1.Group)
		if utils.Contains(groups, groupObj.GetName()) {
			assert.Contains(a.t, group.Users, a.userName, "the user %s should be present in group %s, Actual: %v", a.userName, group.Name, group.Users)
		} else {
			assert.NotContains(a.t, group.Users, a.userName, "the user %s should NOT be present in group %s, Actual: %v", a.userName, group.Name, group.Users)
		}
	}
	return a
}

func (a *storageAssertionImpl) assertThatGroupHasUsers(name string, usernames ...string) *storageAssertionImpl {
	group := &userv1.Group{}
	a.assertObject("", name, group, func() {
		expLabels := map[string]string{
			"provider": "sandbox-sre",
		}
		assert.Equal(a.t, expLabels, group.Labels)
		sort.Strings(group.Users)
		sort.Strings(usernames)
		assert.Equal(a.t, userv1.OptionalNames(usernames), group.Users)
	})
	return a
}

func (a permissionAssertion) hasRole(namespace, roleName, rolebindingName string) permissionAssertion {
	a.assertRole(namespace, roleName)
	return a.hasRoleBinding(namespace, roleName, rolebindingName, "Role")
}

func (a permissionAssertion) hasNsClusterRole(namespace, roleName, rolebindingName string) permissionAssertion {
	return a.hasRoleBinding(namespace, roleName, rolebindingName, "ClusterRole")
}

func (a permissionAssertion) hasRoleBinding(namespace, roleName, rolebindingName, kind string) permissionAssertion {
	rb := &rbacv1.RoleBinding{}
	a.assertObject(namespace, rolebindingName, rb,
		func() {
			roleRef := rbacv1.RoleRef{
				Kind:     kind,
				APIGroup: "rbac.authorization.k8s.io",
				Name:     roleName,
			}
			assert.Equal(a.t, roleRef, rb.RoleRef)
			assert.Equal(a.t, a.expLabels, rb.Labels)
			require.Len(a.t, rb.Subjects, 1)
			assert.Equal(a.t, a.subject, rb.Subjects[0])
		})
	return a
}

func (a permissionAssertion) hasClusterRoleBinding(clusterRoleName, clusterRoleBindingName string) permissionAssertion { //nolint:unparam
	rb := &rbacv1.ClusterRoleBinding{}
	a.assertObject("", clusterRoleBindingName, rb,
		func() {
			roleRef := rbacv1.RoleRef{
				Kind:     "ClusterRole",
				APIGroup: "rbac.authorization.k8s.io",
				Name:     clusterRoleName,
			}
			assert.Equal(a.t, roleRef, rb.RoleRef)
			assert.Equal(a.t, a.expLabels, rb.Labels)
			require.Len(a.t, rb.Subjects, 1)
			assert.Equal(a.t, a.subject, rb.Subjects[0])
		})
	return a
}

// role assertions

type roleContentAssertion func(*testing.T, *rbacv1.Role)

func hasSameRulesAs(expected *rbacv1.Role) roleContentAssertion {
	return func(t *testing.T, role *rbacv1.Role) {
		assert.Equal(t, expected.Rules, role.Rules)
	}
}

func (a *storageAssertionImpl) assertRole(namespace, roleName string, contentAssertion ...roleContentAssertion) *storageAssertionImpl {
	role := &rbacv1.Role{}
	a.assertObject(namespace, roleName, role, func() {
		expLabels := map[string]string{
			"provider": "sandbox-sre",
		}
		assert.Equal(a.t, expLabels, role.Labels)
		for _, assertContent := range contentAssertion {
			assertContent(a.t, role)
		}
	})
	return a
}

func (a *objectsCacheAssertion) assertNumberOfRoles(expectedNumber int) *objectsCacheAssertion {
	roles, err := a.listObjects("roles", "Role", &rbacv1.Role{})
	require.NoError(a.t, err)
	assert.Len(a.t, roles, expectedNumber)
	return a
}
