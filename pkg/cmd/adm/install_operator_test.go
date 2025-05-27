package adm

import (
	"context"
	"fmt"
	"testing"
	"time"

	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	corev1 "k8s.io/api/core/v1"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/pkg/utils"
	v1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				Labels: map[string]string{
					fmt.Sprintf("operators.coreos.com/%s.%s", getOperatorName(operator), namespace): "",
				},
			},
			Status: olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanPhaseComplete},
		}
		timeout := 1 * time.Second
		args := installArgs{
			kubeConfig:          kubeconfig,
			namespace:           namespace,
			waitForReadyTimeout: timeout,
		}

		t.Run("install "+operator+" operator is successful", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t, &installPlan)
			fakeClientWithReadyCatalogSource(fakeClient)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager))

			// then
			require.NoError(t, err)
			ns := &corev1.Namespace{}
			require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)) // check that the namespace was created as well
			AssertCatalogSourceExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertCatalogSourceHasSpec(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace},
				olmv1alpha1.CatalogSourceSpec{
					SourceType:  olmv1alpha1.SourceTypeGrpc,
					Image:       fmt.Sprintf("quay.io/codeready-toolchain/%s-operator-index:latest", operator),
					DisplayName: fmt.Sprintf("KubeSaw %s Operator", operator),
					Publisher:   "Red Hat",
					UpdateStrategy: &olmv1alpha1.UpdateStrategy{
						RegistryPoll: &olmv1alpha1.RegistryPoll{
							RawInterval: "5m",
							Interval: &metav1.Duration{
								Duration: 5 * time.Minute,
							},
						},
					},
				},
			)
			AssertOperatorGroupExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertSubscriptionExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			assert.Contains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace))
		})

		t.Run("install "+operator+" operator fails if CatalogSource is not ready", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager))

			// then
			require.ErrorContains(t, err, "failed waiting for catalog source to be ready.")
			AssertOperatorGroupDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertSubscriptionDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			assert.NotContains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace))
		})

		t.Run("install "+operator+" operators fails if InstallPlan is not ready", func(t *testing.T) {
			// given
			// InstallPlan is pre provisioned but not ready
			notReadyIP := installPlan.DeepCopy()
			notReadyIP.Status = olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanFailed}
			fakeClient := test.NewFakeClient(t, notReadyIP)
			fakeClientWithReadyCatalogSource(fakeClient)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager))

			// then
			require.ErrorContains(t, err, "failed waiting for install plan to be complete.")
			assert.NotContains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace))
		})

		t.Run(operator+" fails to install if the other operator is installed", func(t *testing.T) {
			// given
			operatorAlreadyInstalled := configuration.ClusterType(operator).TheOtherType().String()
			existingSubscription := olmv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{Name: operatorAlreadyInstalled, Namespace: namespace},
			}
			fakeClient := test.NewFakeClient(t, &existingSubscription)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, installArgs{namespace: namespace, waitForReadyTimeout: 1 * time.Second},
				operator,
				commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager),
			)

			// then
			require.EqualError(t, err, fmt.Sprintf("found already installed subscription %s in namespace %s - it's not allowed to have host and member in the same namespace", operatorAlreadyInstalled, namespace))
		})

		t.Run("skip creation of operator group if already present", func(t *testing.T) {
			// given
			existingOperatorGroup := v1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{Name: operatorResourceName(operator), Namespace: namespace},
			}
			fakeClient := test.NewFakeClient(t, &existingOperatorGroup, &installPlan)
			fakeClientWithReadyCatalogSource(fakeClient)
			term := NewFakeTerminalWithResponse("y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager))

			// then
			require.NoError(t, err)
			assert.Contains(t, term.Output(), fmt.Sprintf("OperatorGroup %s already present in namespace %s. Skipping creation of new operator group.", operatorResourceName(operator), namespace))
			assert.NotContains(t, term.Output(), fmt.Sprintf("Creating new operator group %s in namespace %s.", operatorResourceName(operator), namespace))
			assert.Contains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace))
		})

		t.Run("namespace is computed if not provided", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t, &installPlan)
			fakeClientWithReadyCatalogSource(fakeClient)
			term := NewFakeTerminalWithResponse("y")
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, installArgs{namespace: "", kubeConfig: kubeconfig, waitForReadyTimeout: timeout}, // we provide no namespace
				operator,
				commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager),
			)
			// then
			require.NoError(t, err)
			assert.Contains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace)) // and it's installed in the expected namespace
		})
	}

	t.Run("fails if operator name is invalid", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		fakeClientWithReadyCatalogSource(fakeClient)
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewTerminalContext(term)

		// when
		err := installOperator(ctx, installArgs{},
			"INVALIDOPERATOR",
			commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager),
		)

		// then
		require.EqualError(t, err, "invalid operator type provided: INVALIDOPERATOR. Valid ones are host|member")
	})

	t.Run("doesn't install operator if response is no", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		term := NewFakeTerminalWithResponse("n")
		ctx := clicontext.NewTerminalContext(term)

		// when
		operator := "host"
		err := installOperator(ctx, installArgs{namespace: "toolchain-host-operator", waitForReadyTimeout: time.Second * 1},
			operator,
			commonclient.NewSSAApplyClient(fakeClient, utils.KsctlFieldManager),
		)

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), fmt.Sprintf("Are you sure that you want to install %s in namespace", operator))
		assert.NotContains(t, term.Output(), fmt.Sprintf("The %s operator has been successfully installed in the toolchain-host-operator namespace", operator))
	})
}

func fakeClientWithReadyCatalogSource(fakeClient *test.FakeClient) {
	fakeClient.MockPatch = func(ctx context.Context, obj runtimeclient.Object, patch runtimeclient.Patch, opts ...runtimeclient.PatchOption) error {
		switch objT := obj.(type) {
		case *olmv1alpha1.CatalogSource:
			// let's set the status of the CS to be able to test the "happy path"
			objT.Status = olmv1alpha1.CatalogSourceStatus{
				GRPCConnectionState: &olmv1alpha1.GRPCConnectionState{
					LastObservedState: "READY",
				},
			}
			return test.Patch(ctx, fakeClient, objT, patch, opts...)
		default:
			return test.Patch(ctx, fakeClient, objT, patch, opts...)
		}
	}
}
