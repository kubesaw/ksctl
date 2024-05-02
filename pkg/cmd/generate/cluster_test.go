package generate

import (
	"fmt"
	"testing"

	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestEnsureServiceAccounts(t *testing.T) {
	t.Run("create permissions for SA base names", func(t *testing.T) {
		// given
		kubeSawAdmins := newKubeSawAdminsWithDefaultClusters(
			ServiceAccounts(
				Sa("john", "",
					permissionsForAllNamespaces...),
				Sa("bob", "",
					HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("view")),
					MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("view")))),
			[]assets.User{})
		ctx := newAdminManifestsContextWithDefaultFiles(t, kubeSawAdmins)
		cache := objectsCache{}

		for _, clusterType := range configuration.ClusterTypes {
			t.Run("for "+clusterType.String()+" cluster", func(t *testing.T) {
				// given
				clusterCtx := newFakeClusterContext(ctx, clusterType)
				t.Cleanup(gock.OffAll)

				// when
				err := ensureServiceAccounts(clusterCtx, cache)

				// then
				require.NoError(t, err)

				roleNs := fmt.Sprintf("toolchain-%s-operator", clusterType)
				saNs := fmt.Sprintf("sandbox-sre-%s", clusterType)

				inObjectCache(t, ctx.outDir, clusterType.String(), cache).
					assertSa(saNs, "john").
					hasRole(roleNs, clusterType.AsSuffix("install-operator"), clusterType.AsSuffix("install-operator-john")).
					hasNsClusterRole(roleNs, "view", clusterType.AsSuffix("clusterrole-view-john"))

				if clusterType == configuration.Host {
					inObjectCache(t, ctx.outDir, clusterType.String(), cache).
						assertSa(saNs, "john").
						hasRole("openshift-customer-monitoring", clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-john")).
						hasNsClusterRole("openshift-customer-monitoring", "edit", clusterType.AsSuffix("clusterrole-edit-john"))
				} else {
					inObjectCache(t, ctx.outDir, clusterType.String(), cache).
						assertSa(saNs, "john").
						hasRole("codeready-workspaces-operator", clusterType.AsSuffix("register-cluster"), clusterType.AsSuffix("register-cluster-john")).
						hasNsClusterRole("codeready-workspaces-operator", "admin", clusterType.AsSuffix("clusterrole-admin-john"))
				}

				inObjectCache(t, ctx.outDir, clusterType.String(), cache).
					assertSa(saNs, "bob").
					hasRole(roleNs, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob")).
					hasNsClusterRole(roleNs, "view", clusterType.AsSuffix("clusterrole-view-bob"))
			})
		}
	})

	t.Run("create SA with the fixed name, in the given namespace, ClusterRoleBinding set, and don't gather the token", func(t *testing.T) {
		// given
		kubeSawAdmins := newKubeSawAdminsWithDefaultClusters(
			ServiceAccounts(
				Sa("john", "openshift-customer-monitoring",
					HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("view")),
					HostClusterRoleBindings("cluster-monitoring-view"))), Users())
		ctx := newAdminManifestsContextWithDefaultFiles(t, kubeSawAdmins)
		clusterCtx := newFakeClusterContext(ctx, configuration.Host)
		t.Cleanup(gock.OffAll)
		cache := objectsCache{}

		// when
		err := ensureServiceAccounts(clusterCtx, cache)

		// then
		require.NoError(t, err)

		inObjectCache(t, ctx.outDir, "host", cache).
			assertSa("openshift-customer-monitoring", "john").
			hasRole(commontest.HostOperatorNs, "install-operator-host", "install-operator-john-host").
			hasNsClusterRole(commontest.HostOperatorNs, "view", "clusterrole-view-john-host").
			hasClusterRoleBinding("cluster-monitoring-view", "clusterrole-cluster-monitoring-view-john-host")
	})
}

func TestUsers(t *testing.T) {
	t.Run("ensure users", func(t *testing.T) {
		// given
		kubeSawAdmins := newKubeSawAdminsWithDefaultClusters(
			ServiceAccounts(),
			Users(
				User("john-user", []string{"12345"}, "crtadmins",
					permissionsForAllNamespaces...),
				User("bob-crtadmin", []string{"67890"}, "crtadmins",
					HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("view")),
					HostClusterRoleBindings("cluster-monitoring-view"),
					MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("view")),
					MemberClusterRoleBindings("cluster-monitoring-view"))))

		ctx := newAdminManifestsContextWithDefaultFiles(t, kubeSawAdmins)
		cache := objectsCache{}

		for _, clusterType := range configuration.ClusterTypes {
			t.Run("for cluster type: "+clusterType.String(), func(t *testing.T) {
				// given
				clusterCtx := newFakeClusterContext(ctx, clusterType)

				// when
				err := ensureUsers(clusterCtx, cache)

				// then
				require.NoError(t, err)
				ns := fmt.Sprintf("toolchain-%s-operator", clusterType)

				assertion := inObjectCache(t, ctx.outDir, clusterType.String(), cache).
					assertUser("john-user").
					hasIdentity("12345").
					belongsToGroups(groups("crtadmins"), extraGroupsUserIsNotPartOf()).
					hasRole(ns, clusterType.AsSuffix("install-operator"), clusterType.AsSuffix("install-operator-john-user")).
					hasNsClusterRole(ns, "view", clusterType.AsSuffix("clusterrole-view-john-user"))

				if clusterType == configuration.Host {
					// "restart-deployment" RoleBinding prefix was renamed to "restart", but the name of the Role stays the same
					assertion.
						hasRole("openshift-customer-monitoring", clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-john-user")).
						// "edit" RoleBinding prefix was renamed to "editor", but the name of the ClusterRole stays the same
						hasNsClusterRole("openshift-customer-monitoring", "edit", clusterType.AsSuffix("clusterrole-edit-john-user"))

				} else {
					assertion.
						hasRole("codeready-workspaces-operator", clusterType.AsSuffix("register-cluster"), clusterType.AsSuffix("register-cluster-john-user")).
						hasNsClusterRole("codeready-workspaces-operator", "admin", clusterType.AsSuffix("clusterrole-admin-john-user"))
				}

				inObjectCache(t, ctx.outDir, clusterType.String(), cache).
					assertUser("bob-crtadmin").
					hasIdentity("67890").
					belongsToGroups(groups("crtadmins"), extraGroupsUserIsNotPartOf()).
					hasRole(ns, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob-crtadmin")).
					hasNsClusterRole(ns, "view", clusterType.AsSuffix("clusterrole-view-bob-crtadmin")).
					hasClusterRoleBinding("cluster-monitoring-view", clusterType.AsSuffix("clusterrole-cluster-monitoring-view-bob-crtadmin"))
			})
		}
	})
}

func newKubeSawAdminsWithDefaultClusters(serviceAccounts []assets.ServiceAccount, users []assets.User) *assets.KubeSawAdmins {
	return NewKubeSawAdmins(
		Clusters(HostServerAPI).AddMember("member-1", Member1ServerAPI),
		serviceAccounts,
		users)
}
