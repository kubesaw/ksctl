package adm

import (
	"context"
	"fmt"
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

func TestInstallOperator(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	SetFileConfig(t, Host(), Member())

	for _, operator := range []string{"host", "member"} {

		kubeconfig, namespace := "", ""
		if operator == "host" {
			kubeconfig = PersistKubeConfigFile(t, HostKubeConfig())
			namespace = "toolchain-host-operator"
		} else {
			kubeconfig = PersistKubeConfigFile(t, MemberKubeConfig())
			namespace = "toolchain-member-operator"
		}
		installPlan := olmv1alpha1.InstallPlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator + "-ip",
				Namespace: namespace,
			},
			Status: olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanPhaseComplete},
		}

		timeout := 1 * time.Second

		t.Run("install "+operator+" operator is successful", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t, &installPlan)
			fakeClientFromRestConfig := fakeClientWithCatalogSource(t, fakeClient, "READY")
			term := NewFakeTerminalWithResponse("Y")
			ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

			// when
			err := installOperator(ctx, installArgs{
				kubeConfig: kubeconfig,
				namespace:  namespace,
			},
				operator,
				timeout,
			)

			// then
			require.NoError(t, err)
			AssertCatalogSourceExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("source-%s-operator", operator), Namespace: namespace})
			AssertCatalogSourceHasSpec(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("source-%s-operator", operator), Namespace: namespace},
				olmv1alpha1.CatalogSourceSpec{
					SourceType:  olmv1alpha1.SourceTypeGrpc,
					Image:       fmt.Sprintf("quay.io/codeready-toolchain/%s-operator-index:latest", operator),
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
			AssertOperatorGroupExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("og-%s-operator", operator), Namespace: namespace})
			AssertSubscriptionExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("subscription-%s-operator", operator), Namespace: namespace})
			assert.Contains(t, term.Output(), fmt.Sprintf("InstallPlans for %s-operator are ready", operator))
		})

		t.Run("install "+operator+" operator fails if CatalogSource is not ready", func(t *testing.T) {
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
			err := installOperator(ctx, installArgs{
				kubeConfig: kubeconfig,
				namespace:  namespace,
			},
				operator,
				timeout,
			)

			// then
			require.EqualError(t, err, "timed out waiting for the condition")
			AssertOperatorGroupDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("og-%s-operator", operator), Namespace: namespace})
			AssertSubscriptionDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("subscription-%s-operator", operator), Namespace: namespace})
			assert.NotContains(t, term.Output(), fmt.Sprintf("InstallPlans for %s-operator are ready", operator))
		})

		t.Run("install "+operator+" operators fails if InstallPlan is not ready", func(t *testing.T) {
			// given
			// no InstallPlan is pre provisioned
			fakeClient := test.NewFakeClient(t)
			fakeClientFromRestConfig := fakeClientWithCatalogSource(t, fakeClient, "READY")
			term := NewFakeTerminalWithResponse("Y")
			ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

			// when
			err := installOperator(ctx, installArgs{
				kubeConfig: kubeconfig,
				namespace:  namespace,
			},
				operator,
				timeout,
			)

			// then
			require.EqualError(t, err, "timed out waiting for the condition")
			assert.NotContains(t, term.Output(), fmt.Sprintf("InstallPlans for %s-operator are ready", operator))
		})
	}

	t.Run("fails if operator name is invalid", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		fakeClientFromRestConfig := fakeClientWithCatalogSource(t, fakeClient, "READY")
		term := NewFakeTerminalWithResponse("Y")
		ctx := newExtendedCommandContext(term, fakeClientFromRestConfig)

		// when
		err := installOperator(ctx, installArgs{},
			"INVALIDOPERATOR",
			1*time.Second,
		)

		// then
		require.EqualError(t, err, "invalid operator type provided: INVALIDOPERATOR. Valid ones are host|member")
	})
}

func fakeClientWithCatalogSource(t *testing.T, fakeClient *test.FakeClient, catalogSourceState string) func(cfg *rest.Config) (runtimeclient.Client, error) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		switch objT := obj.(type) {
		case *olmv1alpha1.CatalogSource:
			// let's set the status of the CS to be able to test the "happy path"
			objT.Status = olmv1alpha1.CatalogSourceStatus{
				GRPCConnectionState: &olmv1alpha1.GRPCConnectionState{
					LastObservedState: catalogSourceState,
				},
			}
			return fakeClient.Client.Create(ctx, objT)
		default:
			return fakeClient.Client.Create(ctx, objT)
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
