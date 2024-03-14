package adm

import (
	"testing"

	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	installOperatorRole    = newRole("install-operator", newRule("operators.coreos.com", "catalogsources", "create"))
	restartDeploymentRole  = newRole("restart-deployment", newRule("apps", "deployment", "update"))
	editDeploymentRole     = newRole("edit-deployment", newRule("apps", "deployment", "edit"))
	configureComponentRole = newRole("configure-component", newRule("monitoring.coreos.com", "prometheuses", "create"))
	registerClusterRole    = newRole("register-cluster", newRule("toolchain.dev.openshift.com", "usersignups", "update"))
)

func TestGetRole(t *testing.T) {
	// given
	files := NewFakeFiles(t,
		FakeTemplate("setup/roles/host.yaml", installOperatorRole),
		FakeTemplate("setup/roles/member.yaml", restartDeploymentRole, registerClusterRole))
	ctx := newSetupContext(t, &assets.SandboxEnvironmentConfig{}, files)

	t.Run("for host cluster type", func(t *testing.T) {
		// given
		clusterCtx := newFakeClusterContext(ctx, configuration.Host)

		// when
		role, err := getRole(clusterCtx, "install-operator")

		// then
		require.NoError(t, err)
		assert.Equal(t, installOperatorRole, role)

		t.Run("non-existing role", func(t *testing.T) {
			// when
			role, err := getRole(clusterCtx, "does-not-exist")

			// then
			require.NoError(t, err)
			assert.Nil(t, role)
		})
	})

	t.Run("for member cluster type", func(t *testing.T) {
		// given
		clusterCtx := newFakeClusterContext(ctx, configuration.Member)

		// when
		restartRole, err := getRole(clusterCtx, "restart-deployment")
		require.NoError(t, err)
		registerRole, err := getRole(clusterCtx, "register-cluster")
		require.NoError(t, err)

		// then
		assert.Equal(t, restartDeploymentRole, restartRole)
		assert.Equal(t, registerClusterRole, registerRole)

		t.Run("non-existing role", func(t *testing.T) {
			// when
			role, err := getRole(clusterCtx, "does-not-exist")

			// then
			require.NoError(t, err)
			assert.Nil(t, role)
		})
	})

	t.Run("fail for non-existing cluster type", func(t *testing.T) {
		// given
		clusterCtx := newFakeClusterContext(ctx, "fail")

		// when
		restartRole, err := getRole(clusterCtx, "restart-deployment")

		// then
		require.Error(t, err)
		assert.Nil(t, restartRole)
	})
}

func TestEnsureRole(t *testing.T) {
	// given
	files := NewFakeFiles(t,
		FakeTemplate("setup/roles/host.yaml", installOperatorRole),
		FakeTemplate("setup/roles/member.yaml", installOperatorRole, restartDeploymentRole, configureComponentRole, registerClusterRole))

	t.Run("create install-operator role for host", func(t *testing.T) {
		// given
		ctx := newSetupContext(t, &assets.SandboxEnvironmentConfig{}, files)
		hostCtx := newFakeClusterContext(ctx, configuration.Host)
		memberCtx := newFakeClusterContext(ctx, configuration.Member)
		cache := objectsCache{}

		// when
		created, roleName, err := ensureRole(hostCtx, cache, "install-operator", commontest.HostOperatorNs)

		// then
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, "install-operator-host", roleName)
		inObjectCache(t, ctx.outDir, "host", cache).
			assertNumberOfRoles(1).
			assertRole(commontest.HostOperatorNs, roleName, hasSameRulesAs(installOperatorRole))

		t.Run("create install-operator role in another namespace", func(t *testing.T) {
			// when
			created, roleName, err := ensureRole(hostCtx, cache, "install-operator", "monitoring")

			// then
			require.NoError(t, err)
			assert.True(t, created)
			assert.Equal(t, "install-operator-host", roleName)
			inObjectCache(t, ctx.outDir, "host", cache).
				assertNumberOfRoles(2).
				assertRole(commontest.HostOperatorNs, "install-operator-host", hasSameRulesAs(installOperatorRole)).
				assertRole("monitoring", "install-operator-host", hasSameRulesAs(installOperatorRole))

			t.Run("create install-operator role in the same namespace but for member cluster type", func(t *testing.T) {
				// when
				created, roleName, err := ensureRole(memberCtx, cache, "install-operator", "monitoring")

				// then
				require.NoError(t, err)
				assert.True(t, created)
				assert.Equal(t, "install-operator-member", roleName)
				inObjectCache(t, ctx.outDir, "host", cache).
					assertNumberOfRoles(2).
					assertRole(commontest.HostOperatorNs, "install-operator-host", hasSameRulesAs(installOperatorRole)).
					assertRole("monitoring", "install-operator-host", hasSameRulesAs(installOperatorRole))
				inObjectCache(t, ctx.outDir, "member", cache).
					assertNumberOfRoles(1).
					assertRole("monitoring", "install-operator-member", hasSameRulesAs(installOperatorRole))

				t.Run("running for non-existing role fails", func(t *testing.T) {
					// when
					created, _, err := ensureRole(hostCtx, cache, "fail", "failing")

					// then
					require.Error(t, err)
					assert.False(t, created)
					inObjectCache(t, ctx.outDir, "host", cache).
						assertNumberOfRoles(2)
				})
			})
		})
	})

	t.Run("create restart-deployment role for member", func(t *testing.T) {
		// given
		ctx := newSetupContext(t, &assets.SandboxEnvironmentConfig{}, files)
		memberCtx := newFakeClusterContext(ctx, configuration.Member)
		cache := objectsCache{}

		// when
		created, roleName, err := ensureRole(memberCtx, cache, "restart-deployment", commontest.MemberOperatorNs)

		// then
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, "restart-deployment-member", roleName)
		inObjectCache(t, ctx.outDir, "member", cache).
			assertNumberOfRoles(1).
			assertRole(commontest.MemberOperatorNs, "restart-deployment-member", hasSameRulesAs(restartDeploymentRole))

		t.Run("create configure-component role in the same namespace", func(t *testing.T) {
			// when
			created, roleName, err := ensureRole(memberCtx, cache, "configure-component", commontest.MemberOperatorNs)

			// then
			require.NoError(t, err)
			assert.True(t, created)
			assert.Equal(t, "configure-component-member", roleName)
			inObjectCache(t, ctx.outDir, "member", cache).
				assertNumberOfRoles(2).
				assertRole(commontest.MemberOperatorNs, "restart-deployment-member", hasSameRulesAs(restartDeploymentRole)).
				assertRole(commontest.MemberOperatorNs, "configure-component-member", hasSameRulesAs(configureComponentRole))

			t.Run("running for same restart-deployment in the same namespace doesn't have any effect", func(t *testing.T) {
				// when
				created, roleName, err := ensureRole(memberCtx, cache, "restart-deployment", commontest.MemberOperatorNs)

				// then
				require.NoError(t, err)
				assert.True(t, created)
				assert.Equal(t, "restart-deployment-member", roleName)
				inObjectCache(t, ctx.outDir, "member", cache).
					assertNumberOfRoles(2)
			})
		})
	})
}

func newRule(apiGroup, resource, verb string) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{apiGroup},
		Resources: []string{resource},
		Verbs:     []string{verb},
	}
}

func newRole(name string, rules ...rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "Role",
		},
		Rules: rules,
	}
}
