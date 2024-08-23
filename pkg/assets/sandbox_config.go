package assets

type KubeSawAdmins struct {
	Clusters        Clusters         `yaml:"clusters"`
	ServiceAccounts []ServiceAccount `yaml:"serviceAccounts"`
	Users           []User           `yaml:"users"`
}

type Clusters struct {
	Host    ClusterConfig   `yaml:"host"`
	Members []MemberCluster `yaml:"members"`
}

type MemberCluster struct {
	Name                       string `yaml:"name"`
	ClusterConfig              `yaml:",inline"`
	SeparateKustomizeComponent bool `yaml:"separateKustomizeComponent"`
}

type ClusterConfig struct {
	API string `yaml:"api"`
}

type ServiceAccount struct {
	Name                      string   `yaml:"name"`
	Namespace                 string   `yaml:"namespace,omitempty"`
	Selector                  Selector `yaml:"selector"`
	PermissionsPerClusterType `yaml:",inline"`
}

type Selector struct {
	SkipMembers []string `yaml:"skipMembers,omitempty"`
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
