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
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	AddClusterScriptDomain = "https://raw.githubusercontent.com/"
	AddClusterScriptPath   = "codeready-toolchain/toolchain-cicd/master/scripts/add-cluster.sh"
	AddClusterScriptURL    = AddClusterScriptDomain + AddClusterScriptPath
)

// newClientFromRestConfigFunc is a function to create a new Kubernetes client using the provided
// rest configuration.
type newClientFromRestConfigFunc func(*rest.Config) (runtimeclient.Client, error)

// This is an extended version of the CommandContext that is used specifically just in the register member command.
type extendedCommandContext struct {
	*clicontext.CommandContext
	NewClientFromRestConfig newClientFromRestConfigFunc
}

func newExtendedCommandContext(term ioutils.Terminal, clientCtor newClientFromRestConfigFunc) *extendedCommandContext {
	return &extendedCommandContext{
		CommandContext:          clicontext.NewCommandContext(term, nil),
		NewClientFromRestConfig: clientCtor,
	}
}

func NewRegisterMemberCmd() *cobra.Command {
	var hostKubeconfig, memberKubeconfig string
	var useLetsEncrypt bool
	cmd := &cobra.Command{
		Use:   "register-member",
		Short: "Executes add-cluster.sh script",
		Long:  `Downloads the 'add-cluster.sh' script from the 'toolchain-cicd' repo and calls it twice: once to register the Host cluster in the Member cluster and once to register the Member cluster in the host cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := newExtendedCommandContext(term, client.DefaultNewClientFromRestConfig)
			newCommand := func(name string, args ...string) *exec.Cmd {
				return exec.Command(name, args...)
			}
			return registerMemberCluster(ctx, newCommand, hostKubeconfig, memberKubeconfig, useLetsEncrypt, 5*time.Minute)
		},
	}
	defaultKubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	cmd.Flags().StringVar(&hostKubeconfig, "host-kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file of the host cluster (default: "+defaultKubeconfigPath+")")
	cmd.Flags().StringVar(&memberKubeconfig, "member-kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file of the member cluster (default: "+defaultKubeconfigPath+")")
	cmd.Flags().BoolVar(&useLetsEncrypt, "lets-encrypt", true, "Whether to use Let's Encrypt certificates or rely on the cluster certs (default: true)")
	return cmd
}

func registerMemberCluster(ctx *extendedCommandContext, newCommand client.CommandCreator, hostKubeconfig, memberKubeconfig string, useLetsEncrypt bool, waitForReadyTimeout time.Duration) error {
	ctx.AskForConfirmation(ioutils.WithMessagef("register member cluster from kubeconfig %s. Note that the newly registered cluster will not be used for any space placement yet. This command will output an example"+
		" SpaceProvisionerConfig that you can modify with the required configuration options and apply to make the cluster available for space placement.", memberKubeconfig))

	// construct the client to the host cluster and figure out the API endpoint of the member cluster. We will use this information later,
	// but let's obtain it first so that we fail without leaving garbage if there is some kind of simple misconfiguration.
	hostConfig, err := clientcmd.LoadFromFile(hostKubeconfig)
	if err != nil {
		return err
	}
	hostClientConfig, err := clientcmd.NewDefaultClientConfig(*hostConfig, nil).ClientConfig()
	if err != nil {
		return err
	}
	hostClusterClient, err := ctx.NewClientFromRestConfig(hostClientConfig)
	if err != nil {
		return err
	}
	hostApiEndpoint, hostOperatorNamespace := getServerAPIEndpointAndNamespace(hostConfig)
	if hostOperatorNamespace == "" {
		hostClusterConfig, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
		if err != nil {
			return fmt.Errorf("host operator namespace could not be determined and also failed to load the .ksctl.yaml: %w", err)
		}
		hostOperatorNamespace = hostClusterConfig.OperatorNamespace
	}
	memberConfig, err := clientcmd.LoadFromFile(memberKubeconfig)
	if err != nil {
		return err
	}
	memberApiEndpoint, memberOperatorNamespace := getServerAPIEndpointAndNamespace(memberConfig)
	memberClientConfig, err := clientcmd.NewDefaultClientConfig(*memberConfig, nil).ClientConfig()
	if err != nil {
		return err
	}
	memberClusterClient, err := ctx.NewClientFromRestConfig(memberClientConfig)
	if err != nil {
		return err
	}

	// as the first thing, let's check that the member has not been yet registered with the cluster.
	hostsInMember := &toolchainv1alpha1.ToolchainClusterList{}
	if err = memberClusterClient.List(ctx, hostsInMember, runtimeclient.InNamespace(memberOperatorNamespace)); err != nil {
		return err
	}
	if len(hostsInMember.Items) > 0 {
		return fmt.Errorf("the member cluster (%s) is already registered with some host cluster in namespace %s", memberApiEndpoint, memberOperatorNamespace)
	}

	// add the host entry to the member cluster first. We assume that there is just 1 toolchain cluster entry in the member
	// cluster (i.e. it just points back to the host), so there's no need to determine the number of entries with the same
	// API endpoint.
	hostToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Host), hostApiEndpoint, 0)
	if err != nil {
		return err
	}
	hostToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      hostToolchainClusterName,
		Namespace: memberOperatorNamespace,
	}
	if err := runAddClusterScript(ctx, newCommand, configuration.Host, hostKubeconfig, hostOperatorNamespace, memberKubeconfig, memberOperatorNamespace, 0, useLetsEncrypt); err != nil {
		return err
	}

	if err := waitUntilToolchainClusterReady(ctx.CommandContext, memberClusterClient, hostToolchainClusterKey, waitForReadyTimeout); err != nil {
		ctx.Println("The ToolchainCluster resource representing the host in the member cluster has not become ready.")
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s member cluster.", hostToolchainClusterKey, memberApiEndpoint)
		return err
	}

	// figure out the name that will be given to our new ToolchainCluster representing the member in the host cluster.
	// This is the same name that the add-cluster.sh script will deduce and use.
	ord, err := getNumberOfToolchainClustersWithHostname(ctx, hostClusterClient, memberApiEndpoint, hostOperatorNamespace)
	if err != nil {
		return err
	}
	memberToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Member), memberApiEndpoint, ord)
	if err != nil {
		return err
	}
	memberToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      memberToolchainClusterName,
		Namespace: hostOperatorNamespace,
	}
	if err := runAddClusterScript(ctx, newCommand, configuration.Member, hostKubeconfig, hostOperatorNamespace, memberKubeconfig, memberOperatorNamespace, ord, useLetsEncrypt); err != nil {
		return err
	}

	if err := waitUntilToolchainClusterReady(ctx.CommandContext, hostClusterClient, memberToolchainClusterKey, waitForReadyTimeout); err != nil {
		ctx.Println("The ToolchainCluster resource representing the member in the host cluster has not become ready.")
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s host cluster. Note also that there already exists %s ToolchainCluster resource in the member cluster.", memberToolchainClusterKey, hostApiEndpoint, hostToolchainClusterKey)
		return err
	}

	exampleSPC := &toolchainv1alpha1.SpaceProvisionerConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SpaceProvisionerConfig",
			APIVersion: toolchainv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      memberToolchainClusterKey.Name,
			Namespace: memberToolchainClusterKey.Namespace,
		},
		Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
			ToolchainCluster: memberToolchainClusterKey.Name,
			Enabled:          false,
			PlacementRoles: []string{
				cluster.RoleLabel(cluster.Tenant),
			},
		},
	}

	yaml, err := yaml.Marshal(exampleSPC)
	if err != nil {
		return fmt.Errorf("failed to marshal the example SpaceProvisionerConfig to YAML: %w", err)
	}

	ctx.Printlnf(`
Modify and apply the following SpaceProvisionerConfig to the host cluster (%s) to configure the provisioning
of the spaces to the newly registered member cluster. Nothing will be deployed to the cluster
until the SpaceProvisionerConfig.spec.enabled is set to true.

%s`, hostApiEndpoint, string(yaml))

	return nil
}

func runAddClusterScript(term ioutils.Terminal, newCommand client.CommandCreator, joiningClusterType configuration.ClusterType, hostKubeconfig, hostNs, memberKubeconfig, memberNs string, memberOrdinal int, useLetsEncrypt bool) error {
	if !term.AskForConfirmation(ioutils.WithMessagef("register the %s cluster by creating a ToolchainCluster CR, a Secret and a new ServiceAccount resource?", joiningClusterType)) {
		return nil
	}

	script, err := downloadScript(term)
	if err != nil {
		return err
	}
	args := []string{script.Name(), "--type", joiningClusterType.String(), "--host-kubeconfig", hostKubeconfig, "--host-ns", hostNs, "--member-kubeconfig", memberKubeconfig, "--member-ns", memberNs}
	if memberOrdinal > 0 {
		args = append(args, "--multi-member", fmt.Sprintf("%d", memberOrdinal))
	}
	if useLetsEncrypt {
		args = append(args, "--lets-encrypt")
	}
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

func waitUntilToolchainClusterReady(ctx *clicontext.CommandContext, cl runtimeclient.Client, toolchainClusterKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for ToolchainCluster %s to become ready", toolchainClusterKey)
		tc := &toolchainv1alpha1.ToolchainCluster{}
		if err := cl.Get(ctx, toolchainClusterKey, tc); err != nil {
			return false, err
		}

		for _, cond := range tc.Status.Conditions {
			if cond.Type == toolchainv1alpha1.ToolchainClusterReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func getServerAPIEndpointAndNamespace(kubeConfig *clientcmdapi.Config) (string, string) {
	ctx, found := kubeConfig.Contexts[kubeConfig.CurrentContext]
	if !found {
		return "", ""
	}

	cluster, found := kubeConfig.Clusters[ctx.Cluster]
	if !found {
		return "", ctx.Namespace
	}

	return cluster.Server, ctx.Namespace
}

func getNumberOfToolchainClustersWithHostname(ctx context.Context, cl runtimeclient.Client, hostName string, ns string) (int, error) {
	list := &toolchainv1alpha1.ToolchainClusterList{}
	if err := cl.List(ctx, list, runtimeclient.InNamespace(ns)); err != nil {
		return 0, err
	}

	cnt := 0
	for _, tc := range list.Items {
		if tc.Spec.APIEndpoint == hostName {
			cnt += 1
		}
	}

	return cnt, nil
}
