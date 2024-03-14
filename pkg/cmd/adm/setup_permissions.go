package adm

import (
	"fmt"
	"sort"

	commonidentity "github.com/codeready-toolchain/toolchain-common/pkg/identity"
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/utils"
	userv1 "github.com/openshift/api/user/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type permissionsManager struct {
	objectsCache
	createSubject   newSubjectFunc
	subjectBaseName string
}

type newSubjectFunc func(ctx *clusterContext, objsCache objectsCache, subjectBaseName, targetNamespace string, labels map[string]string) (rbacv1.Subject, error)

// ensurePermissions creates/updates Subject, Role, and RoleBinding for all permissions defined for the cluster type
func (m *permissionsManager) ensurePermissions(ctx *clusterContext, roleBindingsPerClusterType assets.PermissionsPerClusterType) error {
	// check if there are any permissions set for this cluster type
	if _, ok := roleBindingsPerClusterType[ctx.clusterType.String()]; !ok {
		return nil
	}

	// go through all roleBindings for the cluster type
	for _, roleBindings := range roleBindingsPerClusterType[ctx.clusterType.String()].RoleBindings {

		if roleBindings.Namespace == "" {
			return fmt.Errorf("the namespace name is not defined for one of the role bindings in the cluster type '%s'", ctx.clusterType)
		}

		// ensure RoleBindings with Roles
		for _, role := range roleBindings.Roles {
			if err := m.ensurePermission(ctx, role, roleBindings.Namespace, "Role", newRoleBindingConstructor()); err != nil {
				return err
			}
		}

		// ensure RoleBindings with ClusterRoles
		for _, clusterRole := range roleBindings.ClusterRoles {
			if err := m.ensurePermission(ctx, clusterRole, roleBindings.Namespace, "ClusterRole", newRoleBindingConstructor()); err != nil {
				return err
			}
		}
	}
	for _, clusterRole := range roleBindingsPerClusterType[ctx.clusterType.String()].ClusterRoleBindings.ClusterRoles {
		if err := m.ensurePermission(ctx, clusterRole, "", "ClusterRole", newClusterRoleBindingConstructor()); err != nil {
			return err
		}
	}

	return nil
}

// ensurePermission generates Subject, Role, and RoleBinding for the given permission, namespace and role kind
func (m *permissionsManager) ensurePermission(ctx *clusterContext, roleName, targetNamespace, roleKind string, newBinding bindingConstructor) error {
	grantedRoleName := roleName

	var roleBindingName string
	if roleKind == "Role" {
		// if it is Role, then make sure that it exists in the namespace
		exists, createdRoleName, err := ensureRole(ctx, m.objectsCache, roleName, targetNamespace)
		if err != nil || !exists {
			return err
		}
		grantedRoleName = createdRoleName

		roleBindingName = fmt.Sprintf("%s-%s-%s", roleName, m.subjectBaseName, ctx.clusterType)
	} else {
		// ClusterRole is not managed by sandbox-sre and should already exist in the cluster

		// create RoleBinding name with the prefix clusterrole- so we can avoid conflicts with RoleBindings created for Roles
		roleBindingName = fmt.Sprintf("clusterrole-%s-%s-%s", roleName, m.subjectBaseName, ctx.clusterType)
	}

	// ensure that the subject exists
	subject, err := m.createSubject(ctx, m.objectsCache, m.subjectBaseName, sandboxSRENamespace(ctx.clusterType), sreLabelsWithUsername(m.subjectBaseName))
	if err != nil {
		return err
	}

	// ensure the (Cluster)RoleBinding
	binding := newBinding(targetNamespace, roleBindingName, subject, grantedRoleName, roleKind, sreLabels())
	return m.storeObject(ctx, binding)
}

type bindingConstructor func(namespace, name string, subject rbacv1.Subject, roleName, roleKind string, labels map[string]string) runtimeclient.Object

func newRoleBindingConstructor() bindingConstructor {
	return func(namespace, name string, subject rbacv1.Subject, roleName, roleKind string, labels map[string]string) runtimeclient.Object {
		return newRoleBinding(namespace, name, subject, roleName, roleKind, labels)
	}
}

func newRoleBinding(namespace, name string, subject rbacv1.Subject, roleName, roleKind string, labels map[string]string) runtimeclient.Object {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Subjects: []rbacv1.Subject{subject},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     roleKind,
			Name:     roleName,
		},
	}
}

func newClusterRoleBindingConstructor() bindingConstructor {
	return func(_, name string, subject rbacv1.Subject, roleName, roleKind string, labels map[string]string) runtimeclient.Object {
		return newClusterRoleBinding(name, subject, roleName, roleKind, labels)
	}
}

func newClusterRoleBinding(name string, subject rbacv1.Subject, roleName, roleKind string, labels map[string]string) runtimeclient.Object {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Subjects: []rbacv1.Subject{subject},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     roleKind,
			Name:     roleName,
		},
	}
}

var ignoreExtraneousAnnotations = map[string]string{"argocd.argoproj.io/compare-options": "IgnoreExtraneous"}

// ensureServiceAccount ensures that the ServiceAccount exists
func ensureServiceAccount(saNamespace string) newSubjectFunc {
	return func(ctx *clusterContext, cache objectsCache, subjectName, targetNamespace string, labels map[string]string) (rbacv1.Subject, error) {
		if saNamespace != "" {
			targetNamespace = saNamespace
		}
		if targetNamespace == "" {
			return rbacv1.Subject{}, fmt.Errorf("the SA %s doesn't have any namespace set but requires a ClusterRoleBinding - you need to specify the target namespace of the SA", subjectName)
		}

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   targetNamespace,
				Name:        subjectName,
				Labels:      labels,
				Annotations: ignoreExtraneousAnnotations,
			},
		}

		if err := cache.storeObject(ctx, sa); err != nil {
			return rbacv1.Subject{}, err
		}

		return rbacv1.Subject{
			Name:      subjectName,
			Namespace: targetNamespace,
			Kind:      "ServiceAccount",
		}, nil
	}
}

// ensureUserIdentityAndGroups ensures that all - User, Identity, IdentityMapping, and Group manifests - exist
func ensureUserIdentityAndGroups(IDs []string, groups []string) newSubjectFunc {
	return func(ctx *clusterContext, cache objectsCache, subjectBaseName, targetNamespace string, labels map[string]string) (rbacv1.Subject, error) {
		// create user
		user := &userv1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name:        subjectBaseName,
				Labels:      labels,
				Annotations: ignoreExtraneousAnnotations,
			},
		}
		if err := cache.storeObject(ctx, user); err != nil {
			return rbacv1.Subject{}, err
		}
		if err := ensureGroupsForUser(ctx, cache, subjectBaseName, groups...); err != nil {
			return rbacv1.Subject{}, err
		}

		// Create identities and identity mappings
		for _, id := range IDs {

			ins := commonidentity.NewIdentityNamingStandard(id, "DevSandbox")

			// create identity
			identity := &userv1.Identity{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: ignoreExtraneousAnnotations,
				},
			}

			ins.ApplyToIdentity(identity)

			if err := cache.storeObject(ctx, identity); err != nil {
				return rbacv1.Subject{}, err
			}
		}
		return rbacv1.Subject{
			Name: subjectBaseName,
			Kind: "User",
		}, nil
	}
}

// ensureGroupsForUser ensures that all given Groups exist and that the user is listed in all of them, but not in other groups
func ensureGroupsForUser(ctx *clusterContext, cache objectsCache, user string, groups ...string) error {
	for _, groupName := range groups {
		group := &userv1.Group{
			ObjectMeta: metav1.ObjectMeta{
				Name:   groupName,
				Labels: sreLabels(),
			},
			Users: []string{user},
		}
		if err := cache.ensureObject(ctx, group, func(object runtimeclient.Object) (bool, error) {
			existing, ok := object.(*userv1.Group)
			if !ok {
				return false, fmt.Errorf("object %s is not of the type of Group", object.GetName())
			}
			// if it already contains the user, then continue with the next group
			if utils.Contains(existing.Users, user) {
				return false, nil
			}
			// add user to the group and update it
			existing.Users = append(existing.Users, user)
			sort.Strings(existing.Users)
			return true, nil

		}); err != nil {
			return err
		}
	}
	return nil
}
