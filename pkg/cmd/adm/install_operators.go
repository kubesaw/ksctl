package adm

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kubesaw/ksctl/pkg/client"
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
	hostKubeConfig    string
	memberKubeConfigs []string
	hostNamespace     string
	memberNamespace   string
}

func NewInstallOperatorsCmd() *cobra.Command {
	commandArgs := installArgs{}
	cmd := &cobra.Command{
		Use:   "install-operators",
		Short: "install kubesaw operators",
		Long:  `This command installs the latest stable versions of the kubesaw operators using OLM`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := newExtendedCommandContext(term, client.DefaultNewClientFromRestConfig)
			return installOperators(ctx, commandArgs, time.Second*60)
		},
	}

	defaultKubeConfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	// keep these values in sync with the values in defaultRegisterMemberArgs() function in the tests.
	defaultHostKubeConfig := defaultKubeConfigPath
	defaultMemberKubeConfig := defaultKubeConfigPath
	defaultHostNs := "toolchain-host-operator"
	defaultMemberNs := "toolchain-member-operator"

	cmd.Flags().StringVar(&commandArgs.hostKubeConfig, "host-kubeconfig", defaultKubeConfigPath, fmt.Sprintf("Path to the kubeconfig file of the host cluster (default: '%s')", defaultHostKubeConfig))
	cmd.Flags().StringSliceVarP(&commandArgs.memberKubeConfigs, "member-kubeconfigs", "m", []string{defaultMemberKubeConfig}, "Kubeconfig(s) for managing multiple member clusters and the access to them - paths should be comma separated when using multiple of them. "+
		"In dev mode, the first one has to represent the host cluster.")
	cmd.Flags().StringVar(&commandArgs.hostNamespace, "host-ns", defaultHostNs, fmt.Sprintf("The namespace of the host operator in the host cluster (default: '%s')", defaultHostNs))
	cmd.Flags().StringVar(&commandArgs.memberNamespace, "member-ns", defaultMemberNs, fmt.Sprintf("The namespace of the member operator in the member cluster (default: '%s')", defaultMemberNs))
	return cmd
}

func installOperators(ctx *extendedCommandContext, args installArgs, timeout time.Duration) error {

	if err := installOperator(ctx, args.hostKubeConfig, args.hostNamespace, configuration.Host, timeout); err != nil {
		return err
	}

	// install member operators
	for _, memberKubeConfig := range args.memberKubeConfigs {
		if err := installOperator(ctx, memberKubeConfig, args.memberNamespace, configuration.Member, timeout); err != nil {
			return err
		}
	}

	return nil
}

func installOperator(ctx *extendedCommandContext, kubeConfig, namespace string, clusterType configuration.ClusterType, timeout time.Duration) error {
	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("install %s in namespace '%s'", string(clusterType), namespace)) {
		return nil
	}

	// install the catalog source
	catalogSourceKey := types.NamespacedName{Name: fmt.Sprintf("source-%s-operator", string(clusterType)), Namespace: namespace}
	catalogSource := newCatalogSource(catalogSourceKey, clusterType)
	kubeClient, err := newKubeClient(ctx, kubeConfig)
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
	operatorGroup := newOperatorGroup(types.NamespacedName{Name: fmt.Sprintf("og-%s-operator", string(clusterType)), Namespace: namespace})
	if err := kubeClient.Create(ctx, operatorGroup); err != nil {
		return err
	}

	// install subscription
	subscription := newSubscription(types.NamespacedName{Name: fmt.Sprintf("subscription-%s-operator", string(clusterType)), Namespace: namespace}, fmt.Sprintf("toolchain-%s-operator", string(clusterType)), catalogSourceKey.Name)
	if err := kubeClient.Create(ctx, subscription); err != nil {
		return err
	}
	if err := waitUntilInstallPlanIsComplete(ctx.CommandContext, kubeClient, namespace, timeout); err != nil {
		return err
	}
	ctx.Println(fmt.Sprintf("InstallPlans for %s-operator are ready", string(clusterType)))
	return nil
}

func newCatalogSource(name types.NamespacedName, clusterType configuration.ClusterType) *olmv1alpha1.CatalogSource {
	return &olmv1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: olmv1alpha1.CatalogSourceSpec{
			SourceType:  olmv1alpha1.SourceTypeGrpc,
			Image:       fmt.Sprintf("quay.io/codeready-toolchain/%s-operator-index:latest", string(clusterType)),
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
