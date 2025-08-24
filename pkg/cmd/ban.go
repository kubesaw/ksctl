package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// getBanningReasonsFromConfigmap reads banning reasons from a ConfigMap named "banning-reasons" in the default namespace
func getBanningReasonsFromConfigMap(ctx *clicontext.CommandContext) ([]string, error) {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return nil, err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return nil, err
	}

	// Try to get the ConfigMap from default namespace
	configMap := &corev1.ConfigMap{}
	err = cl.Get(context.TODO(), types.NamespacedName{
		Name:      "banning-reasons",
		Namespace: "default",
	}, configMap)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap doesn't exist, return empty slice to signal fallback
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get banning-reasons ConfigMap: %w", err)
	}

	// Extract reasons from ConfigMap data
	var reasons []string

	// First, check for a "reasons" key with comma-separated values
	if data, exists := configMap.Data["reasons"]; exists && data != "" {
		// Split by comma and trim whitespace
		for _, reason := range strings.Split(data, ",") {
			trimmed := strings.TrimSpace(reason)
			if trimmed != "" {
				reasons = append(reasons, trimmed)
			}
		}
	}

	// Also check for individual reason keys (reason1, reason2, etc.)
	for key, value := range configMap.Data {
		if key != "reasons" && value != "" {
			reasons = append(reasons, strings.TrimSpace(value))
		}
	}

	return reasons, nil
}

// showInteractiveBanMenu displays an interactive menu for selecting banning reasons
func showInteractiveBanMenu(ctx *clicontext.CommandContext, reasons []string) (string, error) {
	if len(reasons) == 0 {
		return "", fmt.Errorf("no banning reasons available")
	}

	var selectedReason string
	options := make([]huh.Option[string], len(reasons))
	for i, reason := range reasons {
		options[i] = huh.NewOption(reason, reason)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a banning reason:").
				Options(options...).
				Value(&selectedReason),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("failed to show interactive menu: %w", err)
	}

	return selectedReason, nil
}

func NewBanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ban <usersignup-name> <ban-reason>",
		Short: "Ban a user for the given UserSignup resource and reason of the ban",
		Long: `Ban the given UserSignup resource. There is expected 
only two parameters which the first one is the name of the UserSignup to be used for banning 
and the second one the reason of the ban`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Ban(ctx, args...)
		},
	}
}

func Ban(ctx *clicontext.CommandContext, args ...string) error {
	return CreateBannedUser(ctx, args[0], args[1], func(userSignup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		if _, exists := bannedUser.Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey]; !exists {
			ctx.Printlnf("\nINFO: The UserSignup doesn't have the label '%s' set, so the resulting BannedUser resource won't have this label either.\n",
				toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey)
		}

		if err := ctx.PrintObject(bannedUser, "BannedUser resource to be created"); err != nil {
			return false, err
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef(
			"deletion of all user's namespaces and all related data.\nIn addition, the user won't be able to login any more.",
			"ban the user with the UserSignup by creating BannedUser resource that are both above?"))
		return confirmation, nil
	})
}

func CreateBannedUser(ctx *clicontext.CommandContext, userSignupName, banReason string, confirm func(*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.BannedUser) (bool, error)) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	userSignup, err := client.GetUserSignup(cl, cfg.OperatorNamespace, userSignupName)
	if err != nil {
		return err
	}

	ksctlConfig, err := configuration.Load(ctx)
	if err != nil {
		return err
	}

	bannedUser, err := banneduser.NewBannedUser(userSignup, ksctlConfig.Name, banReason)
	if err != nil {
		return err
	}

	alreadyBannedUser, err := banneduser.GetBannedUser(ctx, bannedUser.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey], cl, cfg.OperatorNamespace)
	if err != nil {
		return err
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be banned"); err != nil {
		return err
	}

	if alreadyBannedUser != nil {
		ctx.Println("The user was already banned - there is a BannedUser resource with the same labels already present")
		return ctx.PrintObject(alreadyBannedUser, "BannedUser resource")
	}

	if shouldCreate, err := confirm(userSignup, bannedUser); !shouldCreate || err != nil {
		return err
	}

	if err := cl.Create(context.TODO(), bannedUser); err != nil {
		return err
	}

	ctx.Printlnf("\nUserSignup has been banned by creating BannedUser resource with name " + bannedUser.Name)
	return nil
}
