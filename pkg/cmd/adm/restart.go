package adm

import (
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
// waits for the new pods to come up, then uses rollout-restart command for non-olm based - registration-service)
// 2.  If the command is run for member operator, it restart the whole member operator.(it deletes olm based pods(member-operator pods),
// waits for the new pods to come up, then uses rollout-restart command for non-olm based deployments - webhooks)
func NewRestartCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "restart <cluster-name>",
		Short: "Restarts an operator",
		Long: `Restarts the whole operator, it relies on the target cluster and fetches the cluster config
		1.  If the command is run for host operator, it restarts the whole host operator.
		(it deletes olm based pods(host-operator pods),waits for the new pods to 
		come up, then uses rollout-restart command for non-olm based deployments - registration-service)
		2.  If the command is run for member operator, it restarts the whole member operator.
		(it deletes olm based pods(member-operator pods),waits for the new pods 
		to come up, then uses rollout-restart command for non-olm based deployments - webhooks)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return restart(ctx, args...)
		},
	}
	return command
}

func restart(ctx *clicontext.CommandContext, clusterNames ...string) error {
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
	ctx.Printlnf("Fetching the current OLM and non-OLM deployments of the operator in %s namespace", ns)

	operatorDeploymentList, nonOperatorDeploymentList, err := getExistingDeployments(ctx, cl, ns)
	if err != nil {
		return err
	}

	if len(operatorDeploymentList.Items) == 0 {
		return fmt.Errorf("no operator based deployment restart happened as operator deployment found in namespace %s is 0", ns)
	} else {
		for _, operatorDeployment := range operatorDeploymentList.Items {
			ctx.Printlnf("Proceeding to delete the Pods of %v", operatorDeployment.Name)

			if err := deleteAndWaitForPods(ctx, cl, operatorDeployment); err != nil {
				return err
			}

			ctx.Printlnf("Checking the status of the deleted pod's deployment %v", operatorDeployment.Name)
			//check the rollout status
			if err := checkRolloutStatus(ctx, f, ioStreams, "kubesaw-control-plane=kubesaw-controller-manager"); err != nil {
				return err
			}
		}

		if len(nonOperatorDeploymentList.Items) != 0 {
			for _, nonOperatorDeployment := range nonOperatorDeploymentList.Items {
				if nonOperatorDeployment.Name != "autoscaling-buffer" {
					ctx.Printlnf("Proceeding to restart the non-operator deployment %v", nonOperatorDeployment.Name)

					if err := restartNonOlmDeployments(ctx, nonOperatorDeployment, f, ioStreams); err != nil {
						return err
					}
					//check the rollout status
					ctx.Printlnf("Checking the status of the rolled out deployment %v", nonOperatorDeployment.Name)
					if err := checkRolloutStatus(ctx, f, ioStreams, "toolchain.dev.openshift.com/provider=codeready-toolchain"); err != nil {
						return err
					}
				}
			}
		} else {
			ctx.Printlnf("No Non-operator deployment restart happened as Non-Operator deployment found in namespace %s is 0", ns)
		}
	}
	return nil
}

func deleteAndWaitForPods(ctx *clicontext.CommandContext, cl runtimeclient.Client, deployment appsv1.Deployment) error {
	ctx.Printlnf("Listing the pods to be deleted")
	//get pods by label selector from the deployment
	pods := corev1.PodList{}
	selector, _ := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err := cl.List(ctx, &pods,
		runtimeclient.MatchingLabelsSelector{Selector: selector},
		runtimeclient.InNamespace(deployment.Namespace)); err != nil {
		return err
	}
	ctx.Printlnf("Starting to delete the pods")
	//delete pods
	for _, pod := range pods.Items {
		pod := pod // TODO We won't need it after upgrading to go 1.22: https://go.dev/blog/loopvar-preview
		if err := cl.Delete(ctx, &pod); err != nil {
			return err
		}
	}
	return nil

}

func restartNonOlmDeployments(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {

	o := kubectlrollout.NewRolloutRestartOptions(ioStreams)

	if err := o.Complete(f, nil, []string{"deployment"}); err != nil {
		panic(err)
	}

	o.Resources = []string{"deployment/" + deployment.Name}

	if err := o.Validate(); err != nil {
		panic(err)
	}
	ctx.Printlnf("Running the rollout restart command for non-olm deployment %v", deployment.Name)
	return o.RunRestart()
}

func checkRolloutStatus(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
	cmd := kubectlrollout.NewRolloutStatusOptions(ioStreams)

	if err := cmd.Complete(f, []string{"deployment"}); err != nil {
		panic(err)
	}
	cmd.LabelSelector = labelSelector
	if err := cmd.Validate(); err != nil {
		panic(err)
	}
	ctx.Printlnf("Running the Rollout status to check the status of the deployment")
	return cmd.Run()
}

func getExistingDeployments(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string) (*appsv1.DeploymentList, *appsv1.DeploymentList, error) {

	operatorDeployments := &appsv1.DeploymentList{}
	if err := cl.List(ctx, operatorDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"kubesaw-control-plane": "kubesaw-controller-manager"}); err != nil {
		return nil, nil, err
	}

	nonOperatorDeployments := &appsv1.DeploymentList{}
	if err := cl.List(ctx, nonOperatorDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"toolchain.dev.openshift.com/provider": "codeready-toolchain"}); err != nil {
		return nil, nil, err
	}

	return operatorDeployments, nonOperatorDeployments, nil
}
