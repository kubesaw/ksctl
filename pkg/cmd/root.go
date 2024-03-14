package cmd

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/kubesaw/ksctl/pkg/cmd/adm"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/version"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = NewRootCmd()

func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ksctl",
		Short:   "KubeSaw command-line",
		Long:    `KubeSaw command-line tool that helps you to manage your KubeSaw service`,
		Version: version.NewMessage(),
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configuration.ConfigFileFlag, "config", "", "config file (default is $HOME/.sandbox.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&configuration.Verbose, "verbose", "v", false, "print extra info/debug messages")

	// commands with go runtime client
	rootCmd.AddCommand(NewAddSpaceUsersCmd())
	rootCmd.AddCommand(NewApproveCmd())
	rootCmd.AddCommand(NewBanCmd())
	rootCmd.AddCommand(NewDeactivateCmd())
	rootCmd.AddCommand(NewPromoteSpaceCmd())
	rootCmd.AddCommand(NewPromoteUserCmd())
	rootCmd.AddCommand(NewRemoveSpaceUsersCmd())
	rootCmd.AddCommand(NewRetargetCmd())
	rootCmd.AddCommand(NewStatusCmd())
	rootCmd.AddCommand(NewGdprDeleteCmd())
	rootCmd.AddCommand(NewCreateSocialEventCmd())
	rootCmd.AddCommand(NewGetCmd())
	rootCmd.AddCommand(NewLogsCmd())
	rootCmd.AddCommand(NewDescribeCmd())
	rootCmd.AddCommand(NewDisableUserCmd())

	// administrative commands
	rootCmd.AddCommand(adm.NewAdmCmd())

	// also, by default, we're configuring the underlying http.Client to accept insecured connections.
	// but gopkg.in/h2non/gock.v1 may change the client's Transport to intercept the requests.
	http.DefaultClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // nolint: gosec
		},
	}
}
