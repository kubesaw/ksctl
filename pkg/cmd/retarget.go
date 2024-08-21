package cmd

import (
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/ghodss/yaml"
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
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Retarget(ctx, args[0], args[1])
		},
	}
}

func Retarget(ctx *clicontext.CommandContext, spaceName, targetCluster string) error {
	hostClusterConfig, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	hostClusterClient, err := ctx.NewClient(hostClusterConfig.Token, hostClusterConfig.ServerAPI)
	if err != nil {
		return err
	}

	// note: view toolchain role on the member cluster is good enough for retargeting since the retarget role is mainly for modifying the Space on the host
	fullTargetClusterName, err := configuration.GetMemberClusterName(ctx, targetCluster)
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
	if err := ctx.PrintObject(space, "Space to be retargeted"); err != nil {
		return err
	}

	// and the owner (creator)
	spec, err := yaml.Marshal(userSignup.Spec)
	if err != nil {
		return errs.Wrapf(err, "unable to unmarshal UserSignup.Spec")
	}
	ctx.PrintContextSeparatorWithBodyf(string(spec), "Owned (created) by UserSignup '%s' with spec", userSignup.Name)

	// prompt for confirmation to proceed
	confirmationMsg := ioutils.WithDangerZoneMessagef(
		"deletion of all related namespaces and all related data",
		"retarget the Space '%s' owned (created) by UserSignup '%s' to cluster '%s'?",
		spaceName, userSignup.Name, targetCluster)

	if confirmed := ctx.AskForConfirmation(confirmationMsg); !confirmed {
		return nil
	}

	err = client.PatchSpace(ctx, space.Name, func(space *toolchainv1alpha1.Space) (bool, error) {
		space.Spec.TargetCluster = fullTargetClusterName
		return true, nil
	}, "Space has been patched to target cluster "+targetCluster)
	if err != nil {
		return errs.Wrapf(err, "failed to retarget Space '%s'", spaceName)
	}

	ctx.Printlnf("\nSpace has been retargeted to cluster " + targetCluster)
	return nil
}
