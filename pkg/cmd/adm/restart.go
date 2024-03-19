package adm

import (
	"context"
	"fmt"
	"time"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRestartCmd() *cobra.Command {
	var targetCluster string
	command := &cobra.Command{
		Use:   "restart -t <cluster-name> <deployment-name>",
		Short: "Restarts a deployment",
		Long: `Restarts the deployment with the given name in the operator namespace. 
If no deployment name is provided, then it lists all existing deployments in the namespace.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := &RestartOptions{
				term:      ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout),
				newClient: client.DefaultNewClient,
			}
			return o.restart(cmd.Context(), targetCluster, args...)
		},
	}
	command.Flags().StringVarP(&targetCluster, "target-cluster", "t", "", "The target cluster")
	flags.MustMarkRequired(command, "target-cluster")
	return command
}

type RestartOptions struct {
	term      ioutils.Terminal
	newClient clicontext.NewClientFunc
}

func (o *RestartOptions) restart(ctx context.Context, clusterName string, deployments ...string) error {
	cfg, err := configuration.LoadClusterConfig(o.term, clusterName)
	if err != nil {
		return err
	}
	cl, err := o.newClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	if len(deployments) == 0 {
		err := printExistingDeployments(o.term, cl, cfg)
		if err != nil {
			o.term.Printlnf("\nERROR: Failed to list existing deployments\n :%s", err.Error())
		}
		return fmt.Errorf("at least one deployment name is required, include one or more of the above deployments to restart")
	}
	deploymentName := deployments[0]

	if !o.term.AskForConfirmation(
		ioutils.WithMessagef("restart the deployment '%s' in namespace '%s'", deploymentName, cfg.SandboxNamespace)) {
		return nil
	}
	return restartDeployment(ctx, o.term, cl, cfg, deploymentName)
}

func restartDeployment(ctx context.Context, term ioutils.Terminal, cl runtimeclient.Client, cfg configuration.ClusterConfig, deploymentName string) error {
	namespacedName := types.NamespacedName{
		Namespace: cfg.SandboxNamespace,
		Name:      deploymentName,
	}

	originalReplicas, err := scaleToZero(ctx, cl, namespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			term.Printlnf("\nERROR: The given deployment '%s' wasn't found.", deploymentName)
			return printExistingDeployments(term, cl, cfg)
		}
		return err
	}
	term.Println("The deployment was scaled to 0")
	if err := scaleBack(term, cl, namespacedName, originalReplicas); err != nil {
		term.Printlnf("Scaling the deployment '%s' in namespace '%s' back to '%d' replicas wasn't successful", originalReplicas)
		term.Println("Please, try to contact administrators to scale the deployment back manually")
		return err
	}

	term.Printlnf("The deployment was scaled back to '%d'", originalReplicas)
	return nil
}

func restartHostOperator(ctx context.Context, term ioutils.Terminal, hostClient runtimeclient.Client, hostConfig configuration.ClusterConfig) error {
	deployments := &appsv1.DeploymentList{}
	if err := hostClient.List(context.TODO(), deployments,
		runtimeclient.InNamespace(hostConfig.SandboxNamespace),
		runtimeclient.MatchingLabels{"olm.owner.namespace": "toolchain-host-operator"}); err != nil {
		return err
	}
	if len(deployments.Items) != 1 {
		return fmt.Errorf("there should be a single deployment matching the label olm.owner.namespace=toolchain-host-operator in %s ns, but %d was found. "+
			"It's not possible to restart the Host Operator deployment", hostConfig.SandboxNamespace, len(deployments.Items))
	}

	return restartDeployment(ctx, term, hostClient, hostConfig, deployments.Items[0].Name)
}

func printExistingDeployments(term ioutils.Terminal, cl runtimeclient.Client, cfg configuration.ClusterConfig) error {
	deployments := &appsv1.DeploymentList{}
	if err := cl.List(context.TODO(), deployments, runtimeclient.InNamespace(cfg.SandboxNamespace)); err != nil {
		return err
	}
	deploymentList := "\n"
	for _, deployment := range deployments.Items {
		deploymentList += fmt.Sprintf("%s\n", deployment.Name)
	}
	term.PrintContextSeparatorWithBodyf(deploymentList, "Existing deployments in %s namespace", cfg.SandboxNamespace)
	return nil
}

func scaleToZero(ctx context.Context, cl runtimeclient.Client, namespacedName types.NamespacedName) (int32, error) {

	// get the deployment
	deployment := &appsv1.Deployment{}
	if err := cl.Get(ctx, namespacedName, deployment); err != nil {
		return 0, err
	}
	// keep original number of replicas so we can bring it back
	originalReplicas := *deployment.Spec.Replicas
	zero := int32(0)
	deployment.Spec.Replicas = &zero

	// update the deployment so it scales to zero
	return originalReplicas, cl.Update(ctx, deployment)
}

func scaleBack(term ioutils.Terminal, cl runtimeclient.Client, namespacedName types.NamespacedName, originalReplicas int32) error {
	return wait.Poll(500*time.Millisecond, 10*time.Second, func() (done bool, err error) {
		term.Println("")
		term.Printlnf("Trying to scale the deployment back to '%d'", originalReplicas)
		// get the updated
		deployment := &appsv1.Deployment{}
		if err := cl.Get(context.TODO(), namespacedName, deployment); err != nil {
			return false, err
		}
		// check if the replicas number wasn't already reset by a controller
		if *deployment.Spec.Replicas == originalReplicas {
			return true, nil
		}
		// set the original
		deployment.Spec.Replicas = &originalReplicas
		// and update to scale back
		if err := cl.Update(context.TODO(), deployment); err != nil {
			term.Printlnf("error updating Deployment '%s': %s. Will retry again...", namespacedName.Name, err.Error())
			return false, nil
		}
		return true, nil
	})
}
