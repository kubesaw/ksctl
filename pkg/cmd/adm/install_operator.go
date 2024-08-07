package adm

import (
	"fmt"
	"time"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/cobra"
)

type installArgs struct {
	kubeConfig string
	namespace  string
}

func NewInstallOperatorCmd() *cobra.Command {
	commandArgs := installArgs{}
	cmd := &cobra.Command{
		Use:   "install-operator <host|member> --kubeconfig <path/to/kubeconfig> --namespace <namespace>",
		Short: "install kubesaw operator (host|member)",
		Long:  `This command installs the latest stable versions of the kubesaw operator using OLM`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			kubeClient, err := client.NewKubeClientFromKubeConfig(commandArgs.kubeConfig)
			if err != nil {
				return err
			}
			ctx := clicontext.NewTerminalContext(term, kubeClient)
			return installOperator(ctx, commandArgs, args[0], time.Second*60)
		},
	}

	cmd.Flags().StringVar(&commandArgs.kubeConfig, "kubeconfig", "", "Path to the kubeconfig file to use.")
	flags.MustMarkRequired(cmd, "kubeconfig")
	cmd.Flags().StringVar(&commandArgs.namespace, "namespace", "", "The namespace where the operator will be installed. Host and Member should be installed in separate namespaces")
	flags.MustMarkRequired(cmd, "namespace")
	return cmd
}

func installOperator(ctx *clicontext.TerminalContext, args installArgs, operator string, timeout time.Duration) error {
	// validate cluster type
	if operator != string(configuration.Host) && operator != string(configuration.Member) {
		return fmt.Errorf("invalid operator type provided: %s. Valid ones are %s|%s", operator, string(configuration.Host), string(configuration.Member))
	}

	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("install %s in namespace '%s'", operator, args.namespace)) {
		return nil
	}

	// check that we don't install both host and member in the same namespace
	if err := checkOneOperatorPerNamespace(ctx, args.namespace, operator); err != nil {
		return err
	}

	// install the catalog source
	catalogSourceKey := types.NamespacedName{Name: operatorResourceName(operator), Namespace: args.namespace}
	catalogSource := newCatalogSource(catalogSourceKey, operator)
	if err := ctx.KubeClient.Create(ctx, catalogSource); err != nil {
		return err
	}
	if err := waitUntilCatalogSourceIsReady(ctx, catalogSourceKey, timeout); err != nil {
		return err
	}
	ctx.Printlnf("CatalogSource %s is ready", catalogSourceKey)

	// check if operator group is already there
	ogs := olmv1.OperatorGroupList{}
	if err := ctx.KubeClient.List(ctx, &ogs, runtimeclient.InNamespace(args.namespace)); err != nil {
		return err
	}
	if len(ogs.Items) > 0 {
		ctx.Println(fmt.Sprintf("OperatorGroup %s already present in namespace %s. Skipping creation of new operator group.", ogs.Items[0].GetName(), args.namespace))
	} else {
		// install operator group
		operatorGroup := newOperatorGroup(types.NamespacedName{Name: operatorResourceName(operator), Namespace: args.namespace})
		ctx.Println(fmt.Sprintf("Creating new operator group %s in namespace %s.", operatorGroup.Name, operatorGroup.Namespace))
		if err := ctx.KubeClient.Create(ctx, operatorGroup); err != nil {
			return err
		}
	}

	// install subscription
	subscription := newSubscription(types.NamespacedName{Name: operatorResourceName(operator), Namespace: args.namespace}, fmt.Sprintf("toolchain-%s-operator", operator), catalogSourceKey.Name)
	if err := ctx.KubeClient.Create(ctx, subscription); err != nil {
		return err
	}
	if err := waitUntilInstallPlanIsComplete(ctx, ctx.KubeClient, args.namespace, timeout); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("InstallPlan for the %s operator has been completed", operator))
	ctx.Println("")
	ctx.Println(fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, args.namespace))
	return nil
}

func operatorResourceName(operator string) string {
	return fmt.Sprintf("%s-operator", operator)
}

func newCatalogSource(name types.NamespacedName, operator string) *olmv1alpha1.CatalogSource {
	return &olmv1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: olmv1alpha1.CatalogSourceSpec{
			SourceType:  olmv1alpha1.SourceTypeGrpc,
			Image:       fmt.Sprintf("quay.io/codeready-toolchain/%s-operator-index:latest", operator),
			DisplayName: fmt.Sprintf("KubeSaw %s Operator", operator),
			Publisher:   "Red Hat",
			UpdateStrategy: &olmv1alpha1.UpdateStrategy{
				RegistryPoll: &olmv1alpha1.RegistryPoll{
					Interval: &metav1.Duration{
						Duration: 5 * time.Minute,
					},
				},
			},
		},
	}
}

func newOperatorGroup(name types.NamespacedName) *olmv1.OperatorGroup {
	return &olmv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: olmv1.OperatorGroupSpec{
			TargetNamespaces: []string{
				name.Namespace,
			},
		},
	}
}

func newSubscription(name types.NamespacedName, operatorName, catalogSourceName string) *olmv1alpha1.Subscription {
	return &olmv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: &olmv1alpha1.SubscriptionSpec{
			Channel:                "staging",
			InstallPlanApproval:    olmv1alpha1.ApprovalAutomatic,
			Package:                operatorName,
			CatalogSource:          catalogSourceName,
			CatalogSourceNamespace: name.Namespace,
		},
	}
}

func waitUntilCatalogSourceIsReady(ctx *clicontext.TerminalContext, catalogSourceKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for CatalogSource %s to become ready", catalogSourceKey)
		cs := &olmv1alpha1.CatalogSource{}
		if err := ctx.KubeClient.Get(ctx, catalogSourceKey, cs); err != nil {
			return false, err
		}

		return cs.Status.GRPCConnectionState != nil && cs.Status.GRPCConnectionState.LastObservedState == "READY", nil
	})
}

func waitUntilInstallPlanIsComplete(ctx *clicontext.TerminalContext, cl runtimeclient.Client, namespace string, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for InstallPlans in namespace %s to complete", namespace)
		plans := &olmv1alpha1.InstallPlanList{}
		if err := cl.List(ctx, plans, runtimeclient.InNamespace(namespace)); err != nil {
			return false, err
		}

		for _, ip := range plans.Items {
			if ip.Status.Phase != olmv1alpha1.InstallPlanPhaseComplete {
				return false, nil
			}
		}

		return len(plans.Items) > 0, nil
	})
}

// checkOneOperatorPerNamespace returns an error in case the namespace contains the other operator installed.
// So for host namespace member operator should not be installed in there and vice-versa.
func checkOneOperatorPerNamespace(ctx *clicontext.TerminalContext, namespace, operator string) error {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      getOtherOperator(operator),
	}
	subscription := olmv1alpha1.Subscription{}
	if err := ctx.KubeClient.Get(ctx.Context, namespacedName, &subscription); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}
	return fmt.Errorf("found already installed subscription %s in namespace %s", subscription.GetName(), subscription.GetNamespace())
}

func getOtherOperator(operator string) string {
	operatorToCheck := map[string]string{
		string(configuration.Host):   string(configuration.Member),
		string(configuration.Member): string(configuration.Host),
	}
	return operatorToCheck[operator]
}
