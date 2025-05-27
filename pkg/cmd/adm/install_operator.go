package adm

import (
	"context"
	"fmt"
	"time"

	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/pkg/utils"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/cobra"
)

type installArgs struct {
	kubeConfig          string
	namespace           string
	waitForReadyTimeout time.Duration
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

			cl := commonclient.NewSSAApplyClient(kubeClient, utils.KsctlFieldManager)
			ctx := clicontext.NewTerminalContext(term)
			return installOperator(ctx, commandArgs, args[0], cl)
		},
	}

	cmd.Flags().StringVar(&commandArgs.kubeConfig, "kubeconfig", "", "Path to the kubeconfig file to use.")
	flags.MustMarkRequired(cmd, "kubeconfig")
	cmd.Flags().StringVar(&commandArgs.namespace, "namespace", "", "The namespace where the operator will be installed. Host and Member should be installed in separate namespaces. If the namespace is not provided the standard namespace names are used: toolchain-host|member-operator.")
	cmd.Flags().DurationVar(&commandArgs.waitForReadyTimeout, "timeout", time.Second*180, "The max timeout used when waiting for each of the resources to be installed.")
	return cmd
}

func installOperator(ctx *clicontext.TerminalContext, args installArgs, operator string, applyClient *commonclient.SSAApplyClient) error {
	// validate cluster type
	if operator != string(configuration.Host) && operator != string(configuration.Member) {
		return fmt.Errorf("invalid operator type provided: %s. Valid ones are %s|%s", operator, configuration.Host, configuration.Member)
	}

	// assume "standard" namespace if not provided
	namespace := args.namespace
	if args.namespace == "" {
		namespace = fmt.Sprintf("toolchain-%s-operator", operator)
	}

	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("install %s in namespace '%s'", operator, namespace)) {
		return nil
	}

	// check if namespace exists
	// otherwise create it
	if err := createNamespaceIfNotFound(ctx, applyClient, namespace); err != nil {
		return err
	}

	// check that we don't install both host and member in the same namespace
	if err := checkOneOperatorPerNamespace(ctx, applyClient.Client, namespace, operator); err != nil {
		return err
	}

	// install the catalog source
	namespacedName := types.NamespacedName{Name: operatorResourceName(operator), Namespace: namespace}
	catalogSource := newCatalogSource(namespacedName, operator)
	ctx.Println(fmt.Sprintf("Creating CatalogSource %s in namespace %s.", catalogSource.Name, catalogSource.Namespace))
	if err := applyClient.ApplyObject(ctx.Context, catalogSource); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("CatalogSource %s created.", catalogSource.Name))
	if err := waitUntilCatalogSourceIsReady(ctx, applyClient.Client, namespacedName, args.waitForReadyTimeout); err != nil {
		return err
	}
	ctx.Printlnf("CatalogSource %s is ready", namespacedName)

	// check if operator group is already there
	ogs := olmv1.OperatorGroupList{}
	if err := applyClient.Client.List(ctx, &ogs, runtimeclient.InNamespace(namespace)); err != nil {
		return err
	}
	if len(ogs.Items) > 0 {
		ctx.Println(fmt.Sprintf("OperatorGroup %s already present in namespace %s. Skipping creation of new operator group.", ogs.Items[0].GetName(), namespace))
	} else {
		// install operator group
		operatorGroup := newOperatorGroup(namespacedName)
		ctx.Println(fmt.Sprintf("Creating new operator group %s in namespace %s.", operatorGroup.Name, operatorGroup.Namespace))
		if err := applyClient.ApplyObject(ctx, operatorGroup); err != nil {
			return err
		}
		ctx.Println(fmt.Sprintf("OperatorGroup %s created.", operatorGroup.Name))
	}

	// install subscription
	operatorName := getOperatorName(operator)
	subscription := newSubscription(namespacedName, operatorName, namespacedName.Name)
	ctx.Println(fmt.Sprintf("Creating Subscription %s in namespace %s.", subscription.Name, subscription.Namespace))
	if err := applyClient.ApplyObject(ctx, subscription); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("Subcription %s created.", subscription.Name))
	if err := waitUntilInstallPlanIsComplete(ctx, applyClient.Client, operatorName, namespace, args.waitForReadyTimeout); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("InstallPlan for the %s operator has been completed", operator))
	ctx.Println("")
	ctx.Println(fmt.Sprintf("The %s operator has been successfully installed in the %s namespace", operator, namespace))
	return nil
}

func getOperatorName(operator string) string {
	return fmt.Sprintf("toolchain-%s-operator", operator)
}

func createNamespaceIfNotFound(ctx *clicontext.TerminalContext, applyClient *commonclient.SSAApplyClient, namespace string) error {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := applyClient.ApplyObject(ctx.Context, ns); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("Ensured namespace %s.", namespace))
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
					RawInterval: "5m",
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

func waitUntilCatalogSourceIsReady(ctx *clicontext.TerminalContext, cl runtimeclient.Client, catalogSourceKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	cs := &olmv1alpha1.CatalogSource{}
	if err := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, waitForReadyTimeout, true, func(ctx2 context.Context) (bool, error) {
		ctx.Printlnf("waiting for CatalogSource %s to become ready", catalogSourceKey)
		cs = &olmv1alpha1.CatalogSource{}
		if err := cl.Get(ctx, catalogSourceKey, cs); err != nil {
			return false, err
		}

		return cs.Status.GRPCConnectionState != nil && cs.Status.GRPCConnectionState.LastObservedState == "READY", nil
	}); err != nil {
		csString, _ := json.Marshal(cs)
		return fmt.Errorf("failed waiting for catalog source to be ready.\n CatalogSource found: %v \n\t", string(csString))
	}
	return nil
}

func waitUntilInstallPlanIsComplete(ctx *clicontext.TerminalContext, cl runtimeclient.Client, operator, namespace string, waitForReadyTimeout time.Duration) error {
	plans := &olmv1alpha1.InstallPlanList{}
	if err := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, waitForReadyTimeout, true, func(ctx2 context.Context) (bool, error) {
		ctx.Printlnf("waiting for InstallPlans in namespace %s to complete", namespace)
		plans = &olmv1alpha1.InstallPlanList{}
		if err := cl.List(ctx, plans, runtimeclient.InNamespace(namespace),
			runtimeclient.MatchingLabels{fmt.Sprintf("operators.coreos.com/%s.%s", operator, namespace): ""},
		); err != nil {
			return false, err
		}

		for _, ip := range plans.Items {
			if ip.Status.Phase != olmv1alpha1.InstallPlanPhaseComplete {
				return false, nil
			}
		}

		return len(plans.Items) > 0, nil
	}); err != nil {
		plansString, _ := json.Marshal(plans)
		return fmt.Errorf("failed waiting for install plan to be complete.\n InstallPlans found: %s \n\t", string(plansString))
	}
	return nil
}

// checkOneOperatorPerNamespace returns an error in case the namespace contains the other operator installed.
// So for host namespace member operator should not be installed in there and vice-versa.
func checkOneOperatorPerNamespace(ctx *clicontext.TerminalContext, cl runtimeclient.Client, namespace, operator string) error {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      configuration.ClusterType(operator).TheOtherType().String(),
	}
	subscription := olmv1alpha1.Subscription{}
	if err := cl.Get(ctx.Context, namespacedName, &subscription); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}
	return fmt.Errorf("found already installed subscription %s in namespace %s - it's not allowed to have host and member in the same namespace", subscription.GetName(), subscription.GetNamespace())
}
