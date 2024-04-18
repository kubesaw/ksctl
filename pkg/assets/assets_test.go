package assets_test

import (
	"strings"
	"testing"

	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	hostRoles = `apiVersion: template.openshift.io/v1
kind: Template
metadata:
  name: host-roles
objects:
- kind: Role
  apiVersion: rbac.authorization.k8s.io/v1
  metadata:
    name: get-catalogsources
    labels:
      provider: sandbox-sre
  rules:
  - apiGroups:
    - operators.coreos.com
    resources:
    - "catalogsources"
    verbs:
    - "get"`

	memberRoles = `apiVersion: template.openshift.io/v1
kind: Template
metadata:
  name: member-roles
objects:
- kind: Role
  apiVersion: rbac.authorization.k8s.io/v1
  metadata:
    name: get-deployments
    labels:
      provider: sandbox-sre
  rules:
  - apiGroups:
    - apps
    resources:
    - deployments
    verbs:
    - "get"`
)

func TestGetRoles(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	files := NewFakeFiles(t,
		FakeFile("roles/host.yaml", []byte(hostRoles)),
		FakeFile("roles/member.yaml", []byte(memberRoles)),
	)

	for _, clusterType := range configuration.ClusterTypes {

		t.Run("get roles for cluster type "+clusterType.String(), func(t *testing.T) {
			// when
			objs, err := assets.GetRoles(files, clusterType)

			// then
			require.NoError(t, err)
			require.Len(t, objs, 1)
			roleObject := objs[0]

			unstructuredRole, ok := roleObject.(*unstructured.Unstructured)
			require.True(t, ok)
			role := &rbacv1.Role{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredRole.Object, role)
			require.NoError(t, err)
			assert.Empty(t, role.Namespace)

			if clusterType == configuration.Host {
				assert.Equal(t, "get-catalogsources", role.Name)
			} else {
				assert.Equal(t, "get-deployments", role.Name)
			}
		})
	}
}

func TestGetKubeSawAdmins(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())

	// when
	kubeSawAdmins, err := assets.GetKubeSawAdminsConfig("../../test-resources/dummy.openshiftapps.com/kubesaw-admins.yaml")

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, kubeSawAdmins)
	assert.NotEmpty(t, kubeSawAdmins.Clusters.Host.API)
	assert.NotEmpty(t, kubeSawAdmins.Clusters.Members)

	for _, member := range kubeSawAdmins.Clusters.Members {
		assert.NotEmpty(t, member.Name)
		assert.NotEmpty(t, member.API)
	}

	assert.NotEmpty(t, kubeSawAdmins.ServiceAccounts)
	for _, sa := range kubeSawAdmins.ServiceAccounts {
		assert.NotEmpty(t, sa.Name)
		verifyNamespacePermissions(t, sa.Name, sa.PermissionsPerClusterType)
	}

	assert.NotEmpty(t, kubeSawAdmins.Users)
	for _, user := range kubeSawAdmins.Users {
		assert.NotEmpty(t, user.Name)
		assert.NotEmpty(t, user.ID)
		verifyNamespacePermissions(t, user.Name, user.PermissionsPerClusterType)
	}
}

func verifyNamespacePermissions(t *testing.T, entityName string, perClusterType assets.PermissionsPerClusterType) {
	assert.NotEmpty(t, perClusterType)
	for clusterType, permissions := range perClusterType {
		if clusterType != configuration.Host.String() && clusterType != configuration.Member.String() {
			assert.Failf(t, "not supported cluster type", "the cluster type '%s' should be either host or member", clusterType)
		}
		roles, err := assets.GetRoles(resources.Resources, configuration.ClusterType(clusterType))
		require.NoError(t, err)
		var roleNames []string
		for _, role := range roles {
			roleNames = append(roleNames, role.GetName())
		}

		assert.NotEmpty(t, len(permissions.RoleBindings)+len(permissions.ClusterRoleBindings.ClusterRoles))
		for _, roleBindings := range permissions.RoleBindings {
			assert.NotEmpty(t, roleBindings.Namespace)
			if len(roleBindings.Roles) == 0 && len(roleBindings.ClusterRoles) == 0 {
				assert.Failf(t, "missing permissions definitions", "there is not defined either a role nor a clusterRole for '%s': '%v'", entityName, roleBindings)
			}
			for _, role := range roleBindings.Roles {
				if strings.Contains(role, "=") {
					role = strings.Split(role, "=")[1]
				}
				assert.Contains(t, roleNames, role)
			}
		}
		for _, clusterRole := range permissions.ClusterRoleBindings.ClusterRoles {
			assert.NotEmpty(t, clusterRole)
		}
	}
}
