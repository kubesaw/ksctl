package cmd

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewBanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ban <usersignup-name>",
		Short: "Ban a user for the given UserSignup resource",
		Long: `Ban the given UserSignup resource. There is expected 
only one parameter which is the name of the UserSignup to be used for banning`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient, client.DefaultNewRESTClient)
			return Ban(ctx, args...)
		},
	}
}

func Ban(ctx *clicontext.CommandContext, args ...string) error {
	return CreateBannedUser(ctx, args[0], func(userSignup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
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

func CreateBannedUser(ctx *clicontext.CommandContext, userSignupName string, confirm func(*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.BannedUser) (bool, error)) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	userSignup, err := client.GetUserSignup(cl, cfg.SandboxNamespace, userSignupName)
	if err != nil {
		return err
	}

	bannedUser, err := newBannedUser(userSignup)
	if err != nil {
		return err
	}

	bannedUsers := &toolchainv1alpha1.BannedUserList{}
	if err := cl.List(context.TODO(), bannedUsers, runtimeclient.MatchingLabels(bannedUser.Labels), runtimeclient.InNamespace(cfg.SandboxNamespace)); err != nil {
		return err
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be banned"); err != nil {
		return err
	}
	if len(bannedUsers.Items) > 0 {
		ctx.Println("The user was already banned - there is a BannedUser resource with the same labels already present")
		return ctx.PrintObject(&bannedUsers.Items[0], "BannedUser resource")
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

func newBannedUser(userSignup *toolchainv1alpha1.UserSignup) (*toolchainv1alpha1.BannedUser, error) {
	var emailHashLbl, phoneHashLbl string
	var exists bool

	if userSignup.Spec.IdentityClaims.Email == "" {
		return nil, fmt.Errorf("the UserSignup doesn't have email set")
	}

	if emailHashLbl, exists = userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey]; !exists {
		return nil, fmt.Errorf("the UserSignup doesn't have the label '%s' set", toolchainv1alpha1.UserSignupUserEmailHashLabelKey)
	}

	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    userSignup.Namespace,
			GenerateName: "banneduser-",
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: emailHashLbl,
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: userSignup.Spec.IdentityClaims.Email,
		},
	}

	if phoneHashLbl, exists = userSignup.Labels[toolchainv1alpha1.UserSignupUserPhoneHashLabelKey]; exists {
		bannedUser.Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey] = phoneHashLbl
	}
	return bannedUser, nil
}
