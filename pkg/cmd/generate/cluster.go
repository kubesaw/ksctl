package generate

import (
	"errors"
	"strings"

	"github.com/kubesaw/ksctl/pkg/configuration"
	"k8s.io/apimachinery/pkg/util/validation"
)

type clusterContext struct {
	*adminManifestsContext
	clusterType         configuration.ClusterType
	specificKMemberName string
}

// ensureServiceAccounts reads the list of service accounts definitions and it's permissions.
// It generates SA and roles & roleBindings for them
func ensureServiceAccounts(ctx *clusterContext, objsCache objectsCache) error {
	ctx.Printlnf("-> Ensuring ServiceAccounts and its RoleBindings...")
	for _, sa := range ctx.kubeSawAdmins.ServiceAccounts {
		if sa.Selector.ShouldBeSkippedForMember(ctx.specificKMemberName) {
			continue
		}

		// let's keep this empty (if the target namespace is not defined) so it is recognized in the ensureServiceAccount method based on the cluster type it is being applied in
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
		if user.Selector.ShouldBeSkippedForMember(ctx.specificKMemberName) {
			continue
		}
		if err := validateUserName(user.Name); err != nil {
			return err
		}
		m := &permissionsManager{
			objectsCache:    objsCache,
			createSubject:   ensureUserIdentityAndGroups(user.ID, user.Groups),
			subjectBaseName: user.Name,
		}
		// create the subject if explicitly requested (even if there is no specific permissions)
		if user.AllClusters {
			if _, err := m.createSubject(ctx, m.objectsCache, m.subjectBaseName, defaultSAsNamespace(ctx.kubeSawAdmins, ctx.clusterType), ksctlLabelsWithUsername(m.subjectBaseName)); err != nil {
				return err
			}
		}
		if err := m.ensurePermissions(ctx, user.PermissionsPerClusterType); err != nil {
			return err
		}
	}

	return nil
}

func validateUserName(userName string) error {
	validationErrors := validation.IsDNS1123Subdomain(userName)
	if len(validationErrors) == 0 {
		validationErrors = validation.IsValidLabelValue(userName)
		if len(validationErrors) == 0 {
			return nil
		}
	}
	errs := make([]error, len(validationErrors))
	for i := 0; i < len(validationErrors); i++ {
		message := strings.ReplaceAll(validationErrors[i], "label", "user name")
		message = strings.ReplaceAll(message, "subdomain", "user name")
		errs[i] = errors.New(message)
	}
	return errors.Join(errs...)
}
