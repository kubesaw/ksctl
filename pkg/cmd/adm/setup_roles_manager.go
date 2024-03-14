package adm

import (
	"fmt"

	"github.com/kubesaw/ksctl/pkg/assets"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func getRole(ctx *clusterContext, roleName string) (*rbacv1.Role, error) {
	// get all roles for the cluster type
	roles, err := assets.GetRoles(ctx.files, ctx.clusterType)
	if err != nil {
		return nil, err
	}

	for _, roleObject := range roles {

		if roleObject.GetName() == roleName {
			// cast to Unstructured so we can then convert it to Role object
			unstructuredRole, ok := roleObject.(*unstructured.Unstructured)
			if !ok {
				return nil, fmt.Errorf("unable to cast Role to Unstructured object '%+v'", roleObject)
			}
			role := &rbacv1.Role{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredRole.Object, role); err != nil {
				return nil, err
			}
			return role, nil
		}
	}
	return nil, nil
}

// ensureRole generates the given role for the given namespace
func ensureRole(ctx *clusterContext, cache objectsCache, roleName, namespace string) (bool, string, error) {
	roleNameToBeCreated := ctx.clusterType.AsSuffix(roleName)
	role, err := getRole(ctx, roleName)
	if err != nil {
		return false, roleNameToBeCreated, err
	}
	if role == nil {
		return false, roleNameToBeCreated, fmt.Errorf("there is no such role with the name '%s' defined", roleName)
	}

	roleToBeCreated := role.DeepCopy()
	roleToBeCreated.SetNamespace(namespace)
	roleToBeCreated.SetName(roleNameToBeCreated)
	roleToBeCreated.SetLabels(sreLabels())
	return true, roleNameToBeCreated, cache.storeObject(ctx, roleToBeCreated)
}
