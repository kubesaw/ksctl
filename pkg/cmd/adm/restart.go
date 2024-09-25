package adm

import (
	"context"
	"fmt"
	"os"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectlrollout "k8s.io/kubectl/pkg/cmd/rollout"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewRestartCmd() is a function to restart the whole operator, it relies on the target cluster and fetches the cluster config
// 1.  If the command is run for host operator, it restart the whole host operator.(it deletes olm based pods(host-operator pods),
// waits for the new deployment to come up, then uses rollout-restart command for non-olm based - registration-service)
// 2.  If the command is run for member operator, it restart the whole member operator.(it deletes olm based pods(member-operator pods),
// waits for the new deployment to come up, then uses rollout-restart command for non-olm based deployments - webhooks)
func NewRestartCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "restart <cluster-name>",
		Short: "Restarts an operator",
		Long: `Restarts the whole operator in the given cluster name. 
		It restarts the operator and checks the status of the deployment`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return restart(ctx, args...)
		},
	}
	return command
}

func restart(ctx *clicontext.CommandContext, clusterNames ...string) error {
	if clusterNames == nil || len(clusterNames) != 1 {
		return fmt.Errorf("please provide 1 cluster name to restart the operator e.g `ksctl adm restart host`")
	}
	clusterName := clusterNames[0]
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	factory := cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(kubeConfigFlags))
	ioStreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	kubeConfigFlags.ClusterName = nil  // `cluster` flag is redefined for our own purpose
	kubeConfigFlags.AuthInfoName = nil // unused here, so we can hide it
	kubeConfigFlags.Context = nil      // unused here, so we can hide it

	cfg, err := configuration.LoadClusterConfig(ctx, clusterName)
	if err != nil {
		return err
	}
	kubeConfigFlags.Namespace = &cfg.OperatorNamespace
	kubeConfigFlags.APIServer = &cfg.ServerAPI
	kubeConfigFlags.BearerToken = &cfg.Token
	kubeconfig, err := client.EnsureKsctlConfigFile()
	if err != nil {
		return err
	}
	kubeConfigFlags.KubeConfig = &kubeconfig

	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)

	if err != nil {
		return err
	}

	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("restart all the deployments in the cluster  '%s' and namespace '%s' \n", clusterName, cfg.OperatorNamespace)) {
		return nil
	}

	return restartDeployment(ctx, cl, cfg.OperatorNamespace, factory, ioStreams)
}

func restartDeployment(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
	fmt.Printf("Fetching the current OLM and non-OLM deployments of the operator in %s \n", ns)

	olmDeploymentList, nonOlmDeploymentlist, err := getExistingDeployments(cl, ns)
	if err != nil {
		return err
	}

	if len(olmDeploymentList.Items) == 0 {
		return fmt.Errorf("OLM based deployment not found in %s", ns)
	} else {
		for _, olmDeployment := range olmDeploymentList.Items {
			fmt.Printf("Proceeding to delete the Pods of %v \n", olmDeployment)

			if err := deleteAndWaitForPods(ctx, cl, olmDeployment, f, ioStreams); err != nil {
				return err
			}
		}
	}
	if len(nonOlmDeploymentlist.Items) != 0 {
		for _, nonOlmDeployment := range nonOlmDeploymentlist.Items {

			fmt.Printf("Proceeding to restart the non-OLM deployment %v \n", nonOlmDeployment)

			if err := restartNonOlmDeployments(nonOlmDeployment, f, ioStreams); err != nil {
				return err
			}
			//check the rollout status
			fmt.Printf("Checking the status of the rolled out deployment %v \n", nonOlmDeployment)
			if err := checkRolloutStatus(f, ioStreams, "provider=codeready-toolchain"); err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("non-OLM based deployment not found in %s \n", ns)
	}
	return nil
}

func deleteAndWaitForPods(ctx *clicontext.CommandContext, cl runtimeclient.Client, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
	fmt.Printf("Listing the pods to be deleted \n")
	//get pods by label selector from the deployment
	pods := corev1.PodList{}
	selector, _ := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err := cl.List(ctx, &pods,
		runtimeclient.MatchingLabelsSelector{Selector: selector},
		runtimeclient.InNamespace(deployment.Namespace)); err != nil {
		return err
	}
	fmt.Printf("Starting to delete the pods \n")
	//delete pods
	for _, pod := range pods.Items {
		pod := pod // TODO We won't need it after upgrading to go 1.22: https://go.dev/blog/loopvar-preview
		if err := cl.Delete(ctx, &pod); err != nil {
			return err
		}

		fmt.Printf("Checking the status of the rolled out deployment %v", deployment)
		//check the rollout status
		if err := checkRolloutStatus(f, ioStreams, "kubesaw-control-plane=kubesaw-controller-manager"); err != nil {
			return err
		}
	}
	return nil

}

func restartNonOlmDeployments(deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {

	o := kubectlrollout.NewRolloutRestartOptions(ioStreams)

	if err := o.Complete(f, nil, []string{"deployment"}); err != nil {
		panic(err)
	}

	o.Resources = []string{"deployment/" + deployment.Name}

	if err := o.Validate(); err != nil {
		panic(err)
	}
	fmt.Printf("Running the rollout restart command for non-olm deployment %v", deployment)
	return o.RunRestart()
}

func checkRolloutStatus(f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
	cmd := kubectlrollout.NewRolloutStatusOptions(ioStreams)

	if err := cmd.Complete(f, []string{"deployment"}); err != nil {
		panic(err)
	}
	cmd.LabelSelector = labelSelector
	if err := cmd.Validate(); err != nil {
		panic(err)
	}
	fmt.Printf("Running the Rollout status to check the status of the deployment")
	return cmd.Run()
}

func getExistingDeployments(cl runtimeclient.Client, ns string) (*appsv1.DeploymentList, *appsv1.DeploymentList, error) {

	olmDeployments := &appsv1.DeploymentList{}
	if err := cl.List(context.TODO(), olmDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"kubesaw-control-plane": "kubesaw-controller-manager"}); err != nil {
		return nil, nil, err
	}

	nonOlmDeployments := &appsv1.DeploymentList{}
	if err := cl.List(context.TODO(), nonOlmDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"provider": "codeready-toolchain"}); err != nil {
		return nil, nil, err
	}

	return olmDeployments, nonOlmDeployments, nil
}
