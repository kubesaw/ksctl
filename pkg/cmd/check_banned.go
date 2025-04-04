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
		Short: "Unban the user so that they can use the platform again",
		Long:  "Unban the user that previously registered with the provided email so that they can start using the platform again.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return CheckBanned(ctx, mur, signup, email)
		},
	}
	command.Flags().StringVarP(&mur, "mur", "m", "", "the name of the master user record to check the banned status of")
	command.Flags().StringVarP(&signup, "signup", "s", "", "the name of the signup to check the banned status of")
	command.Flags().StringVarP(&email, "email", "e", "", "the email of the user to check the banned status of")

	return command
}

func CheckBanned(ctx *clicontext.CommandContext, mur string, signup string, email string) error {
	if !validateCheckBannedInput(mur, signup, email) {
		return fmt.Errorf("exactly 1 of --mur, --signup or --email must be specified")
	}

	cl, cfg, err := getClient(ctx)
	if err != nil {
		return err
	}

	if mur != "" {
		email, err = findEmailByMUR(ctx, cl, mur, cfg.OperatorNamespace)
	} else if signup != "" {
		email, err = findEmailByUserSignup(ctx, cl, signup, cfg.OperatorNamespace)
	}
	if err != nil {
		return err
	}

	if email == "" {
		ctx.Println("User not found.")
		return nil
	}

	emailHash := hash.EncodeString(email)
	bu, err := banneduser.GetBannedUser(ctx, emailHash, cl, cfg.OperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to find the banned user: %w", err)
	}

	if bu == nil {
		ctx.Println("User is NOT banned.")
	} else {
		ctx.Println("User is banned.")
	}
	return nil
}

func validateCheckBannedInput(mur, signup, email string) bool {
	var flagCount int

	if mur != "" {
		flagCount += 1
	}
	if signup != "" {
		flagCount += 1
	}
	if email != "" {
		flagCount += 1
	}

	return flagCount == 1
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

func findEmailByUserSignup(ctx context.Context, cl runtimeclient.Client, usersignup, namespace string) (string, error) {
	obj := &toolchainv1alpha1.UserSignup{}
	if err := cl.Get(ctx, runtimeclient.ObjectKey{Name: usersignup, Namespace: namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return obj.Spec.IdentityClaims.Email, nil
}
