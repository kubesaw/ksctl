package adm

import (
	"fmt"
	"os"
	"time"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kubectlrollout "k8s.io/kubectl/pkg/cmd/rollout"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	RolloutRestartFunc             func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error
	RolloutStatusCheckerFunc       func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error
	ConfigFlagsAndClientGetterFunc func(ctx *clicontext.CommandContext, clusterName string) (kubeConfigFlag *genericclioptions.ConfigFlags, rccl runtimeclient.Client, err error)
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
			return restart(ctx, args[0], getConfigFlagsAndClient)
		},
	}
	return command
}

func restart(ctx *clicontext.CommandContext, clusterName string, configFlagsClientGetter ConfigFlagsAndClientGetterFunc) error {
	ioStreams := genericiooptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	kubeConfigFlags, cl, err := configFlagsClientGetter(ctx, clusterName)
	if err != nil {
		return err
	}
	factory := cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(kubeConfigFlags))

	if !ctx.AskForConfirmation(
		ioutils.WithMessagef("restart all the deployments in the cluster  '%s' and namespace '%s' \n", clusterName, *kubeConfigFlags.Namespace)) {
		return nil
	}

	return restartDeployments(ctx, cl, *kubeConfigFlags.Namespace, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
		return checkRolloutStatus(ctx, factory, ioStreams, deployment)
	}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
		return restartNonOlmDeployments(ctx, deployment, factory, ioStreams)
	})
}

func getConfigFlagsAndClient(ctx *clicontext.CommandContext, clusterName string) (kubeConfigFlag *genericclioptions.ConfigFlags, rccl runtimeclient.Client, err error) {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()

	kubeConfigFlags.ClusterName = nil  // `cluster` flag is redefined for our own purpose
	kubeConfigFlags.AuthInfoName = nil // unused here, so we can hide it
	kubeConfigFlags.Context = nil      // unused here, so we can hide it

	cfg, err := configuration.LoadClusterConfig(ctx, clusterName)
	if err != nil {
		return nil, nil, err
	}
	kubeConfigFlags.Namespace = &cfg.OperatorNamespace
	kubeConfigFlags.APIServer = &cfg.ServerAPI
	kubeConfigFlags.BearerToken = &cfg.Token
	kubeconfig, err := client.EnsureKsctlConfigFile()
	if err != nil {
		return nil, nil, err
	}
	kubeConfigFlags.KubeConfig = &kubeconfig

	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return nil, nil, err
	}
	return kubeConfigFlags, cl, nil
}

// This function has the whole logic of getting the list of olm and non-olm based deployment, then proceed on restarting/deleting accordingly
func restartDeployments(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string, checker RolloutStatusCheckerFunc, restarter RolloutRestartFunc) error {

	ctx.Printlnf("Fetching the current OLM and non-OLM deployments of the operator in %s namespace", ns)
	olmDeploymentList, nonOlmDeploymentList, err := getExistingDeployments(ctx, cl, ns)
	if err != nil {
		return err
	}
	//if there is no olm operator deployment, no need for restart
	if len(olmDeploymentList.Items) == 0 {
		return fmt.Errorf("no operator deployment found in namespace %s , it is required for the operator deployment to be running so the command can proceed with restarting the KubeSaw components", ns)
	}
	//Deleting the pods of the olm based operator deployment  and then checking the status
	for _, olmOperatorDeployment := range olmDeploymentList.Items {
		ctx.Printlnf("Proceeding to delete the Pods of %v", olmOperatorDeployment.Name)

		if err := deleteDeploymentPods(ctx, cl, olmOperatorDeployment); err != nil {
			return err
		}
		//sleeping here so that when the status is called we get the correct status
		time.Sleep(1 * time.Second)

		ctx.Printlnf("Checking the status of the deleted pod's deployment %v", olmOperatorDeployment.Name)
		//check the rollout status
		if err := checker(ctx, olmOperatorDeployment); err != nil {
			return err
		}
	}

	//Non-Olm deployments like reg-svc,to be restarted
	//if no Non-OL deployment found it should just return with a message
	if len(nonOlmDeploymentList.Items) == 0 {
		// if there are no non-olm deployments
		ctx.Printlnf("No Non-OLM deployment found in namespace %s, hence no restart happened", ns)
		return nil
	}
	// if there is a Non-olm deployment found use rollout-restart command
	for _, nonOlmDeployment := range nonOlmDeploymentList.Items {
		//it should only use rollout restart for the deployments which are NOT autoscaling-buffer
		if nonOlmDeployment.Name != "autoscaling-buffer" {
			ctx.Printlnf("Proceeding to restart the non-olm deployment %v", nonOlmDeployment.Name)
			//using rollout-restart
			if err := restarter(ctx, nonOlmDeployment); err != nil {
				return err
			}
			//check the rollout status
			ctx.Printlnf("Checking the status of the rolled out deployment %v", nonOlmDeployment.Name)
			if err := checker(ctx, nonOlmDeployment); err != nil {
				return err
			}
			//if the deployment is not auto-scaling buffer, it should return from the function and not go to print the message for autoscaling buffer
			//We do not expect more than 1 non-olm deployment for each OLM deployment and hence returning here
			return nil
		}
		//message if there is a autoscaling buffer, it shouldn't be restarted but successfully exit
		ctx.Printlnf("Found only autoscaling-buffer deployment in namespace %s , which is not required to be restarted", ns)
	}

	return nil
}

func deleteDeploymentPods(ctx *clicontext.CommandContext, cl runtimeclient.Client, deployment appsv1.Deployment) error {
	//get pods by label selector from the deployment
	pods := corev1.PodList{}
	selector, _ := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err := cl.List(ctx, &pods,
		runtimeclient.MatchingLabelsSelector{Selector: selector},
		runtimeclient.InNamespace(deployment.Namespace)); err != nil {
		return err
	}

	//delete pods
	for _, pod := range pods.Items {
		pod := pod // TODO We won't need it after upgrading to go 1.22: https://go.dev/blog/loopvar-preview
		ctx.Printlnf("Deleting pod: %s", pod.Name)
		if err := cl.Delete(ctx, &pod); err != nil {
			return err
		}
	}

	return nil

}

func restartNonOlmDeployments(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {

	o := kubectlrollout.NewRolloutRestartOptions(ioStreams)

	if err := o.Complete(f, nil, []string{"deployment/" + deployment.Name}); err != nil {
		return err
	}

	if err := o.Validate(); err != nil {
		return err
	}
	ctx.Printlnf("Running the rollout restart command for non-Olm deployment %v", deployment.Name)
	return o.RunRestart()
}

func checkRolloutStatus(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, deployment appsv1.Deployment) error {

	cmd := kubectlrollout.NewRolloutStatusOptions(ioStreams)

	if err := cmd.Complete(f, []string{"deployment/" + deployment.Name}); err != nil {
		return err
	}

	if err := cmd.Validate(); err != nil {
		return err
	}
	ctx.Printlnf("Running the Rollout status to check the status of the deployment")
	return cmd.Run()
}

func getExistingDeployments(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string) (*appsv1.DeploymentList, *appsv1.DeploymentList, error) {

	olmDeployments := &appsv1.DeploymentList{}
	if err := cl.List(ctx, olmDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"kubesaw-control-plane": "kubesaw-controller-manager"}); err != nil {
		return nil, nil, err
	}

	nonOlmDeployments := &appsv1.DeploymentList{}
	if err := cl.List(ctx, nonOlmDeployments,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{"toolchain.dev.openshift.com/provider": "codeready-toolchain"}); err != nil {
		return nil, nil, err
	}

	return olmDeployments, nonOlmDeployments, nil
}
