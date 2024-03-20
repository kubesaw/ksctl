package cmd

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewApproveCmd() *cobra.Command {
	var skipPhone bool
	var usersignupName string
	var emailAddress string
	var targetCluster string
	command := &cobra.Command{
		Use:   "approve <--email someone@example.com or --name someone>",
		Short: "Approve the given UserSignup resource",
		Long: `Approve the given UserSignup resource. There is expected 
only one parameter which is the name of the UserSignup to be approved`,
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if usersignupName != "" && emailAddress != "" {
				return fmt.Errorf("you cannot specify both 'name' and `email` flags")
			}
			if usersignupName == "" && emailAddress == "" {
				return fmt.Errorf("you must specify one of 'name' and `email` flags")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient, client.DefaultNewRESTClient)
			switch {
			case usersignupName != "":
				return Approve(ctx, ByName(usersignupName), skipPhone, targetCluster)
			default:
				return Approve(ctx, ByEmailAddress(emailAddress), skipPhone, targetCluster)
			}
		},
	}
	command.Flags().StringVar(&usersignupName, "name", "", "the name of the UserSignup resource")
	command.Flags().StringVar(&emailAddress, "email", "", "the email address of the user")
	command.Flags().BoolVarP(&skipPhone, "skip-phone-check", "s", false, "skip the phone hash label check")
	command.Flags().StringVar(&targetCluster, "target-cluster", "", "the target cluster where the user should be provisioned")
	return command
}

type LookupUserSignup func(configuration.ClusterConfig, runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error)

func ByName(name string) LookupUserSignup {
	return func(cfg configuration.ClusterConfig, cl runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error) {
		userSignup := &toolchainv1alpha1.UserSignup{}
		err := cl.Get(context.TODO(), types.NamespacedName{
			Namespace: cfg.OperatorNamespace,
			Name:      name,
		}, userSignup)
		return userSignup, err
	}
}

func ByEmailAddress(emailAddress string) LookupUserSignup {
	return func(cfg configuration.ClusterConfig, cl runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error) {
		usersignups := toolchainv1alpha1.UserSignupList{}
		if err := cl.List(context.TODO(), &usersignups, runtimeclient.InNamespace(cfg.OperatorNamespace), runtimeclient.MatchingLabels{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: hash.EncodeString(emailAddress),
		}); err != nil {
			return nil, err
		}

		// check that there's only 1 usersignup matching the email address
		if l := len(usersignups.Items); l != 1 {
			return nil, fmt.Errorf("expected a single match with the email address, but found %d", l)
		}
		u := usersignups.Items[0]
		return &u, nil
	}
}

func Approve(ctx *clicontext.CommandContext, lookupUserSignup LookupUserSignup, skipPhone bool, targetCluster string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	userSignup, err := lookupUserSignup(cfg, cl)
	if err != nil {
		return err
	}
	if state, found := userSignup.Labels[toolchainv1alpha1.StateLabelKey]; found && state == toolchainv1alpha1.UserSignupStateLabelValueApproved {
		return fmt.Errorf(`UserSignup "%s" is already approved`, userSignup.Name)
	}
	// check that the usersignup provided a phone number
	_, found := userSignup.Labels[toolchainv1alpha1.UserSignupUserPhoneHashLabelKey]
	if !skipPhone && !found {
		return fmt.Errorf(`UserSignup "%s" is missing a phone hash label - the user may not have provided a phone number for verification. In most cases, the user should be asked to attempt the phone verification process. For exceptions, skip this check using the --skip-phone-check parameter`, userSignup.Name)
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be approved"); err != nil {
		return err
	}
	if !ctx.AskForConfirmation(ioutils.WithMessagef("approve the UserSignup above?")) {
		return nil
	}
	states.SetVerificationRequired(userSignup, false)
	states.SetDeactivated(userSignup, false)
	states.SetApprovedManually(userSignup, true)
	if targetCluster != "" {
		if err = setTargetCluster(ctx, targetCluster, userSignup); err != nil {
			return err
		}
	}
	if err := cl.Update(context.TODO(), userSignup); err != nil {
		return err
	}
	ctx.Printlnf("UserSignup has been approved")
	return nil
}

func setTargetCluster(ctx *clicontext.CommandContext, targetCluster string, userSignup *toolchainv1alpha1.UserSignup) error {
	memberClusterConfig, err := configuration.LoadClusterConfig(ctx, targetCluster)
	if err != nil {
		return err
	}
	// target cluster must have 'member' cluster type
	if memberClusterConfig.ClusterType != configuration.Member {
		return fmt.Errorf("expected target cluster to have clusterType '%s', actual: '%s'", configuration.Member, memberClusterConfig.ClusterType)
	}
	// set the specified target cluster
	userSignup.Spec.TargetCluster = memberToolchainClusterName(memberClusterConfig)
	return nil
}
