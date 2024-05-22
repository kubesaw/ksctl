package adm

import (
	"context"
	"errors"
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

type registerMemberArgs struct {
	hostKubeconfig   string
	memberKubeconfig string
	hostNamespace    string
	memberNamespace  string
	useLetsEncrypt   bool
	nameSuffix       string
}

func newRegisterMemberArgs() registerMemberArgs {
	args := registerMemberArgs{}

	defaultKubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	args.hostKubeconfig = defaultKubeconfigPath
	args.memberKubeconfig = defaultKubeconfigPath
	args.hostNamespace = "toolchain-host-operator"
	args.memberNamespace = "toolchain-member-operator"
	args.useLetsEncrypt = true

	return args
}

func NewRegisterMemberCmd() *cobra.Command {
	commandArgs := newRegisterMemberArgs()
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
			return registerMemberCluster(ctx, newCommand, 5*time.Minute, commandArgs)
		},
	}
	cmd.Flags().StringVar(&commandArgs.hostKubeconfig, "host-kubeconfig", commandArgs.hostKubeconfig, fmt.Sprintf("Path to the kubeconfig file of the host cluster (default: %s)", commandArgs.hostKubeconfig))
	cmd.Flags().StringVar(&commandArgs.memberKubeconfig, "member-kubeconfig", commandArgs.memberKubeconfig, fmt.Sprintf("Path to the kubeconfig file of the member cluster (default: %s)", commandArgs.memberKubeconfig))
	cmd.Flags().BoolVar(&commandArgs.useLetsEncrypt, "lets-encrypt", commandArgs.useLetsEncrypt, fmt.Sprintf("Whether to use Let's Encrypt certificates or rely on the cluster certs (default: %t)", commandArgs.useLetsEncrypt))
	cmd.Flags().StringVar(&commandArgs.nameSuffix, "name-suffix", commandArgs.nameSuffix, fmt.Sprintf("The suffix to append to the member name used when there are multiple members in a single cluster (default: %s)", commandArgs.nameSuffix))
	cmd.Flags().StringVar(&commandArgs.hostNamespace, "host-ns", commandArgs.hostNamespace, fmt.Sprintf("The namespace of the host operator in the host cluster (default: %s)", commandArgs.hostNamespace))
	cmd.Flags().StringVar(&commandArgs.memberNamespace, "member-ns", commandArgs.memberNamespace, fmt.Sprintf("The namespace of the member operator in the member cluster (default: %s)", commandArgs.memberNamespace))
	return cmd
}

func registerMemberCluster(ctx *extendedCommandContext, newCommand client.CommandCreator, waitForReadyTimeout time.Duration, args registerMemberArgs) error {
	data, err := dataFromArgs(ctx, args, waitForReadyTimeout)
	if err != nil {
		return err
	}

	validated, err := data.validate(ctx)
	if err != nil {
		return err
	}

	if len(validated.errors) > 0 {
		sb := strings.Builder{}
		sb.WriteString("Cannot proceed because of the following problems:")
		for _, e := range validated.errors {
			sb.WriteString("\n- ")
			sb.WriteString(e)
		}
		return errors.New(sb.String())
	}

	if !ctx.AskForConfirmation(validated.confirmationPrompt()) {
		return nil
	}

	return validated.perform(ctx, newCommand)
}

func runAddClusterScript(term ioutils.Terminal, newCommand client.CommandCreator, joiningClusterType configuration.ClusterType, hostKubeconfig, hostNs, memberKubeconfig, memberNs, nameSuffix string, useLetsEncrypt bool) error {
	if !term.AskForConfirmation(ioutils.WithMessagef("register the %s cluster by creating a ToolchainCluster CR, a Secret and a new ServiceAccount resource?", joiningClusterType)) {
		return nil
	}

	script, err := downloadScript(term)
	if err != nil {
		return err
	}
	args := []string{script.Name(), "--type", joiningClusterType.String(), "--host-kubeconfig", hostKubeconfig, "--host-ns", hostNs, "--member-kubeconfig", memberKubeconfig, "--member-ns", memberNs}
	if len(nameSuffix) > 0 {
		args = append(args, "--multi-member", nameSuffix)
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

func getServerAPIEndpoint(kubeConfig *clientcmdapi.Config) string {
	ctx, found := kubeConfig.Contexts[kubeConfig.CurrentContext]
	if !found {
		return ""
	}

	cluster, found := kubeConfig.Clusters[ctx.Cluster]
	if !found {
		return ""
	}

	return cluster.Server
}

func getToolchainClustersWithHostname(ctx context.Context, cl runtimeclient.Client, hostName string, ns string) ([]toolchainv1alpha1.ToolchainCluster, error) {
	list := &toolchainv1alpha1.ToolchainClusterList{}
	if err := cl.List(ctx, list, runtimeclient.InNamespace(ns)); err != nil {
		return nil, err
	}

	clusters := []toolchainv1alpha1.ToolchainCluster{}
	for _, tc := range list.Items {
		if tc.Spec.APIEndpoint == hostName {
			clusters = append(clusters, tc)
		}
	}

	return clusters, nil
}

type registerMemberData struct {
	hostClusterClient       runtimeclient.Client
	memberClusterClient     runtimeclient.Client
	hostApiEndpoint         string
	memberApiEndpoint       string
	hostOperatorNamespace   string
	memberOperatorNamespace string
	args                    registerMemberArgs
	waitForReadyTimeout     time.Duration
}

type registerMemberValidated struct {
	registerMemberData
	hostToolchainClusterName   string
	memberToolchainClusterName string
	warnings                   []string
	errors                     []string
}

func dataFromArgs(ctx *extendedCommandContext, args registerMemberArgs, waitForReadyTimeout time.Duration) (*registerMemberData, error) {
	hostConfig, err := clientcmd.LoadFromFile(args.hostKubeconfig)
	if err != nil {
		return nil, err
	}
	hostClientConfig, err := clientcmd.NewDefaultClientConfig(*hostConfig, nil).ClientConfig()
	if err != nil {
		return nil, err
	}
	hostClusterClient, err := ctx.NewClientFromRestConfig(hostClientConfig)
	if err != nil {
		return nil, err
	}
	hostApiEndpoint := getServerAPIEndpoint(hostConfig)
	hostOperatorNamespace := args.hostNamespace

	memberConfig, err := clientcmd.LoadFromFile(args.memberKubeconfig)
	if err != nil {
		return nil, err
	}
	memberClientConfig, err := clientcmd.NewDefaultClientConfig(*memberConfig, nil).ClientConfig()
	if err != nil {
		return nil, err
	}
	memberClusterClient, err := ctx.NewClientFromRestConfig(memberClientConfig)
	if err != nil {
		return nil, err
	}
	memberApiEndpoint := getServerAPIEndpoint(memberConfig)
	memberOperatorNamespace := args.memberNamespace

	return &registerMemberData{
		args:                    args,
		hostApiEndpoint:         hostApiEndpoint,
		memberApiEndpoint:       memberApiEndpoint,
		hostOperatorNamespace:   hostOperatorNamespace,
		memberOperatorNamespace: memberOperatorNamespace,
		hostClusterClient:       hostClusterClient,
		memberClusterClient:     memberClusterClient,
		waitForReadyTimeout:     waitForReadyTimeout,
	}, nil
}

func (d *registerMemberData) validate(ctx *extendedCommandContext) (*registerMemberValidated, error) {
	hostToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Host), d.hostApiEndpoint, "")
	if err != nil {
		return nil, err
	}

	// figure out the name that will be given to our new ToolchainCluster representing the member in the host cluster.
	// This is the same name that the add-cluster.sh script will deduce and use.
	membersInHost, err := getToolchainClustersWithHostname(ctx, d.hostClusterClient, d.memberApiEndpoint, d.hostOperatorNamespace)
	if err != nil {
		return nil, err
	}
	memberToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Member), d.memberApiEndpoint, d.args.nameSuffix)
	if err != nil {
		return nil, err
	}

	hostsInMember := &toolchainv1alpha1.ToolchainClusterList{}
	if err = d.memberClusterClient.List(ctx, hostsInMember, runtimeclient.InNamespace(d.memberOperatorNamespace)); err != nil {
		return nil, err
	}

	var warnings []string
	var errors []string

	if len(hostsInMember.Items) > 1 {
		errors = append(errors, fmt.Sprintf("member misconfigured: the member cluster (%s) is already registered with more than 1 host in namespace %s", d.memberApiEndpoint, d.memberOperatorNamespace))
	} else if len(hostsInMember.Items) == 1 {
		if hostsInMember.Items[0].Spec.APIEndpoint != d.hostApiEndpoint {
			errors = append(errors, fmt.Sprintf("the member is already registered with another host (%s) so registering it with the new one (%s) would result in an invalid configuration", hostsInMember.Items[0].Spec.APIEndpoint, d.hostApiEndpoint))
		} 
		if hostsInMember.Items[0].Name != hostToolchainClusterName {
			errors = append(errors, fmt.Sprintf("the host is already in the member namespace using a ToolchainCluster object with the name '%s' but the new registration would use a ToolchainCluster with the name '%s' which would lead to an invalid configuration", hostsInMember.Items[0].Name, hostToolchainClusterName))
		}
	}
	existingMemberToolchainCluster := findToolchainClusterForMember(membersInHost, d.memberApiEndpoint, d.memberOperatorNamespace)
	if existingMemberToolchainCluster != nil {
		warnings = append(warnings, fmt.Sprintf("there already is a registered member for the same member API endpoint and operator namespace (%s), proceeding will overwrite the objects representing it in the host and member clusters", runtimeclient.ObjectKeyFromObject(existingMemberToolchainCluster)))
		if existingMemberToolchainCluster.Name != memberToolchainClusterName {
			errors = append(errors, fmt.Sprintf("the newly registered member cluster would have a different name (%s) than the already existing one (%s) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration", memberToolchainClusterName, existingMemberToolchainCluster.Name))
		}
	}

	return &registerMemberValidated{
		registerMemberData:         *d,
		hostToolchainClusterName:   hostToolchainClusterName,
		memberToolchainClusterName: memberToolchainClusterName,
		warnings:                   warnings,
		errors:                     errors,
	}, nil
}

func (v *registerMemberValidated) confirmationPrompt() ioutils.ConfirmationMessage {
	// we have a single replacement argument at the moment so maybe this is a bit of
	// an overkill but, let's be explicit about using a format string and its arguments
	// so that mistakes are not made in the future when we update this stuff.
	sb := strings.Builder{}
	args := []any{}
	sb.WriteString("register the member cluster from kubeconfig %s?")
	args = append(args, v.args.memberKubeconfig)

	sb.WriteString("\nNote that the newly registered cluster will not be used for any space placement yet. This command will output an example SpaceProvisionerConfig that you can modify with the required configuration options and apply to make the cluster available for space placement.")

	if len(v.warnings) > 0 {
		sb.WriteString("\nPlease confirm that the following is ok and you are willing to proceed:")
		for _, f := range v.warnings {
			sb.WriteString("\n- ")
			sb.WriteString(f)
		}
		sb.WriteString("\n")
	}

	return ioutils.WithMessagef(sb.String(), args...)
}

func (v *registerMemberValidated) perform(ctx *extendedCommandContext, newCommand client.CommandCreator) error {
	// add the host entry to the member cluster first. We assume that there is just 1 toolchain cluster entry in the member
	// cluster (i.e. it just points back to the host), so there's no need to determine the number of entries with the same
	// API endpoint.
	hostToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      v.hostToolchainClusterName,
		Namespace: v.memberOperatorNamespace,
	}
	if err := runAddClusterScript(ctx, newCommand, configuration.Host, v.args.hostKubeconfig, v.hostOperatorNamespace, v.args.memberKubeconfig, v.memberOperatorNamespace, "", v.args.useLetsEncrypt); err != nil {
		return err
	}

	if err := waitUntilToolchainClusterReady(ctx.CommandContext, v.memberClusterClient, hostToolchainClusterKey, v.waitForReadyTimeout); err != nil {
		ctx.Println("The ToolchainCluster resource representing the host in the member cluster has not become ready.")
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s member cluster.", hostToolchainClusterKey, v.memberApiEndpoint)
		return err
	}

	memberToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      v.memberToolchainClusterName,
		Namespace: v.hostOperatorNamespace,
	}
	if err := runAddClusterScript(ctx, newCommand, configuration.Member, v.args.hostKubeconfig, v.hostOperatorNamespace, v.args.memberKubeconfig, v.memberOperatorNamespace, v.args.nameSuffix, v.args.useLetsEncrypt); err != nil {
		return err
	}

	if err := waitUntilToolchainClusterReady(ctx.CommandContext, v.hostClusterClient, memberToolchainClusterKey, v.waitForReadyTimeout); err != nil {
		ctx.Println("The ToolchainCluster resource representing the member in the host cluster has not become ready.")
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s host cluster. Note also that there already exists %s ToolchainCluster resource in the member cluster.", memberToolchainClusterKey, v.hostApiEndpoint, hostToolchainClusterKey)
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

	return ctx.PrintObject(exampleSPC, fmt.Sprintf(`
Modify and apply the following SpaceProvisionerConfig to the host cluster (%s) to configure the provisioning
of the spaces to the newly registered member cluster. Nothing will be deployed to the cluster
until the SpaceProvisionerConfig.spec.enabled is set to true.

`, v.hostApiEndpoint))
}

func findToolchainClusterForMember(allToolchainClusters []toolchainv1alpha1.ToolchainCluster, memberAPIEndpoint, memberOperatorNamespace string) *toolchainv1alpha1.ToolchainCluster {
	for _, tc := range allToolchainClusters {
		if tc.Spec.APIEndpoint == memberAPIEndpoint && tc.Labels["namespace"] == memberOperatorNamespace {
			return &tc
		}
	}
	return nil
}
