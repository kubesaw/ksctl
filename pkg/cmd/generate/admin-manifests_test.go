package generate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	uuid "github.com/google/uuid"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdminManifests(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	kubeSawAdmins := NewKubeSawAdmins(
		Clusters(HostServerAPI).
			AddMember("member1", Member1ServerAPI).
			AddMember("member2", Member2ServerAPI, WithSeparateKustomizeComponent()),
		ServiceAccounts(
			Sa("john", "",
				HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("admin")),
				MemberRoleBindings("toolchain-member-operator", Role("install-operator"), ClusterRole("admin"))).
				WithSkippedMembers("member2"),
			Sa("bob", "",
				HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("edit")),
				MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("edit")))),
		Users(
			User("john-user", []string{"12345"}, false, "crtadmins-view",
				HostRoleBindings("toolchain-host-operator", Role("register-cluster"), ClusterRole("edit")),
				MemberRoleBindings("toolchain-member-operator", Role("register-cluster"), ClusterRole("edit"))).
				WithSkippedMembers("member2"),
			User("bob-crtadmin", []string{"67890"}, false, "crtadmins-exec",
				HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("admin")),
				MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("admin")))))
	kubeSawAdmins.DefaultServiceAccountsNamespace.Host = "kubesaw-sre-host"
	kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
	require.NoError(t, err)

	configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
	files := newDefaultFiles(t)

	t.Run("all created", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("in single-cluster mode", func(t *testing.T) {
		t.Run("fails with separateKustomizeComponent set for member2", func(t *testing.T) {
			// given
			outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
			require.NoError(t, err)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), singleCluster())

			// when
			err = adminManifests(term, files, flags)

			// then
			require.EqualError(t, err, "--single-cluster flag cannot be used with separateKustomizeComponent set in one of the members (member2)")
		})

		t.Run("without separateKustomizeComponent set for member2", func(t *testing.T) {
			// given
			kubeSawAdmins.Clusters.Members[1].SeparateKustomizeComponent = false
			t.Cleanup(func() {
				kubeSawAdmins.Clusters.Members[1].SeparateKustomizeComponent = true
			})
			kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
			require.NoError(t, err)

			configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
			files := newDefaultFiles(t)

			outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
			require.NoError(t, err)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), singleCluster())

			// when
			err = adminManifests(term, files, flags)

			// then
			require.NoError(t, err)
			verifyFiles(t, flags)
		})
	})

	t.Run("in custom host root directory", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), hostRootDir("host-cluster"))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("in custom member root directory", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), memberRootDir("member-clusters"))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("previous deleted", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		storeDummySA(t, outTempDir)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("if out dir doesn't exist then it creates", func(t *testing.T) {
		// given
		outTempDir := filepath.Join(os.TempDir(), fmt.Sprintf("admin-manifests-cli-test-%s", uuid.NewString()))
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("fails for non-existing kubesaw-admins.yaml file", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile("does/not/exist"))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.Error(t, err)
	})

	t.Run("when default SAs namespace names are the same, then fail", func(t *testing.T) {
		// given
		kubeSawAdmins.DefaultServiceAccountsNamespace.Host = "kubesaw-sre"
		kubeSawAdmins.DefaultServiceAccountsNamespace.Member = "kubesaw-sre"
		t.Cleanup(func() {
			kubeSawAdmins.DefaultServiceAccountsNamespace.Host = "kubesaw-sre-host"
			kubeSawAdmins.DefaultServiceAccountsNamespace.Member = ""
		})
		kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
		require.NoError(t, err)

		configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
		files := newDefaultFiles(t)

		outTempDir, err := os.MkdirTemp("", "admin-manifests-cli-test-")
		require.NoError(t, err)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		flags := newAdminManifestsFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = adminManifests(term, files, flags)

		// then
		require.EqualError(t, err, "the default ServiceAccounts namespace has the same name for host cluster as for the member clusters (kubesaw-sre), they have to be different")
	})

}

func storeDummySA(t *testing.T, outDir string) {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "User",
			APIVersion: userv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "dummy-namespace",
			Name:      "dummy-name",
		},
	}
	storeCtx := manifestStoreContext{
		outDir:        outDir,
		memberRootDir: "member",
		hostRootDir:   "host",
	}
	err := writeManifest(storeCtx, filePath(filepath.Join(outDir, "dummy"), sa, "ServiceAccount"), sa)
	require.NoError(t, err)
}

func verifyFiles(t *testing.T, flags adminManifestsFlags) {
	dirEntries, err := os.ReadDir(flags.outDir)
	require.NoError(t, err)
	assert.Len(t, dirEntries, 3)
	dirNames := []string{dirEntries[0].Name(), dirEntries[1].Name(), dirEntries[2].Name()}

	for _, clusterType := range configuration.ClusterTypes {
		ns := commontest.HostOperatorNs
		expectedRootDir := flags.hostRootDir
		if clusterType == configuration.Member {
			ns = commontest.MemberOperatorNs
			expectedRootDir = flags.memberRootDir
		}
		assert.Contains(t, dirNames, expectedRootDir)
		verifyServiceAccounts(t, flags.outDir, expectedRootDir, clusterType, ns)
		verifyUsers(t, flags.outDir, expectedRootDir, clusterType, ns, flags.singleCluster)
	}

	if !flags.singleCluster {
		// if singleCluster is not used then let's verify that member2 was generated in a separate kustomize component
		verifyServiceAccounts(t, flags.outDir, "member2", configuration.Member, commontest.MemberOperatorNs)
		verifyUsers(t, flags.outDir, "member2", configuration.Member, commontest.MemberOperatorNs, flags.singleCluster)
	}
}

func verifyServiceAccounts(t *testing.T, outDir, expectedRootDir string, clusterType configuration.ClusterType, roleNs string) {
	saNs := "kubesaw-sre-host"
	if clusterType == configuration.Member {
		saNs = "kubesaw-admins-member"
	}

	if expectedRootDir != "member2" {
		// john is skipped for member2 (when generated as a separate kustomize component)
		inKStructure(t, outDir, expectedRootDir).
			assertSa(saNs, "john").
			hasRole(roleNs, clusterType.AsSuffix("install-operator"), clusterType.AsSuffix("install-operator-john")).
			hasNsClusterRole(roleNs, "admin", clusterType.AsSuffix("clusterrole-admin-john"))
	}
	inKStructure(t, outDir, expectedRootDir).
		assertSa(saNs, "bob").
		hasRole(roleNs, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob")).
		hasNsClusterRole(roleNs, "edit", clusterType.AsSuffix("clusterrole-edit-bob"))
}

func verifyUsers(t *testing.T, outDir, expectedRootDir string, clusterType configuration.ClusterType, ns string, singleCluster bool) {
	rootDir := expectedRootDir
	if singleCluster {
		rootDir = "base"
	}

	storageAssertion := inKStructure(t, outDir, expectedRootDir).storageAssertionImpl
	bobsExtraGroupsUserIsNotPartOf := extraGroupsUserIsNotPartOf()
	if expectedRootDir != "member2" {
		// john is skipped for member2 (when generated as a separate kustomize component)
		inKStructure(t, outDir, rootDir).
			assertUser("john-user").
			hasIdentity("12345").
			belongsToGroups(groups("crtadmins-view"), extraGroupsUserIsNotPartOf("crtadmins-exec"))

		newPermissionAssertion(storageAssertion, "", "john-user", "User").
			hasRole(ns, clusterType.AsSuffix("register-cluster"), clusterType.AsSuffix("register-cluster-john-user")).
			hasNsClusterRole(ns, "edit", clusterType.AsSuffix("clusterrole-edit-john-user"))

		// crtadmins-view group is not generated for member2 at all
		bobsExtraGroupsUserIsNotPartOf = extraGroupsUserIsNotPartOf("crtadmins-view")
	}

	inKStructure(t, outDir, rootDir).
		assertUser("bob-crtadmin").
		hasIdentity("67890").
		belongsToGroups(groups("crtadmins-exec"), bobsExtraGroupsUserIsNotPartOf)

	newPermissionAssertion(storageAssertion, "", "bob-crtadmin", "User").
		hasRole(ns, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob-crtadmin")).
		hasNsClusterRole(ns, "admin", clusterType.AsSuffix("clusterrole-admin-bob-crtadmin"))
}

func createKubeconfigFiles(t *testing.T, contents ...string) []string {
	var fileNames []string
	for _, content := range contents {
		tempFile, err := os.CreateTemp("", "ksctl-kubeconfig-")
		require.NoError(t, err)

		err = os.WriteFile(tempFile.Name(), []byte(content), os.FileMode(0755))
		require.NoError(t, err)

		require.NoError(t, tempFile.Close())
		fileNames = append(fileNames, tempFile.Name())
	}
	return fileNames
}

const ksctlKubeconfigContent = `
apiVersion: v1
clusters:
- cluster:
    server: https://api.kubesaw.host.openshiftapps.com:6443
  name: api-kubesaw-host-openshiftapps-com:6443
- cluster:
    server: https://api.kubesaw.member1.openshiftapps.com:6443
  name: api-kubesaw-member1-openshiftapps-com:6443
contexts:
- context:
    cluster: api-kubesaw-host-openshiftapps-com:6443
    namespace: toolchain-host-operator
    user: dedicatedadmin
  name: host
- context:
    cluster: api-kubesaw-member1-openshiftapps-com:6443
    namespace: toolchain-member-operator
    user: dedicatedadmin
  name: member1
current-context: host
kind: Config
preferences: {}
users:
- name: dedicatedadmin
  user:
    token: my-cool-token
`

const ksctlKubeconfigContentMember2 = `
apiVersion: v1
clusters:
- cluster:
    server: https://api.kubesaw.member2.openshiftapps.com:6443
  name: api-kubesaw-member2-openshiftapps-com:6443
contexts:
- context:
    cluster: api-kubesaw-member2-openshiftapps-com:6443
    namespace: toolchain-member-operator
    user: dedicatedadmin
  name: member2
current-context: member2
kind: Config
preferences: {}
users:
- name: dedicatedadmin
  user:
    token: my-cool-token
`

type adminManifestsFlagsOption func(*adminManifestsFlags)

func newAdminManifestsFlags(adminManifestsFlagsOptions ...adminManifestsFlagsOption) adminManifestsFlags {
	flags := adminManifestsFlags{
		hostRootDir:   "host",
		memberRootDir: "member",
		idpName:       "KubeSaw",
	}
	for _, applyOption := range adminManifestsFlagsOptions {
		applyOption(&flags)
	}
	return flags
}

func kubeSawAdminsFile(configName string) adminManifestsFlagsOption {
	return func(flags *adminManifestsFlags) {
		flags.kubeSawAdminsFile = configName
	}
}

func outDir(outDir string) adminManifestsFlagsOption {
	return func(flags *adminManifestsFlags) {
		flags.outDir = outDir
	}
}

func hostRootDir(hostRootDir string) adminManifestsFlagsOption {
	return func(flags *adminManifestsFlags) {
		flags.hostRootDir = hostRootDir
	}
}

func memberRootDir(memberRootDir string) adminManifestsFlagsOption {
	return func(flags *adminManifestsFlags) {
		flags.memberRootDir = memberRootDir
	}
}

func singleCluster() adminManifestsFlagsOption {
	return func(flags *adminManifestsFlags) {
		flags.singleCluster = true
	}
}
