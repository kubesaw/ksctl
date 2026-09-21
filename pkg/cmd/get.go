package cmd

import (
	"fmt"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewGetCmd() *cobra.Command {
	// Set up the base command.
	cmd := setupKubectlCmd(func(factory cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
		return kubectlget.NewCmdGet("ksctl", factory, ioStreams)
	})

	// Define any custom flags.
	var email string
	cmd.Flags().StringVar(&email, "email", "", "email address to filter UserSignups by, which will be automatically hashed for you")

	originalPreRunE := cmd.PreRunE
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		// Run any previous prerun functions before continuing further.
		if err := originalPreRunE(cmd, args); err != nil {
			return err
		}

		// If the email flag is given, compute the hash and add it as a label.
		if email != "" {
			found := false
			for _, arg := range args {
				if strings.HasPrefix(strings.ToLower(arg), "usersignup") {
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf(`the "--email" flag can only be used to filter UserSignups`)
			}

			emailHash := hash.EncodeString(email)
			selector := toolchainv1alpha1.UserSignupUserEmailHashLabelKey + "=" + emailHash

			existingSelector := cmd.Flag("selector").Value.String()
			if existingSelector != "" {
				selector = existingSelector + "," + selector
			}

			if err := cmd.Flag("selector").Value.Set(selector); err != nil {
				return fmt.Errorf("unable to set email hash selector: %w", err)
			}
		}

		return nil
	}

	return cmd
}
