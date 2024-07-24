package adm

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
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
			ctx := newExtendedCommandContext(term, client.DefaultNewClientFromRestConfig)
			return installOperator(ctx, commandArgs, args[0], time.Second*60)
		},
	}

	defaultKubeConfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	cmd.Flags().StringVar(&commandArgs.kubeConfig, "kubeconfig", defaultKubeConfigPath, fmt.Sprintf("Path to the kubeconfig file to use (default: '%s')", defaultKubeConfigPath))
	flags.MustMarkRequired(cmd, "kubeconfig")
	cmd.Flags().StringVar(&commandArgs.namespace, "namespace", "", fmt.Sprintf("The namespace where the operator will be installed "))
	flags.MustMarkRequired(cmd, "namespace")
	return cmd
}

func installOperator(ctx *extendedCommandContext, args installArgs, operator string, timeout time.Duration) error {
	// validate cluster type
	if operator != string(configuration.Host) && operator != string(configuration.Member) {
		return fmt.Errorf("invalid operator type provided: %s. Valid ones are %s|%s", operator, string(configuration.Host), string(configuration.Member))
	}
	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("install %s in namespace '%s'", operator, args.namespace)) {
		return nil
	}

	// install the catalog source
	catalogSourceKey := types.NamespacedName{Name: fmt.Sprintf("source-%s-operator", operator), Namespace: args.namespace}
	catalogSource := newCatalogSource(catalogSourceKey, operator)
	kubeClient, err := newKubeClient(ctx, args.kubeConfig)
	if err != nil {
		return err
	}
	if err := kubeClient.Create(ctx, catalogSource); err != nil {
		return err
	}
	if err := waitUntilCatalogSourceIsReady(ctx.CommandContext, kubeClient, catalogSourceKey, timeout); err != nil {
		return err
	}
	ctx.Printlnf("CatalogSource %s is ready", catalogSourceKey)

	// install operator group
	operatorGroup := newOperatorGroup(types.NamespacedName{Name: fmt.Sprintf("og-%s-operator", operator), Namespace: args.namespace})
	if err := kubeClient.Create(ctx, operatorGroup); err != nil {
		return err
	}

	// install subscription
	subscription := newSubscription(types.NamespacedName{Name: fmt.Sprintf("subscription-%s-operator", operator), Namespace: args.namespace}, fmt.Sprintf("toolchain-%s-operator", operator), catalogSourceKey.Name)
	if err := kubeClient.Create(ctx, subscription); err != nil {
		return err
	}
	if err := waitUntilInstallPlanIsComplete(ctx.CommandContext, kubeClient, args.namespace, timeout); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("InstallPlans for %s-operator are ready", operator))
	return nil
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

func newKubeClient(ctx *extendedCommandContext, kubeConfigPath string) (cl runtimeclient.Client, err error) {
	var kubeConfig *clientcmdapi.Config
	var clientConfig *rest.Config

	kubeConfig, err = clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return
	}
	clientConfig, err = clientcmd.NewDefaultClientConfig(*kubeConfig, nil).ClientConfig()
	if err != nil {
		return
	}
	cl, err = ctx.NewClientFromRestConfig(clientConfig)
	if err != nil {
		return
	}

	return
}

func waitUntilCatalogSourceIsReady(ctx *clicontext.CommandContext, cl runtimeclient.Client, catalogSourceKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for CatalogSource %s to become ready", catalogSourceKey)
		cs := &olmv1alpha1.CatalogSource{}
		if err := cl.Get(ctx, catalogSourceKey, cs); err != nil {
			return false, err
		}

		return cs.Status.GRPCConnectionState != nil && cs.Status.GRPCConnectionState.LastObservedState == "READY", nil
	})
}

func waitUntilInstallPlanIsComplete(ctx *clicontext.CommandContext, cl runtimeclient.Client, namespace string, waitForReadyTimeout time.Duration) error {
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
