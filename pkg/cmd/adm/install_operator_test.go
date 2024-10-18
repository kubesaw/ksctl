package adm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"
	v1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewApplyClient(fakeClient))

			// then
			require.NoError(t, err)
			ns := &corev1.Namespace{}
			require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)) // check that the namespace was created as well
			AssertCatalogSourceExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertCatalogSourceHasSpec(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace},
				olmv1alpha1.CatalogSourceSpec{
					SourceType:  olmv1alpha1.SourceTypeGrpc,
					Image:       fmt.Sprintf("quay.io/codeready-toolchain/%s-operator-index:latest", operator),
					DisplayName: fmt.Sprintf("KubeSaw '%s' Operator", operator),
					Publisher:   "Red Hat",
					UpdateStrategy: &olmv1alpha1.UpdateStrategy{
						RegistryPoll: &olmv1alpha1.RegistryPoll{
							Interval: &metav1.Duration{
								Duration: 5 * time.Minute,
							},
						},
					},
				},
			)
			AssertOperatorGroupExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertSubscriptionExists(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			assert.Contains(t, buffy.String(), fmt.Sprintf("The '%s' operator has been successfully installed in the '%s' namespace", operator, namespace))
		})

		t.Run("install "+operator+" operator fails if CatalogSource is not ready", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewApplyClient(fakeClient))

			// then
			require.ErrorContains(t, err, "failed waiting for catalog source to be ready.")
			AssertOperatorGroupDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			AssertSubscriptionDoesNotExist(t, fakeClient, types.NamespacedName{Name: fmt.Sprintf("%s-operator", operator), Namespace: namespace})
			assert.NotContains(t, buffy.String(), "The '%s' operator has been successfully installed in the '%s' namespace", operator, namespace)
		})

		t.Run("install "+operator+" operators fails if InstallPlan is not ready", func(t *testing.T) {
			// given
			// InstallPlan is pre provisioned but not ready
			notReadyIP := installPlan.DeepCopy()
			notReadyIP.Status = olmv1alpha1.InstallPlanStatus{Phase: olmv1alpha1.InstallPlanFailed}
			fakeClient := test.NewFakeClient(t, notReadyIP)
			fakeClientWithReadyCatalogSource(fakeClient)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewApplyClient(fakeClient))

			// then
			require.ErrorContains(t, err, "failed waiting for install plan to be complete.")
			assert.NotContains(t, buffy.String(), "The '%s' operator has been successfully installed in the '%s' namespace", operator, namespace)
		})

		t.Run(operator+" fails to install if the other operator is installed", func(t *testing.T) {
			// given
			operatorAlreadyInstalled := configuration.ClusterType(operator).TheOtherType().String()
			existingSubscription := olmv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{Name: operatorAlreadyInstalled, Namespace: namespace},
			}
			fakeClient := test.NewFakeClient(t, &existingSubscription)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, installArgs{namespace: namespace, waitForReadyTimeout: 1 * time.Second},
				operator,
				commonclient.NewApplyClient(fakeClient),
			)

			// then
			require.EqualError(t, err, fmt.Sprintf("found existing subscription '%s' in namespace '%s', but host and member operators cannot be installed in the same namespace", operatorAlreadyInstalled, namespace))
		})

		t.Run("skip creation of operator group if already present", func(t *testing.T) {
			// given
			existingOperatorGroup := v1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{Name: operatorResourceName(operator), Namespace: namespace},
			}
			fakeClient := test.NewFakeClient(t, &existingOperatorGroup, &installPlan)
			fakeClientWithReadyCatalogSource(fakeClient)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, args, operator, commonclient.NewApplyClient(fakeClient))

			// then
			require.NoError(t, err)
			assert.Contains(t, buffy.String(), fmt.Sprintf("OperatorGroup '%s' already exists in namespace '%s', skipping creation of the '%s' OperatorGroup.", operatorResourceName(operator), namespace, operatorResourceName(operator)))
			assert.NotContains(t, buffy.String(), fmt.Sprintf("Creating the OperatorGroup '%s' in namespace '%s'.", operatorResourceName(operator), namespace))
			assert.Contains(t, buffy.String(), fmt.Sprintf("The '%s' operator has been successfully installed in the '%s' namespace", operator, namespace))
		})

		t.Run("namespace is computed if not provided", func(t *testing.T) {
			// given
			fakeClient := test.NewFakeClient(t, &installPlan)
			fakeClientWithReadyCatalogSource(fakeClient)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true), ioutils.WithTee(os.Stdout))
			ctx := clicontext.NewTerminalContext(term)

			// when
			err := installOperator(ctx, installArgs{namespace: "", kubeConfig: kubeconfig, waitForReadyTimeout: timeout}, // we provide no namespace
				operator,
				commonclient.NewApplyClient(fakeClient),
			)
			// then
			require.NoError(t, err)
			assert.Contains(t, buffy.String(), fmt.Sprintf("The '%s' operator has been successfully installed in the '%s' namespace", operator, namespace)) // and it's installed in the expected namespace
		})
	}

	t.Run("fails if operator name is invalid", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		fakeClientWithReadyCatalogSource(fakeClient)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy)
		ctx := clicontext.NewTerminalContext(term)

		// when
		err := installOperator(ctx, installArgs{},
			"INVALIDOPERATOR",
			commonclient.NewApplyClient(fakeClient),
		)

		// then
		require.EqualError(t, err, "invalid operator type provided: INVALIDOPERATOR. Valid ones are host|member")
	})

	t.Run("doesn't install operator if response is no", func(t *testing.T) {
		// given
		fakeClient := test.NewFakeClient(t)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
		ctx := clicontext.NewTerminalContext(term)

		// when
		operator := "host"
		err := installOperator(ctx, installArgs{namespace: "toolchain-host-operator", waitForReadyTimeout: time.Second * 1},
			operator,
			commonclient.NewApplyClient(fakeClient),
		)

		// then
		require.NoError(t, err)
		// assert.Contains(t, buffy.String(), fmt.Sprintf("Are you sure that you want to install '%s' in namespace", operator))
		assert.NotContains(t, buffy.String(), fmt.Sprintf("The '%s' operator has been successfully installed in the toolchain-host-operator namespace", operator))
	})
}

func fakeClientWithReadyCatalogSource(fakeClient *test.FakeClient) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		switch objT := obj.(type) {
		case *olmv1alpha1.CatalogSource:
			// let's set the status of the CS to be able to test the "happy path"
			objT.Status = olmv1alpha1.CatalogSourceStatus{
				GRPCConnectionState: &olmv1alpha1.GRPCConnectionState{
					LastObservedState: "READY",
				},
			}
			return fakeClient.Client.Create(ctx, objT)
		default:
			return fakeClient.Client.Create(ctx, objT)
		}
	}
}
