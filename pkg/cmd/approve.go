package cmd

import (
	"context"
	"fmt"
	"time"

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
	var provision bool
	var usersignupName string
	var emailAddress string
	var targetCluster string
	command := &cobra.Command{
		Use:   "approve <--email someone@example.com or --name someone>",
		Short: "Approve/verify the given UserSignup resource",
		Long: `Approve the given UserSignup resource. Mark as verified when UserSignup is in no-provisioning state.
Specify the UserSignup to be approved/verified with either --name or --email flag.`,
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
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			provision = provision || targetCluster != ""
			switch {
			case usersignupName != "":
				return Approve(ctx, ByName(usersignupName), skipPhone, provision, targetCluster)
			default:
				return Approve(ctx, ByEmailAddress(emailAddress), skipPhone, provision, targetCluster)
			}
		},
	}
	command.Flags().StringVar(&usersignupName, "name", "", "the name of the UserSignup resource")
	command.Flags().StringVar(&emailAddress, "email", "", "the email address of the user")
	command.Flags().BoolVarP(&skipPhone, "skip-phone-check", "s", false, "skip the phone hash label check")
	command.Flags().StringVar(&targetCluster, "target-cluster", "", "the target cluster where the user should be provisioned")
	command.Flags().BoolVarP(&provision, "provision", "p", false, "ensures that no-provisioning state is NOT set")
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

func Approve(ctx *clicontext.CommandContext, lookupUserSignup LookupUserSignup, skipPhone, provision bool, targetCluster string) error {
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

	printObjMsg := "UserSignup to be approved"
	confirmMsg := ioutils.WithMessagef("approve the UserSignup above?")
	if states.NoProvisioning(userSignup) && !provision {
		printObjMsg = "UserSignup to be marked as verified"
		confirmMsg = ioutils.WithMessagef("mark the UserSignup as verified? It won't be provisioned.")
	}

	if err := ctx.PrintObject(userSignup, printObjMsg); err != nil {
		return err
	}
	if !ctx.AskForConfirmation(confirmMsg) {
		return nil
	}

	userSignup.Annotations[toolchainv1alpha1.UserSignupVerifiedTimestampAnnotationKey] = time.Now().Format(time.RFC3339)
	if states.NoProvisioning(userSignup) && !provision {
		states.SetVerificationRequired(userSignup, false)
		states.SetRejected(userSignup, false)
	} else {
		states.SetApprovedManually(userSignup, true)
		if targetCluster != "" {
			userSignup.Spec.TargetCluster, err = configuration.GetMemberClusterName(ctx, targetCluster)
			if err != nil {
				return err
			}
		}
	}
	if err := cl.Update(context.TODO(), userSignup); err != nil {
		return err
	}
	ctx.Printlnf("UserSignup has been approved")
	return nil
}
