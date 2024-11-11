package assets

import "k8s.io/utils/strings/slices"

type KubeSawAdmins struct {
	Clusters                        Clusters                        `yaml:"clusters"`
	ServiceAccounts                 []ServiceAccount                `yaml:"serviceAccounts"`
	Users                           []User                          `yaml:"users"`
	DefaultServiceAccountsNamespace DefaultServiceAccountsNamespace `yaml:"defaultServiceAccountsNamespace"`
}

type Clusters struct {
	Host    ClusterConfig   `yaml:"host"`
	Members []MemberCluster `yaml:"members"`
}

type MemberCluster struct {
	Name          string `yaml:"name"`
	ClusterConfig `yaml:",inline"`
	// SeparateKustomizeComponent when set to true, then the manifests for the member cluster will be generated in a separate
	// Kustomize component (a directory structure that will contain all the generated manifests including the kustomization.yaml files).
	// The name of the root folder will have the same name as the name of the member cluster.
	SeparateKustomizeComponent bool `yaml:"separateKustomizeComponent"`
}

type ClusterConfig struct {
	API string `yaml:"api"`
}

// DefaultServiceAccountsNamespace defines the names of the default namespaces where the ksctl SAs should be created.
// If not specified, then the names kubesaw-admins-host and kubesaw-admins-member are used.
type DefaultServiceAccountsNamespace struct {
	Host   string `yaml:"host"`
	Member string `yaml:"member"`
}

type ServiceAccount struct {
	Name                      string   `yaml:"name"`
	Namespace                 string   `yaml:"namespace,omitempty"`
	Selector                  Selector `yaml:"selector"`
	PermissionsPerClusterType `yaml:",inline"`
}

// Selector contains fields to select clusters the entity should (not) be applied for
type Selector struct {
	// SkipMembers can contain a list of member cluster names the entity shouldn't be applied for
	SkipMembers []string `yaml:"skipMembers,omitempty"`
	// MemberClusters defines a list of member cluster names the entity should be applied for
	MemberClusters []string `yaml:"memberClusters,omitempty"`
}

func (s Selector) ShouldBeSkippedForMember(memberName string) bool {
	// should be skipped if the specific member cluster name is provided
	//   and
	// the name is listed in the skipped members
	if memberName != "" && slices.Contains(s.SkipMembers, memberName) {
		return true
	}
	// should be skipped if there is at least one selected member cluster
	//   and
	// the name is either empty or is not specified in the selected member clusters
	return len(s.MemberClusters) > 0 && (memberName == "" || !slices.Contains(s.MemberClusters, memberName))
}

type User struct {
	Name                      string   `yaml:"name"`
	ID                        []string `yaml:"id"`
	AllClusters               bool     `yaml:"allClusters,omitempty"` // force user and identity manifest generation on all clusters, even if no permissions are specified
	Groups                    []string `yaml:"groups"`
	Selector                  Selector `yaml:"selector"`
	PermissionsPerClusterType `yaml:",inline,omitempty"`
}

type PermissionsPerClusterType map[string]PermissionBindings

type PermissionBindings struct {
	RoleBindings        []RoleBindings      `yaml:"roleBindings"`
	ClusterRoleBindings ClusterRoleBindings `yaml:"clusterRoleBindings"`
}

type RoleBindings struct {
	Namespace    string   `yaml:"namespace"`
	Roles        []string `yaml:"roles,omitempty"`
	ClusterRoles []string `yaml:"clusterRoles,omitempty"`
}

type ClusterRoleBindings struct {
	ClusterRoles []string `yaml:"clusterRoles,omitempty"`
}
