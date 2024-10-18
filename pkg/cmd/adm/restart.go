package adm

import (
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
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return restart(ctx, targetCluster, args...)
		},
	}
	command.Flags().StringVarP(&targetCluster, "target-cluster", "t", "", "The target cluster")
	flags.MustMarkRequired(command, "target-cluster")
	return command
}

func restart(ctx *clicontext.CommandContext, clusterName string, deployments ...string) error {
	cfg, err := configuration.LoadClusterConfig(ctx.Logger, clusterName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	if len(deployments) == 0 {
		err := printExistingDeployments(ctx, cl, cfg.OperatorNamespace)
		if err != nil {
			ctx.Errorf("Failed to list existing deployments\n :%s", err.Error())
		}
		return fmt.Errorf("at least one deployment name is required, include one or more of the above deployments to restart")
	}
	deploymentName := deployments[0]

	if confirm, err := ctx.Confirm("Restart the '%s' Deployment in the '%s' namespace", deploymentName, cfg.OperatorNamespace); err != nil || !confirm {
		return err
	}
	return restartDeployment(ctx, cl, cfg.OperatorNamespace, deploymentName)
}

func restartDeployment(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string, deploymentName string) error {
	namespacedName := types.NamespacedName{
		Namespace: ns,
		Name:      deploymentName,
	}

	originalReplicas, err := scaleToZero(ctx, cl, namespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Errorf("The '%s' Deployment could not be found", deploymentName)
			return printExistingDeployments(ctx, cl, ns)
		}
		return err
	}
	ctx.Info("The deployment was scaled to 0")
	if err := scaleBack(ctx, cl, namespacedName, originalReplicas); err != nil {
		ctx.Infof("Scaling the deployment '%s' in namespace '%s' back to '%d' replicas wasn't successful", namespacedName.Name, namespacedName.Namespace, int(originalReplicas))
		ctx.Info("Please, try to contact administrators to scale the deployment back manually")
		return err
	}

	ctx.Infof("The deployment was scaled back to '%d'", originalReplicas)
	return nil
}

func restartHostOperator(ctx *clicontext.CommandContext, hostClient runtimeclient.Client, hostNamespace string) error {
	deployments := &appsv1.DeploymentList{}
	if err := hostClient.List(ctx.Context, deployments,
		runtimeclient.InNamespace(hostNamespace),
		runtimeclient.MatchingLabels{"olm.owner.namespace": "toolchain-host-operator"}); err != nil {
		return err
	}
	if len(deployments.Items) != 1 {
		return fmt.Errorf("there should be a single deployment matching the label olm.owner.namespace=toolchain-host-operator in %s ns, but %d was found. "+
			"It's not possible to restart the Host Operator deployment", hostNamespace, len(deployments.Items))
	}

	return restartDeployment(ctx, hostClient, hostNamespace, deployments.Items[0].Name)
}

func printExistingDeployments(ctx *clicontext.CommandContext, cl runtimeclient.Client, ns string) error {
	deployments := &appsv1.DeploymentList{}
	if err := cl.List(ctx.Context, deployments, runtimeclient.InNamespace(ns)); err != nil {
		return err
	}
	ctx.Infof("Existing deployments in the '%s' namespace:", ns)
	for _, deployment := range deployments.Items {
		ctx.Infof("- %s", deployment.Name)
	}
	return nil
}

func scaleToZero(ctx *clicontext.CommandContext, cl runtimeclient.Client, namespacedName types.NamespacedName) (int32, error) {
	// get the deployment
	deployment := &appsv1.Deployment{}
	if err := cl.Get(ctx.Context, namespacedName, deployment); err != nil {
		return 0, err
	}
	// keep original number of replicas so we can bring it back
	originalReplicas := *deployment.Spec.Replicas
	zero := int32(0)
	deployment.Spec.Replicas = &zero

	// update the deployment so it scales to zero
	return originalReplicas, cl.Update(ctx.Context, deployment)
}

func scaleBack(ctx *clicontext.CommandContext, cl runtimeclient.Client, namespacedName types.NamespacedName, originalReplicas int32) error {
	return wait.Poll(500*time.Millisecond, 10*time.Second, func() (done bool, err error) {
		ctx.Info("")
		ctx.Infof("Trying to scale the deployment back to '%d'", originalReplicas)
		// get the updated
		deployment := &appsv1.Deployment{}
		if err := cl.Get(ctx.Context, namespacedName, deployment); err != nil {
			return false, err
		}
		// check if the replicas number wasn't already reset by a controller
		if *deployment.Spec.Replicas == originalReplicas {
			return true, nil
		}
		// set the original
		deployment.Spec.Replicas = &originalReplicas
		// and update to scale back
		if err := cl.Update(ctx.Context, deployment); err != nil {
			ctx.Warnf("error updating Deployment '%s': %s. Will retry again...", namespacedName.Name, err.Error())
			return false, nil
		}
		return true, nil
	})
}
