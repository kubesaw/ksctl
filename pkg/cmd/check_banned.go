package cmd

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCheckBannedCommand() *cobra.Command {
	var mur string
	var signup string
	var email string

	command := &cobra.Command{
		Use:   "check-banned [--mur|-m <mur-name>] | [--signup|-s <signup-name>] | [--email|-s <email>]",
		Short: "Check whether the user is banned",
		Long:  "Check whether the user is banned and if so print the BannedUser resource.",
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return CheckBanned(ctx, mur, signup, email)
		},
	}
	command.Flags().StringVarP(&mur, "mur", "m", "", "the name of the master user record to check the banned status of")
	command.Flags().StringVarP(&signup, "signup", "s", "", "the name of the signup to check the banned status of")
	command.Flags().StringVarP(&email, "email", "e", "", "the email of the user to check the banned status of")
	command.MarkFlagsOneRequired("mur", "signup", "email")
	command.MarkFlagsMutuallyExclusive("mur", "signup", "email")

	return command
}

func CheckBanned(ctx *clicontext.CommandContext, mur string, signup string, email string) error {
	cl, cfg, err := getClient(ctx)
	if err != nil {
		return err
	}

	if mur != "" {
		email, err = findEmailByMUR(ctx, cl, mur, cfg.OperatorNamespace)
		// if we find the MUR then the user is most probably not banned. The only time a user can be banned and the MUR
		// exists is when the user is still in the process of being banned and the CRs are being deleted.
		if email == "" && err == nil {
			// the use might be banned because we didn't find a MUR. But we still need to distinguish between
			// a non-existent and banned user. We can try to lookup the user by UserSignup.Status.CompliantUserName (which
			// is used as the name of the MUR).
			email, err = findEmailByBannedUserSignupCompliantUserName(ctx, cl, mur, cfg.OperatorNamespace)
		}
	} else if signup != "" {
		email, err = findEmailByUserSignupName(ctx, cl, signup, cfg.OperatorNamespace)
	}
	if err != nil {
		return err
	}

	if email == "" {
		ctx.Println("User not found.")
		return nil
	}

	// we get the email either as the user input or we found it from the mur or signup name.
	emailHash := hash.EncodeString(email)

	bu, err := banneduser.GetBannedUser(ctx, emailHash, cl, cfg.OperatorNamespace)
	if err != nil {
		return fmt.Errorf("banned user request failed: %w", err)
	}

	if bu == nil {
		ctx.Println("User is NOT banned.")
	} else {
		ctx.Println("User is banned.")
		return ctx.PrintObject(bu, "BannedUser resource")
	}
	return nil
}

func findEmailByMUR(ctx context.Context, cl runtimeclient.Client, mur, namespace string) (string, error) {
	obj := &toolchainv1alpha1.MasterUserRecord{}
	if err := cl.Get(ctx, runtimeclient.ObjectKey{Name: mur, Namespace: namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return obj.Spec.PropagatedClaims.Email, nil
}

func findEmailByUserSignupName(ctx context.Context, cl runtimeclient.Client, usersignup, namespace string) (string, error) {
	obj := &toolchainv1alpha1.UserSignup{}
	if err := cl.Get(ctx, runtimeclient.ObjectKey{Name: usersignup, Namespace: namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return obj.Spec.IdentityClaims.Email, nil
}

func findEmailByBannedUserSignupCompliantUserName(ctx context.Context, cl runtimeclient.Client, compliantUsername, namespace string) (string, error) {
	list := &toolchainv1alpha1.UserSignupList{}
	if err := cl.List(ctx, list, runtimeclient.InNamespace(namespace), runtimeclient.MatchingLabels{toolchainv1alpha1.UserSignupStateLabelKey: "banned"}); err != nil {
		return "", err
	}

	for _, us := range list.Items {
		if us.Status.CompliantUsername == compliantUsername {
			return us.Spec.IdentityClaims.Email, nil
		}
	}

	return "", nil
}
