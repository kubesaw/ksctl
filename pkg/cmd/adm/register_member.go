package adm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"k8s.io/client-go/util/homedir"

	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AddClusterScriptDomain = "https://raw.githubusercontent.com/"
	AddClusterScriptPath   = "codeready-toolchain/toolchain-cicd/master/scripts/add-cluster.sh"
	AddClusterScriptURL    = AddClusterScriptDomain + AddClusterScriptPath
)

func NewRegisterMemberCmd() *cobra.Command {
	var hostKubeconfig, memberKubeconfig string
	cmd := &cobra.Command{
		Use:   "register-member",
		Short: "Executes add-cluster.sh script",
		Long:  `Downloads the 'add-cluster.sh' script from the 'toolchain-cicd' repo and calls it twice: once to register the Host cluster in the Member cluster and once to register the Member cluster in the host cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			newCommand := func(name string, args ...string) *exec.Cmd {
				return exec.Command(name, args...)
			}
			return registerMemberCluster(ctx, newCommand, hostKubeconfig, memberKubeconfig)
		},
	}
	defaultKubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	cmd.Flags().StringVar(&hostKubeconfig, "host-kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file of the host cluster (default: "+defaultKubeconfigPath+")")
	flags.MustMarkRequired(cmd, "host-kubeconfig")
	cmd.Flags().StringVar(&memberKubeconfig, "member-kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file of the member cluster (default: "+defaultKubeconfigPath+")")
	flags.MustMarkRequired(cmd, "member-kubeconfig")
	return cmd
}

func registerMemberCluster(ctx *clicontext.CommandContext, newCommand client.CommandCreator, hostKubeconfig, memberKubeconfig string) error {
	ctx.AskForConfirmation(ioutils.WithMessagef("register member cluster from kubeconfig %s. Be aware that the ksctl disables automatic approval to prevent new users being provisioned to the new member cluster. "+
		"You will need to enable it again manually.", memberKubeconfig))

	hostClusterConfig, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	hostClusterClient, err := ctx.NewClient(hostClusterConfig.Token, hostClusterConfig.ServerAPI)
	if err != nil {
		return err
	}

	if err := disableAutomaticApproval(hostClusterConfig, hostClusterClient); err != nil {
		return err
	}

	if err := runAddClusterScript(ctx, newCommand, configuration.Host, hostKubeconfig, memberKubeconfig); err != nil {
		return err
	}
	if err := runAddClusterScript(ctx, newCommand, configuration.Member, hostKubeconfig, memberKubeconfig); err != nil {
		return err
	}

	warningMessage := "The automatic approval was disabled!\n Configure the new member cluster in ToolchainConfig and apply the changes to the cluster."

	if err := restartHostOperator(ctx, hostClusterClient, hostClusterConfig); err != nil {
		return fmt.Errorf("%w\nIn Additon, there is another warning you should be aware of:\n%s", err, warningMessage)
	}

	ctx.Printlnf("!!!!!!!!!!!!!!!")
	ctx.Printlnf("!!! WARNING !!!")
	ctx.Printlnf("!!!!!!!!!!!!!!!")
	ctx.Printlnf(warningMessage)
	return nil
}

func disableAutomaticApproval(hostClusterConfig configuration.ClusterConfig, cl runtimeclient.Client) error {
	configs := &toolchainv1alpha1.ToolchainConfigList{}
	if err := cl.List(context.TODO(), configs, runtimeclient.InNamespace(hostClusterConfig.SandboxNamespace)); err != nil {
		return err
	}

	if len(configs.Items) == 0 {
		return nil
	}

	if len(configs.Items) > 1 {
		return fmt.Errorf("there are more than one instance of ToolchainConfig")
	}

	toolchainConfig := configs.Items[0]
	if toolchainConfig.Spec.Host.AutomaticApproval.Enabled != nil && *toolchainConfig.Spec.Host.AutomaticApproval.Enabled {
		enabled := false
		toolchainConfig.Spec.Host.AutomaticApproval.Enabled = &enabled
		return cl.Update(context.TODO(), &toolchainConfig)
	}
	return nil
}

func runAddClusterScript(term ioutils.Terminal, newCommand client.CommandCreator, joiningClusterType configuration.ClusterType, hostKubeconfig, memberKubeconfig string) error {
	if !term.AskForConfirmation(ioutils.WithMessagef("register the %s cluster by creating a ToolchainCluster CR, a Secret and a new ServiceAccount resource?", joiningClusterType)) {
		return nil
	}

	script, err := downloadScript(term)
	if err != nil {
		return err
	}
	args := []string{script.Name(), "--type", joiningClusterType.String(), "--host-kubeconfig", hostKubeconfig, "--member-kubeconfig", memberKubeconfig, "--lets-encrypt"}
	term.Printlnf("Command to be called: bash %s\n", strings.Join(args, " "))
	bash := newCommand("bash", args...)
	bash.Stdout = os.Stdout
	bash.Stderr = os.Stderr
	return bash.Run()
}

func downloadScript(term ioutils.Terminal) (*os.File, error) {
	resp, err := http.Get(AddClusterScriptURL)
	if err != nil {
		return nil, errs.Wrapf(err, "unable to get add-script.sh")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("unable to get add-script.sh - response status %s", resp.Status)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			term.Printlnf(err.Error())
		}
	}()
	// Create the file
	file, err := os.CreateTemp("", "add-cluster-*.sh")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			term.Printlnf(err.Error())
		}
	}()

	// Write the body to file
	_, err = io.Copy(file, resp.Body)
	return file, err
}
