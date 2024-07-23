package adm

import (
	"context"
	"testing"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/client"
	. "github.com/kubesaw/ksctl/pkg/test"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestInstallOperators(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())

	hostNamespace := "toolchain-host-operator"
	memberNamespace := "toolchain-member-operator"
	hostInstallPlan := olmv1alpha1.InstallPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-ip",
			Namespace: hostNamespace,
		},
		Status: olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanPhaseComplete},
	}
	memberInstallPlan := olmv1alpha1.InstallPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member-ip",
			Namespace: memberNamespace,
		},
		Status: olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanPhaseComplete},
	}
	timeout := 1 * time.Second

	t.Run("install operators is successful", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t, &hostInstallPlan, &memberInstallPlan)
		fakeClientFromRestConfig := fakeClientWithCatalogSource(t, fakeClient, "READY")
		term := NewFakeTerminalWithResponse("Y")
		ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

		// when
		err := installOperators(ctx, installArgs{
			hostKubeConfig:    hostKubeconfig,
			memberKubeConfigs: []string{memberKubeconfig},
			hostNamespace:     hostNamespace,
			memberNamespace:   memberNamespace,
		},
			timeout,
		)

		// then
		require.NoError(t, err)
		AssertCatalogSourceExists(t, fakeClient, types.NamespacedName{Name: "source-host-operator", Namespace: hostNamespace})
		AssertCatalogSourceHasSpec(t, fakeClient, types.NamespacedName{Name: "source-host-operator", Namespace: hostNamespace},
			olmv1alpha1.CatalogSourceSpec{
				SourceType:  olmv1alpha1.SourceTypeGrpc,
				Image:       "quay.io/codeready-toolchain/host-operator-index:latest",
				DisplayName: "Dev Sandbox Operators",
				Publisher:   "Red Hat",
				UpdateStrategy: &olmv1alpha1.UpdateStrategy{
					RegistryPoll: &olmv1alpha1.RegistryPoll{
						Interval: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
				},
			},
		)
		AssertCatalogSourceExists(t, fakeClient, types.NamespacedName{Name: "source-member-operator", Namespace: memberNamespace})
		AssertCatalogSourceHasSpec(t, fakeClient, types.NamespacedName{Name: "source-member-operator", Namespace: memberNamespace},
			olmv1alpha1.CatalogSourceSpec{
				SourceType:  olmv1alpha1.SourceTypeGrpc,
				Image:       "quay.io/codeready-toolchain/member-operator-index:latest",
				DisplayName: "Dev Sandbox Operators",
				Publisher:   "Red Hat",
				UpdateStrategy: &olmv1alpha1.UpdateStrategy{
					RegistryPoll: &olmv1alpha1.RegistryPoll{
						Interval: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
				},
			},
		)
		AssertOperatorGroupExists(t, fakeClient, types.NamespacedName{Name: "og-host-operator", Namespace: hostNamespace})
		AssertOperatorGroupExists(t, fakeClient, types.NamespacedName{Name: "og-member-operator", Namespace: memberNamespace})
		AssertSubscriptionExists(t, fakeClient, types.NamespacedName{Name: "subscription-host-operator", Namespace: hostNamespace})
		AssertSubscriptionExists(t, fakeClient, types.NamespacedName{Name: "subscription-member-operator", Namespace: memberNamespace})
		assert.Contains(t, term.Output(), "InstallPlans for host-operator are ready")
		assert.Contains(t, term.Output(), "InstallPlans for member-operator are ready")
	})

	t.Run("install operators fails if CatalogSource is not ready", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		fakeClientFromRestConfig := func(cfg *rest.Config) (runtimeclient.Client, error) {
			assert.Contains(t, cfg.Host, "http")
			assert.Contains(t, cfg.Host, "://")
			assert.Contains(t, cfg.Host, ".com")
			return fakeClient, nil
		}
		term := NewFakeTerminalWithResponse("Y")
		ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

		// when
		err := installOperators(ctx, installArgs{
			hostKubeConfig:    hostKubeconfig,
			memberKubeConfigs: []string{memberKubeconfig},
			hostNamespace:     hostNamespace,
			memberNamespace:   memberNamespace,
		},
			timeout,
		)

		// then
		require.EqualError(t, err, "timed out waiting for the condition")
		AssertCatalogSourceDoesNotExist(t, fakeClient, types.NamespacedName{Name: "source-member-operator", Namespace: memberNamespace})
		AssertOperatorGroupDoesNotExist(t, fakeClient, types.NamespacedName{Name: "og-host-operator", Namespace: hostNamespace})
		AssertOperatorGroupDoesNotExist(t, fakeClient, types.NamespacedName{Name: "og-member-operator", Namespace: memberNamespace})
		AssertSubscriptionDoesNotExist(t, fakeClient, types.NamespacedName{Name: "subscription-host-operator", Namespace: hostNamespace})
		AssertSubscriptionDoesNotExist(t, fakeClient, types.NamespacedName{Name: "subscription-member-operator", Namespace: memberNamespace})
		assert.NotContains(t, term.Output(), "InstallPlans for host-operator are ready")
		assert.NotContains(t, term.Output(), "InstallPlans for member-operator are ready")
	})

	t.Run("install operators fails if InstallPlan is not ready", func(t *testing.T) {
		// given
		// no InstallPlan is pre provisioned
		fakeClient := test.NewFakeClient(t)
		fakeClientFromRestConfig := fakeClientWithCatalogSource(t, fakeClient, "READY")
		term := NewFakeTerminalWithResponse("Y")
		ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

		// when
		err := installOperators(ctx, installArgs{
			hostKubeConfig:    hostKubeconfig,
			memberKubeConfigs: []string{memberKubeconfig},
			hostNamespace:     hostNamespace,
			memberNamespace:   memberNamespace,
		},
			timeout,
		)

		// then
		require.EqualError(t, err, "timed out waiting for the condition")
		assert.NotContains(t, term.Output(), "InstallPlans for host-operator are ready")
		assert.NotContains(t, term.Output(), "InstallPlans for member-operator are ready")
	})
}

func fakeClientWithCatalogSource(t *testing.T, fakeClient *test.FakeClient, catalogSourceState string) func(cfg *rest.Config) (runtimeclient.Client, error) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		switch obj.(type) {
		case *olmv1alpha1.CatalogSource:
			// let's set the status of the CS to be able to test the "happy path"
			cs := obj.(*olmv1alpha1.CatalogSource)
			cs.Status = olmv1alpha1.CatalogSourceStatus{
				GRPCConnectionState: &olmv1alpha1.GRPCConnectionState{
					LastObservedState: catalogSourceState,
				},
			}
			return fakeClient.Client.Create(ctx, cs)
		default:
			return fakeClient.Client.Create(ctx, obj)
		}
	}
	fakeClientFromRestConfig := func(cfg *rest.Config) (runtimeclient.Client, error) {
		assert.Contains(t, cfg.Host, "http")
		assert.Contains(t, cfg.Host, "://")
		assert.Contains(t, cfg.Host, ".com")
		return fakeClient, nil
	}
	return fakeClientFromRestConfig
}
