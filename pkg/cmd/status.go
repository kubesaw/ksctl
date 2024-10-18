package cmd

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ToolchainStatus CR",
		Long:  `Show the ToolchainStatus CR`,
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Status(ctx)
		},
	}
}

func Status(ctx *clicontext.CommandContext) error {
	cfg, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	namespacedName := types.NamespacedName{
		Namespace: cfg.OperatorNamespace,
		Name:      "toolchain-status",
	}
	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := cl.Get(context.TODO(), namespacedName, status); err != nil {
		return err
	}

	cond, exists := condition.FindConditionByType(status.Status.Conditions, toolchainv1alpha1.ConditionReady)
	title := "Current ToolchainStatus - "
	if exists {
		title += fmt.Sprintf("Condition: %s, Status: %s, Reason: %s", cond.Type, cond.Status, cond.Reason)
		if cond.Message != "" {
			title += fmt.Sprintf(", Message: %s", cond.Message)
		}
	} else {
		title += "Condition Ready not found"
	}
	return ctx.PrintObject(title, status)
}
