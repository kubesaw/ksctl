package generate

import (
	"github.com/kubesaw/ksctl/pkg/configuration"
)

type clusterContext struct {
	*adminManifestsContext
	clusterType configuration.ClusterType
}

// ensureServiceAccounts reads the list of service accounts definitions and it's permissions.
// It generates SA and roles & roleBindings for them
func ensureServiceAccounts(ctx *clusterContext, objsCache objectsCache) error {
	ctx.Printlnf("-> Ensuring ServiceAccounts and its RoleBindings...")
	for _, sa := range ctx.kubeSawAdmins.ServiceAccounts {

		// by default, it should use the sandbox sre namespace. let's keep this empty (if the target namespace is not defined) so it is recognized in the ensureServiceAccount method based on the cluster type it is being applied in
		saNamespace := ""
		if sa.Namespace != "" {
			saNamespace = sa.Namespace
		}

		pm := &permissionsManager{
			objectsCache:    objsCache,
			createSubject:   ensureServiceAccount(saNamespace),
			subjectBaseName: sa.Name,
		}

		if err := pm.ensurePermissions(ctx, sa.PermissionsPerClusterType); err != nil {
			return err
		}
	}

	return nil
}

// ensureUsers reads the list of users definitions and it's permissions.
// For each of them it generates User and Identity manifests
// If user belongs to a group, then it makes sure that there is a Group manifest with the user name
func ensureUsers(ctx *clusterContext, objsCache objectsCache) error {
	ctx.Printlnf("-> Ensuring Users and its RoleBindings...")

	for _, user := range ctx.kubeSawAdmins.Users {

		permissions := &permissionsManager{
			objectsCache:    objsCache,
			createSubject:   ensureUserIdentityAndGroups(user.ID, user.Groups),
			subjectBaseName: user.Name,
		}

		if err := permissions.ensurePermissions(ctx, user.PermissionsPerClusterType); err != nil {
			return err
		}
	}

	return nil
}
