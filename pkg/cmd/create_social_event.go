package cmd

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonsocialevent "github.com/codeready-toolchain/toolchain-common/pkg/socialevent"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewCreateSocialEventCmd() *cobra.Command {
	var startDate string     // format" YYYY-MM-DD
	var endDate string       // format" YYYY-MM-DD
	var maxAttendees int     // must be greater than 0
	var description string   // optional
	var userTier string      // optional, default to `base`
	var spaceTier string     // optional, default to `deactivate30`
	var targetCluster string // optional

	command := &cobra.Command{
		Use:   "create-event --description=<description> --start-date=<YYYY-MM-DD> --end-date=<YYYY-MM-DD> --max-attendees=<int>",
		Short: "Create an event with a code to signup",
		Long:  `Create an event (workshop, lab, etc.) to which attendees can signup to with a code.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return CreateSocialEvent(ctx, startDate, endDate, description, userTier, spaceTier, maxAttendees, targetCluster)
		},
	}
	command.Flags().StringVar(&startDate, "start-date", "", "start date of the event/when the activation code becomes valid (YYYY-MM-DD)")
	flags.MustMarkRequired(command, "start-date")
	command.Flags().StringVar(&endDate, "end-date", "", "end date of the event/when the activation code becomes invalid (YYYY-MM-DD)")
	flags.MustMarkRequired(command, "end-date")
	command.Flags().IntVar(&maxAttendees, "max-attendees", 0, "maximum number of expected attendees for the event")
	flags.MustMarkRequired(command, "max-attendees")
	command.Flags().StringVar(&description, "description", "", "event description")
	command.Flags().StringVar(&userTier, "user-tier", "deactivate30", "tier to provision users")
	command.Flags().StringVar(&spaceTier, "space-tier", "base", "tier to provision spaces")
	command.Flags().StringVar(&targetCluster, "target-cluster", "", "the cluster in which the user/space should be provisioned in. If not set then the target cluster will be picked automatically")
	return command
}

func CreateSocialEvent(ctx *clicontext.CommandContext, startDate, endDate, description, userTier, spaceTier string, maxAttendees int, targetCluster string) error {
	if targetCluster != "" {
		// Verify the target cluster name
		targetCfg, err := configuration.LoadClusterAccessDefinition(ctx, targetCluster)
		if err != nil {
			return err
		}
		if targetCfg.ClusterType != configuration.Member {
			return fmt.Errorf("cluster '%s' is not a member cluster", targetCluster)
		}
	}

	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	// generate a unique ActivationCode if it was not specified in the CLI
	code := commonsocialevent.NewName()
	// convert the start-time and end-time
	start, err := time.ParseInLocation("2006-01-02 15:04:05", startDate+" 00:00:00", time.Local) //nolint:gosmopolitan
	if err != nil {
		return errs.Wrapf(err, "start date is invalid: '%s' (expected YYYY-MM-DD)", startDate)
	}
	end, err := time.ParseInLocation("2006-01-02 15:04:05", endDate+" 23:59:59", time.Local) //nolint:gosmopolitan
	if err != nil {
		return errs.Wrapf(err, "end date is invalid: '%s' (expected YYYY-MM-DD)", endDate)
	}
	if end.Before(start) {
		return errs.New("end date is not after start date")
	}
	// check that the user and space tiers exist
	if err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: cfg.OperatorNamespace,
		Name:      userTier,
	}, &toolchainv1alpha1.UserTier{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("UserTier '%s' does not exist", userTier)
		}
	}
	if err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: cfg.OperatorNamespace,
		Name:      spaceTier,
	}, &toolchainv1alpha1.NSTemplateTier{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("NSTemplateTier '%s' does not exist", spaceTier)
		}
	}

	se := &toolchainv1alpha1.SocialEvent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      code,
		},
		Spec: toolchainv1alpha1.SocialEventSpec{
			StartTime:     metav1.NewTime(start),
			EndTime:       metav1.NewTime(end),
			MaxAttendees:  maxAttendees,
			UserTier:      userTier,
			SpaceTier:     spaceTier,
			Description:   description,
			TargetCluster: targetCluster,
		},
	}

	if err := cl.Create(context.TODO(), se); err != nil {
		return err
	}
	ctx.Printlnf("Social Event successfully created. Activation code is '%s'", se.Name)
	return nil
}
