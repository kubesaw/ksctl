package cmd

import (
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewRetargetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retarget <space-name> <target-cluster>",
		Short: "Retarget the Space with the given name to the given target cluster",
		Long:  `Retargets the given Space by patching the Space.Spec.TargetCluster field to the name of the given target cluster`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Retarget(ctx, args[0], args[1])
		},
	}
}

func Retarget(ctx *clicontext.CommandContext, spaceName, targetCluster string) error {
	hostClusterConfig, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName)
	if err != nil {
		return err
	}
	hostClusterClient, err := ctx.NewClient(hostClusterConfig.Token, hostClusterConfig.ServerAPI)
	if err != nil {
		return err
	}

	// note: view toolchain role on the member cluster is good enough for retargeting since the retarget role is mainly for modifying the Space on the host
	fullTargetClusterName, err := configuration.GetMemberClusterName(ctx.Logger, targetCluster)
	if err != nil {
		return err
	}

	space, err := client.GetSpace(hostClusterClient, hostClusterConfig.OperatorNamespace, spaceName)
	if err != nil {
		return err
	}

	// let's get the creator
	creator := space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey]
	if creator == "" {
		return fmt.Errorf("spaces without the creator label are not supported")
	}
	userSignup, err := client.GetUserSignup(hostClusterClient, hostClusterConfig.OperatorNamespace, creator)
	if err != nil {
		return err
	}

	if space.Spec.TargetCluster == fullTargetClusterName {
		return fmt.Errorf("the Space '%s' is already targeted to cluster '%s'", spaceName, targetCluster)
	}

	// print Space before prompt
	if err := ctx.PrintObject("Space to be retargeted:", space); err != nil {
		return err
	}

	// and the owner (creator)
	// spec, err := yaml.Marshal(userSignup.Spec)
	// if err != nil {
	// 	return errs.Wrapf(err, "unable to unmarshal UserSignup.Spec")
	// }

	if err := ctx.PrintObject(fmt.Sprintf("Owned (created) by UserSignup '%s' with spec", userSignup.Name), userSignup); err != nil {
		return err
	}

	ctx.Warn("!!!  DANGER ZONE  !!!")
	ctx.Warn("Deleting all the user's namespaces and all their resources")
	if confirm, err := ctx.Confirm("Retarget the '%s' Space owned (created) by the '%s' UserSignup to the '%s' cluster?", spaceName, userSignup.Name, targetCluster); err != nil || !confirm {
		return err
	}

	err = client.PatchSpace(ctx, space.Name, func(space *toolchainv1alpha1.Space) (bool, error) {
		space.Spec.TargetCluster = fullTargetClusterName
		return true, nil
	}, fmt.Sprintf("Space has been patched to the '%s' target cluster ", targetCluster))
	if err != nil {
		return errs.Wrapf(err, "failed to retarget Space '%s'", spaceName)
	}

	ctx.Infof("Space has been retargeted to the '%s' cluster ", targetCluster)
	return nil
}
