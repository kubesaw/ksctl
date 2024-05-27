package test

import (
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/configuration"
)

func NewKubeSawAdmins(addClusters ClustersCreator, serviceAccounts []assets.ServiceAccount, users []assets.User) *assets.KubeSawAdmins {
	sc := &assets.KubeSawAdmins{
		ServiceAccounts: serviceAccounts,
		Users:           users,
	}
	addClusters(&sc.Clusters)
	return sc
}

type ClustersCreator func(*assets.Clusters)

func Clusters(hostURL string) ClustersCreator {
	return func(clusters *assets.Clusters) {
		clusters.Host = assets.ClusterConfig{
			API: hostURL,
		}
	}
}

func (m ClustersCreator) AddMember(name, URL string) ClustersCreator {
	return func(clusters *assets.Clusters) {
		m(clusters)
		clusters.Members = append(clusters.Members, assets.MemberCluster{
			Name: name,
			ClusterConfig: assets.ClusterConfig{
				API: URL,
			},
		})
	}
}

type ServiceAccountCreator func() assets.ServiceAccount

func ServiceAccounts(serviceAccountModifiers ...ServiceAccountCreator) []assets.ServiceAccount {
	var serviceAccounts []assets.ServiceAccount
	for _, createSa := range serviceAccountModifiers {
		serviceAccounts = append(serviceAccounts, createSa())
	}
	return serviceAccounts
}

func Sa(baseName, namespace string, permissions ...PermissionsPerClusterTypeModifier) ServiceAccountCreator { //nolint:unparam
	return func() assets.ServiceAccount {
		sa := assets.ServiceAccount{
			Name:                      baseName,
			Namespace:                 namespace,
			PermissionsPerClusterType: NewPermissionsPerClusterType(permissions...),
		}
		return sa
	}
}

func NewPermissionsPerClusterType(permissions ...PermissionsPerClusterTypeModifier) assets.PermissionsPerClusterType {
	perm := map[string]assets.PermissionBindings{}
	for _, addPermissions := range permissions {
		addPermissions(perm)
	}
	return perm
}

type RoleBindingsModifier func(*assets.RoleBindings)
type PermissionsPerClusterTypeModifier func(assets.PermissionsPerClusterType)

func HostRoleBindings(namespace string, modifiers ...RoleBindingsModifier) PermissionsPerClusterTypeModifier {
	return func(namespacePermissionsPerClusterType assets.PermissionsPerClusterType) {
		RoleBindings(namespacePermissionsPerClusterType, configuration.Host, namespace, modifiers...)
	}
}

func MemberRoleBindings(namespace string, modifiers ...RoleBindingsModifier) PermissionsPerClusterTypeModifier {
	return func(namespacePermissionsPerClusterType assets.PermissionsPerClusterType) {
		RoleBindings(namespacePermissionsPerClusterType, configuration.Member, namespace, modifiers...)
	}
}

func RoleBindings(namespacePermissionsPerClusterType assets.PermissionsPerClusterType, clusterType configuration.ClusterType, namespace string, modifiers ...RoleBindingsModifier) {
	nsPermissions := assets.RoleBindings{
		Namespace: namespace,
	}
	for _, modify := range modifiers {
		modify(&nsPermissions)
	}
	permissions := namespacePermissionsPerClusterType[clusterType.String()]
	permissions.RoleBindings = append(permissions.RoleBindings, nsPermissions)
	namespacePermissionsPerClusterType[clusterType.String()] = permissions
}

func HostClusterRoleBindings(clusterRoles ...string) PermissionsPerClusterTypeModifier {
	return func(namespacePermissionsPerClusterType assets.PermissionsPerClusterType) {
		ClusterRolesBindings(namespacePermissionsPerClusterType, configuration.Host, clusterRoles...)
	}
}

func MemberClusterRoleBindings(clusterRoles ...string) PermissionsPerClusterTypeModifier {
	return func(namespacePermissionsPerClusterType assets.PermissionsPerClusterType) {
		ClusterRolesBindings(namespacePermissionsPerClusterType, configuration.Member, clusterRoles...)
	}
}

func ClusterRolesBindings(namespacePermissionsPerClusterType assets.PermissionsPerClusterType, clusterType configuration.ClusterType, clusterRoles ...string) {
	roles := namespacePermissionsPerClusterType[clusterType.String()]
	roles.ClusterRoleBindings.ClusterRoles = append(roles.ClusterRoleBindings.ClusterRoles, clusterRoles...)
	namespacePermissionsPerClusterType[clusterType.String()] = roles
}

func Role(roles ...string) RoleBindingsModifier {
	return func(roleBinding *assets.RoleBindings) {
		roleBinding.Roles = append(roleBinding.Roles, roles...)
	}
}

func ClusterRole(clusterRoles ...string) RoleBindingsModifier {
	return func(roleBinding *assets.RoleBindings) {
		roleBinding.ClusterRoles = append(roleBinding.ClusterRoles, clusterRoles...)
	}
}

type UserCreator func() assets.User

func Users(userCreators ...UserCreator) []assets.User {
	var users []assets.User
	for _, createUser := range userCreators {
		users = append(users, createUser())
	}
	return users
}

func User(name string, IDs []string, allCluster bool, group string, permissions ...PermissionsPerClusterTypeModifier) UserCreator {
	return func() assets.User {
		var groups []string
		if group != "" {
			groups = []string{group}
		}
		user := assets.User{
			Name:                      name,
			ID:                        IDs,
			Groups:                    groups,
			AllClusters:               allCluster,
			PermissionsPerClusterType: map[string]assets.PermissionBindings{},
		}
		for _, addPermissions := range permissions {
			addPermissions(user.PermissionsPerClusterType)
		}
		return user
	}
}
