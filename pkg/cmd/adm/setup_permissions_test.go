package adm

import (
	"fmt"
	"testing"

	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var permissionsForAllNamespaces = []PermissionsPerClusterTypeModifier{
	HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("view")),
	HostRoleBindings("openshift-customer-monitoring", Role("restart-deployment"), ClusterRole("edit")),
	HostRoleBindings("openshift-customer-monitoring", Role("edit-deployment"), ClusterRole("edit-secrets")),
	MemberRoleBindings("toolchain-member-operator", Role("install-operator"), ClusterRole("view")),
	MemberRoleBindings("codeready-workspaces-operator", Role("register-cluster"), ClusterRole("admin")),
}

func TestEnsurePermissionsInNamespaces(t *testing.T) {
	// given
	config := newKubeSawAdminsWithDefaultClusters([]assets.ServiceAccount{}, []assets.User{})

	t.Run("create permissions", func(t *testing.T) {
		// given
		permissionsPerClusterTypes := NewPermissionsPerClusterType(permissionsForAllNamespaces...)

		for _, clusterType := range configuration.ClusterTypes {
			t.Run("for cluster type: "+clusterType.String(), func(t *testing.T) {
				permManager, ctx := newPermissionsManager(t, clusterType, config)

				// when
				err := permManager.ensurePermissions(ctx, permissionsPerClusterTypes)

				// then
				require.NoError(t, err)
				roleNs := fmt.Sprintf("toolchain-%s-operator", clusterType)
				saNs := fmt.Sprintf("sandbox-sre-%s", clusterType)

				inObjectCache(t, ctx.outDir, clusterType.String(), permManager.objectsCache).
					assertSa(saNs, "john").
					hasRole(roleNs, clusterType.AsSuffix("install-operator"), clusterType.AsSuffix("install-operator-john")).
					hasNsClusterRole(roleNs, "view", clusterType.AsSuffix("clusterrole-view-john"))

				if clusterType == configuration.Host {
					inObjectCache(t, ctx.outDir, clusterType.String(), permManager.objectsCache).
						assertSa(saNs, "john").
						hasRole("openshift-customer-monitoring", clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-john")).
						hasRole("openshift-customer-monitoring", clusterType.AsSuffix("edit-deployment"), clusterType.AsSuffix("edit-deployment-john")).
						hasNsClusterRole("openshift-customer-monitoring", "edit", clusterType.AsSuffix("clusterrole-edit-john")).
						hasNsClusterRole("openshift-customer-monitoring", "edit-secrets", clusterType.AsSuffix("clusterrole-edit-secrets-john"))
				} else {
					inObjectCache(t, ctx.outDir, clusterType.String(), permManager.objectsCache).
						assertSa(saNs, "john").
						hasRole("codeready-workspaces-operator", clusterType.AsSuffix("register-cluster"), clusterType.AsSuffix("register-cluster-john")).
						hasNsClusterRole("codeready-workspaces-operator", "admin", clusterType.AsSuffix("clusterrole-admin-john"))
				}
			})
		}
	})

	t.Run("if there is no record for the member cluster type, then skip", func(t *testing.T) {
		// given
		perms := NewPermissionsPerClusterType(HostRoleBindings(commontest.HostOperatorNs, Role("install-operator"), ClusterRole("view")))
		permissionsManager, ctx := newPermissionsManager(t, configuration.Member, config)

		// when
		err := permissionsManager.ensurePermissions(ctx, perms)

		// then
		require.NoError(t, err)
		assertNoClusterTypeEntry(t, ctx.outDir, configuration.Member)
	})
}

func TestEnsureServiceAccount(t *testing.T) {

	labels := map[string]string{
		"provider": "sandbox-sre",
		"username": "john",
	}

	t.Run("create SA", func(t *testing.T) {
		// given
		ctx := newFakeClusterContext(newSetupContextWithDefaultFiles(t, nil), configuration.Host)
		cache := objectsCache{}

		// when
		subject, err := ensureServiceAccount("")(
			ctx, cache, "john", "sandbox-sre-host", labels)

		// then
		require.NoError(t, err)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertSa("sandbox-sre-host", "john")
		assert.Equal(t, "ServiceAccount", subject.Kind)
		assert.Equal(t, "john", subject.Name)
		assert.Equal(t, "sandbox-sre-host", subject.Namespace)
	})

	t.Run("create SA in the given namespace", func(t *testing.T) {
		// given
		ctx := newFakeClusterContext(newSetupContextWithDefaultFiles(t, nil), configuration.Host)
		cache := objectsCache{}

		// when
		subject, err := ensureServiceAccount("openshift-customer-monitoring")(
			ctx, cache, "john", "sandbox-sre-host", labels)

		// then
		require.NoError(t, err)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertSa("openshift-customer-monitoring", "john")
		assert.Equal(t, "ServiceAccount", subject.Kind)
		assert.Equal(t, "john", subject.Name)
		assert.Equal(t, "openshift-customer-monitoring", subject.Namespace)
	})
}

func TestEnsureUserAndIdentity(t *testing.T) {
	labels := map[string]string{
		"provider": "sandbox-sre",
		"username": "john-crtadmin",
	}
	require.NoError(t, client.AddToScheme())

	t.Run("create user, multiple identity & groups", func(t *testing.T) {
		// given
		ctx := newFakeClusterContext(newSetupContextWithDefaultFiles(t, nil), configuration.Host)
		cache := objectsCache{}

		// when
		subject, err := ensureUserIdentityAndGroups([]string{"12345", "abc:19944:FZZ"}, []string{"crtadmins", "cooladmins"})(ctx, cache, "john-crtadmin", commontest.HostOperatorNs, labels)

		// then
		require.NoError(t, err)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertUser("john-crtadmin").
			hasIdentity("12345").
			hasIdentity("abc:19944:FZZ").
			belongsToGroups(groups("crtadmins", "cooladmins"), extraGroupsUserIsNotPartOf())
		assert.Equal(t, "User", subject.Kind)
		assert.Equal(t, "john-crtadmin", subject.Name)
		assert.Empty(t, subject.Namespace)
	})

	t.Run("don't create any group", func(t *testing.T) {
		// given
		ctx := newFakeClusterContext(newSetupContextWithDefaultFiles(t, nil), configuration.Host)
		cache := objectsCache{}

		// when
		_, err := ensureUserIdentityAndGroups([]string{"12345"}, []string{})(ctx, cache, "john-crtadmin", commontest.HostOperatorNs, labels)

		// then
		require.NoError(t, err)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertUser("john-crtadmin").
			hasIdentity("12345").
			belongsToGroups(groups(), extraGroupsUserIsNotPartOf())
	})
}

func TestEnsureGroupsForUser(t *testing.T) {
	require.NoError(t, client.AddToScheme())

	t.Run("when creating group(s)", func(t *testing.T) {
		// given
		ctx := newFakeClusterContext(newSetupContextWithDefaultFiles(t, nil), configuration.Host)
		cache := objectsCache{}

		// when
		err := ensureGroupsForUser(ctx, cache, "cool-user", "crtadmins")

		// then
		require.NoError(t, err)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertThatGroupHasUsers("crtadmins", "cool-user")

		t.Run("another user with multiple groups", func(t *testing.T) {
			// when
			err := ensureGroupsForUser(ctx, cache, "another-user", "cool-group", "crtadmins", "another-group")

			// then
			require.NoError(t, err)
			inObjectCache(t, ctx.outDir, "host", cache).
				assertThatGroupHasUsers("crtadmins", "cool-user", "another-user").
				assertThatGroupHasUsers("cool-group", "another-user").
				assertThatGroupHasUsers("another-group", "another-user")
		})
	})
}

func newPermissionsManager(t *testing.T, clusterType configuration.ClusterType, config *assets.KubeSawAdmins) (permissionsManager, *clusterContext) { // nolint:unparam
	ctx := newSetupContextWithDefaultFiles(t, config)
	clusterCtx := newFakeClusterContext(ctx, clusterType)
	cache := objectsCache{}

	return permissionsManager{
		objectsCache:    cache,
		subjectBaseName: "john",
		createSubject:   ensureServiceAccount(""),
	}, clusterCtx
}
