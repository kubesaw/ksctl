package adm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/cmd/generate"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/pkg/utils"
	"github.com/spf13/cobra"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TokenExpirationDays = 3650
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
	hostKubeConfig      string
	memberKubeConfig    string
	hostNamespace       string
	memberNamespace     string
	nameSuffix          string
	useLetsEncrypt      bool
	waitForReadyTimeout time.Duration
}

func NewRegisterMemberCmd() *cobra.Command {
	commandArgs := registerMemberArgs{}
	cmd := &cobra.Command{
		Use:   "register-member",
		Short: "Registers a member cluster in the host cluster and vice versa.",
		Long:  `Register the Host cluster in the Member cluster and then registers the Member cluster in the host cluster by creating toolchaincluster resources in the host and member namespaces.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := newExtendedCommandContext(term, client.DefaultNewClientFromRestConfig)
			return registerMemberCluster(ctx, commandArgs)
		},
	}

	// keep these values in sync with the values in defaultRegisterMemberArgs() function in the tests.
	defaultTimeout := 2 * time.Minute
	defaultNameSuffix := ""
	defaultHostNs := "toolchain-host-operator"
	defaultMemberNs := "toolchain-member-operator"

	cmd.Flags().StringVar(&commandArgs.hostKubeConfig, "host-kubeconfig", "", "Path to the kubeconfig file of the host cluster")
	flags.MustMarkRequired(cmd, "host-kubeconfig")
	cmd.Flags().StringVar(&commandArgs.memberKubeConfig, "member-kubeconfig", "", "Path to the kubeconfig file of the member cluster")
	flags.MustMarkRequired(cmd, "member-kubeconfig")
	cmd.Flags().BoolVar(&commandArgs.useLetsEncrypt, "lets-encrypt", true, "Whether to use Let's Encrypt certificates or rely on the cluster certs.")
	cmd.Flags().StringVar(&commandArgs.nameSuffix, "name-suffix", defaultNameSuffix, "The suffix to append to the member name used when there are multiple members in a single cluster.")
	cmd.Flags().StringVar(&commandArgs.hostNamespace, "host-ns", defaultHostNs, "The namespace of the host operator in the host cluster.")
	cmd.Flags().StringVar(&commandArgs.memberNamespace, "member-ns", defaultMemberNs, "The namespace of the member operator in the member cluster.")
	cmd.Flags().DurationVar(&commandArgs.waitForReadyTimeout, "timeout", defaultTimeout, "The max timeout used when waiting for each of the computations to be completed.")
	return cmd
}

func registerMemberCluster(ctx *extendedCommandContext, args registerMemberArgs) error {
	validated, err := validateArgs(ctx, args)
	if err != nil {
		return err
	}

	if len(validated.errors) > 0 {
		sb := strings.Builder{}
		sb.WriteString("Cannot proceed because of the following problems:")
		for _, e := range validated.errors {
			sb.WriteString("\n\t- ")
			sb.WriteString(e)
		}
		return errors.New(sb.String())
	}

	if !ctx.AskForConfirmation(validated.confirmationPrompt()) {
		return nil
	}

	return validated.perform(ctx)
}

func (v *registerMemberValidated) getSourceAndTargetClusters(sourceClusterType configuration.ClusterType) (clusterData, clusterData) {
	if sourceClusterType == configuration.Member {
		return v.memberClusterData, v.hostClusterData
	}
	return v.hostClusterData, v.memberClusterData
}

// addCluster creates a secret and a ToolchainCluster resource on the `targetCluster`.
// This ToolchainCluster CR stores a reference to the secret which contains the kubeconfig of the `sourceCluster`. Thus enables the `targetCluster` to interact with the `sourceCluster`.
// - `targetCluster` is the cluster where we create the ToolchainCluster resource and the secret
// - `sourceCluster` is the cluster referenced in the kubeconfig/ToolchainCluster of the `targetCluster`
func (v *registerMemberValidated) addCluster(ctx *extendedCommandContext, sourceClusterType configuration.ClusterType) error {
	ctx.PrintContextSeparatorf("Ensuring connection from the %s cluster to the %s via a ToolchainCluster CR, a Secret, and a new ServiceAccount resource", sourceClusterType, sourceClusterType.TheOtherType())
	sourceClusterDetails, targetClusterDetails := v.getSourceAndTargetClusters(sourceClusterType)
	// wait for the SA to be ready
	toolchainClusterSAKey := runtimeclient.ObjectKey{
		Name:      fmt.Sprintf("toolchaincluster-%s", sourceClusterType),
		Namespace: sourceClusterDetails.namespace,
	}
	if err := waitForToolchainClusterSA(ctx.CommandContext, sourceClusterDetails.client, toolchainClusterSAKey, v.args.waitForReadyTimeout); err != nil {
		ctx.Printlnf("The %s ServiceAccount is not present in the %s cluster.", toolchainClusterSAKey, sourceClusterType)
		ctx.Printlnf("Please check the %[1]s ToolchainCluster ServiceAccount in the %[2]s %[3]s cluster or the deployment of the %[3]s operator.", toolchainClusterSAKey, sourceClusterDetails.apiEndpoint, sourceClusterType)
		return err
	}
	// source cluster details
	ctx.Printlnf("The source cluster name: %s", sourceClusterDetails.toolchainClusterName)
	ctx.Printlnf("The API endpoint of the source cluster: %s", sourceClusterDetails.apiEndpoint)

	// target to details
	ctx.Printlnf("The name of the target cluster: %s", targetClusterDetails.toolchainClusterName)
	ctx.Printlnf("The API endpoint of the target cluster: %s", targetClusterDetails.apiEndpoint)

	// generate a token that will be used for the kubeconfig
	sourceTargetRestClient, err := newRestClient(sourceClusterDetails.kubeConfig)
	if err != nil {
		return err
	}
	token, err := generate.GetServiceAccountToken(sourceTargetRestClient, toolchainClusterSAKey, TokenExpirationDays)
	if err != nil {
		return err
	}
	// TODO drop this part together with the --lets-encrypt flag and start loading certificate from the kubeconfig as soon as ToolchainCluster controller supports loading certificates from kubeconfig
	var insecureSkipTLSVerify bool
	if v.args.useLetsEncrypt {
		ctx.Printlnf("using let's encrypt certificate")
		insecureSkipTLSVerify = false
	} else {
		ctx.Printlnf("setting insecure skip tls verification flags")
		insecureSkipTLSVerify = true
	}
	// generate the kubeconfig that can be used by target cluster to interact with the source cluster
	generatedKubeConfig := generateKubeConfig(token, sourceClusterDetails.apiEndpoint, sourceClusterDetails.namespace, insecureSkipTLSVerify)
	generatedKubeConfigFormatted, err := clientcmd.Write(*generatedKubeConfig)
	if err != nil {
		return err
	}

	// Create or Update the secret on the targetCluster
	secretName := toolchainClusterSAKey.Name + "-" + sourceClusterDetails.toolchainClusterName
	ctx.Printlnf("creating secret %s/%s in the %s cluster", targetClusterDetails.namespace, secretName, sourceClusterType.TheOtherType())
	kubeConfigSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: targetClusterDetails.namespace}}
	_, err = controllerutil.CreateOrUpdate(context.TODO(), targetClusterDetails.client, kubeConfigSecret, func() error {

		// update the secret label
		labels := kubeConfigSecret.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[toolchainv1alpha1.ToolchainClusterLabel] = sourceClusterDetails.toolchainClusterName
		kubeConfigSecret.Labels = labels

		// update the kubeconfig data
		kubeConfigSecret.StringData = map[string]string{
			"kubeconfig": string(generatedKubeConfigFormatted),
			"token":      token,
		}

		return nil
	})

	if err != nil {
		return err
	}
	ctx.Println("Secret successfully reconciled")

	// TODO -- temporary logic
	// The creation of the toolchaincluster is just temporary until we implement https://issues.redhat.com/browse/KUBESAW-44,
	// the creation logic will be moved to the toolchaincluster_resource controller in toolchain-common and will be based on the secret created above.
	//
	// create/update toolchaincluster on the targetCluster
	ctx.Printlnf("creating ToolchainCluster representation of %s in %s:", sourceClusterType, targetClusterDetails.toolchainClusterName)
	toolchainClusterCR := &toolchainv1alpha1.ToolchainCluster{ObjectMeta: metav1.ObjectMeta{Name: sourceClusterDetails.toolchainClusterName, Namespace: targetClusterDetails.namespace}}
	_, err = controllerutil.CreateOrUpdate(context.TODO(), targetClusterDetails.client, toolchainClusterCR, func() error {

		// update the tc label
		labels := toolchainClusterCR.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		// TODO drop this "namespace" label as soon as ToolchainCluster controller supports loading data from kubeconfig
		labels["namespace"] = sourceClusterDetails.namespace
		toolchainClusterCR.Labels = labels
		toolchainClusterCR.Spec.APIEndpoint = sourceClusterDetails.apiEndpoint
		toolchainClusterCR.Spec.SecretRef.Name = secretName
		if insecureSkipTLSVerify {
			toolchainClusterCR.Spec.DisabledTLSValidations = []toolchainv1alpha1.TLSValidation{toolchainv1alpha1.TLSAll}
		}

		return nil
	})

	if err != nil {
		return err
	}
	ctx.Println("Toolchaincluster successfully reconciled")
	toolchainClusterKey := runtimeclient.ObjectKey{
		Name:      sourceClusterDetails.toolchainClusterName,
		Namespace: targetClusterDetails.namespace,
	}
	if err := waitUntilToolchainClusterReady(ctx.CommandContext, targetClusterDetails.client, toolchainClusterKey, v.args.waitForReadyTimeout); err != nil {
		ctx.Printlnf("The ToolchainCluster resource representing the %s in the %s cluster has not become ready.", sourceClusterType, sourceClusterType.TheOtherType())
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s %s cluster.", toolchainClusterKey, targetClusterDetails.apiEndpoint, sourceClusterType.TheOtherType())
		return err
	}
	// -- end temporary logic

	return nil
}

func newRestClient(kubeConfigPath string) (*rest.RESTClient, error) {
	restClientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	// those fields are required when using the rest client otherwise it throws and error
	// see: https://github.com/kubernetes/client-go/blob/46965213e4561ad1b9c585d1c3551a0cc8d3fcd6/rest/config.go#L310-L315
	restClientConfig.ContentConfig = rest.ContentConfig{
		GroupVersion:         &authv1.SchemeGroupVersion,
		NegotiatedSerializer: scheme.Codecs,
	}
	if err != nil {
		return nil, err
	}
	restClient, err := rest.RESTClientFor(restClientConfig)
	if err != nil {
		return nil, err
	}
	return restClient, nil
}

func generateKubeConfig(token, apiEndpoint, namespace string, insecureSkipTLSVerify bool) *clientcmdapi.Config {
	// create apiConfig based on the secret content
	return &clientcmdapi.Config{
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {
				Cluster:   "cluster",
				Namespace: namespace,
				AuthInfo:  "auth",
			},
		},
		CurrentContext: "ctx",
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {
				Server:                apiEndpoint,
				InsecureSkipTLSVerify: insecureSkipTLSVerify,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"auth": {
				Token: token,
			},
		},
	}
}

// waitForToolchainClusterSA waits for the toolchaincluster service account to be present
func waitForToolchainClusterSA(ctx *clicontext.CommandContext, cl runtimeclient.Client, toolchainClusterKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for ToolchainCluster SA %s to become ready", toolchainClusterKey)
		tc := &v1.ServiceAccount{}
		if err := cl.Get(ctx, toolchainClusterKey, tc); err != nil {
			if apierrors.IsNotFound(err) {
				// keep looking for the resource
				return false, nil
			}
			// exit if and error occurred
			return false, err
		}
		// exit if we found the resource
		return true, nil
	})
}

func waitUntilToolchainClusterReady(ctx *clicontext.CommandContext, cl runtimeclient.Client, toolchainClusterKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for ToolchainCluster %s to become ready", toolchainClusterKey)
		tc := &toolchainv1alpha1.ToolchainCluster{}
		if err := cl.Get(ctx, toolchainClusterKey, tc); err != nil {
			if apierrors.IsNotFound(err) {
				// keep looking for the resource
				return false, nil
			}
			// exit if and error occurred
			return false, err
		}

		return condition.IsTrue(tc.Status.Conditions, toolchainv1alpha1.ConditionReady), nil
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

type clusterData struct {
	client               runtimeclient.Client
	apiEndpoint          string
	namespace            string
	toolchainClusterName string
	kubeConfig           string
}

type registerMemberValidated struct {
	args              registerMemberArgs
	hostClusterData   clusterData
	memberClusterData clusterData
	warnings          []string
	errors            []string
}

func getApiEndpointAndClient(ctx *extendedCommandContext, kubeConfigPath string) (apiEndpoint string, cl runtimeclient.Client, err error) {
	var kubeConfig *clientcmdapi.Config
	var clientConfig *rest.Config

	kubeConfig, err = clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return
	}
	clientConfig, err = clientcmd.NewDefaultClientConfig(*kubeConfig, nil).ClientConfig()
	if err != nil {
		return
	}
	cl, err = ctx.NewClientFromRestConfig(clientConfig)
	if err != nil {
		return
	}
	apiEndpoint = getServerAPIEndpoint(kubeConfig)

	return
}

func validateArgs(ctx *extendedCommandContext, args registerMemberArgs) (*registerMemberValidated, error) {
	hostApiEndpoint, hostClusterClient, err := getApiEndpointAndClient(ctx, args.hostKubeConfig)
	if err != nil {
		return nil, err
	}

	memberApiEndpoint, memberClusterClient, err := getApiEndpointAndClient(ctx, args.memberKubeConfig)
	if err != nil {
		return nil, err
	}

	hostToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Host), hostApiEndpoint, "")
	if err != nil {
		return nil, err
	}

	// figure out the name that will be given to our new ToolchainCluster representing the member in the host cluster.
	membersInHost, err := getToolchainClustersWithHostname(ctx, hostClusterClient, memberApiEndpoint, args.hostNamespace)
	if err != nil {
		return nil, err
	}
	memberToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Member), memberApiEndpoint, args.nameSuffix)
	if err != nil {
		return nil, err
	}

	hostsInMember := &toolchainv1alpha1.ToolchainClusterList{}
	if err = memberClusterClient.List(ctx, hostsInMember, runtimeclient.InNamespace(args.memberNamespace)); err != nil {
		return nil, err
	}

	var warnings []string
	var errors []string

	if len(hostsInMember.Items) > 1 {
		errors = append(errors, fmt.Sprintf("member misconfigured: the member cluster (%s) is already registered with more than 1 host in namespace %s", memberApiEndpoint, args.memberNamespace))
	} else if len(hostsInMember.Items) == 1 {
		if hostsInMember.Items[0].Spec.APIEndpoint != hostApiEndpoint {
			errors = append(errors, fmt.Sprintf("the member is already registered with another host (%s) so registering it with the new one (%s) would result in an invalid configuration", hostsInMember.Items[0].Spec.APIEndpoint, hostApiEndpoint))
		}
		if hostsInMember.Items[0].Name != hostToolchainClusterName {
			errors = append(errors, fmt.Sprintf("the host is already in the member namespace using a ToolchainCluster object with the name '%s' but the new registration would use a ToolchainCluster with the name '%s' which would lead to an invalid configuration", hostsInMember.Items[0].Name, hostToolchainClusterName))
		}
	}
	existingMemberToolchainCluster := findToolchainClusterForMember(membersInHost, memberApiEndpoint, args.memberNamespace)
	if existingMemberToolchainCluster != nil {
		warnings = append(warnings, fmt.Sprintf("there already is a registered member for the same member API endpoint and operator namespace (%s), proceeding will overwrite the objects representing it in the host and member clusters", runtimeclient.ObjectKeyFromObject(existingMemberToolchainCluster)))
		if existingMemberToolchainCluster.Name != memberToolchainClusterName {
			errors = append(errors, fmt.Sprintf("the newly registered member cluster would have a different name (%s) than the already existing one (%s) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration", memberToolchainClusterName, existingMemberToolchainCluster.Name))
		}
	}

	return &registerMemberValidated{
		args: args,
		hostClusterData: clusterData{
			client:               hostClusterClient,
			apiEndpoint:          hostApiEndpoint,
			namespace:            args.hostNamespace,
			toolchainClusterName: hostToolchainClusterName,
			kubeConfig:           args.hostKubeConfig,
		},
		memberClusterData: clusterData{
			client:               memberClusterClient,
			apiEndpoint:          memberApiEndpoint,
			namespace:            args.memberNamespace,
			toolchainClusterName: memberToolchainClusterName,
			kubeConfig:           args.memberKubeConfig,
		},
		warnings: warnings,
		errors:   errors,
	}, nil
}

func (v *registerMemberValidated) confirmationPrompt() ioutils.ConfirmationMessage {
	// we have a single replacement argument at the moment so maybe this is a bit of
	// an overkill but, let's be explicit about using a format string and its arguments
	// so that mistakes are not made in the future when we update this stuff.
	sb := strings.Builder{}
	args := []any{}
	sb.WriteString("register the member cluster from kubeconfig %s?")
	args = append(args, v.args.memberKubeConfig)

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

func (v *registerMemberValidated) perform(ctx *extendedCommandContext) error {
	// add the host entry to the member cluster first. We assume that there is just 1 toolchain cluster entry in the member
	// cluster (i.e. it just points back to the host), so there's no need to determine the number of entries with the same
	// API endpoint.
	if err := v.addCluster(ctx, configuration.Host); err != nil {
		return err
	}

	// add the member entry in the host cluster
	if err := v.addCluster(ctx, configuration.Member); err != nil {
		return err
	}

	exampleSPC := &toolchainv1alpha1.SpaceProvisionerConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SpaceProvisionerConfig",
			APIVersion: toolchainv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.memberClusterData.toolchainClusterName,
			Namespace: v.hostClusterData.namespace,
		},
		Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
			ToolchainCluster: v.memberClusterData.toolchainClusterName,
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

`, v.hostClusterData.apiEndpoint))
}

func findToolchainClusterForMember(allToolchainClusters []toolchainv1alpha1.ToolchainCluster, memberAPIEndpoint, memberOperatorNamespace string) *toolchainv1alpha1.ToolchainCluster {
	for _, tc := range allToolchainClusters {
		if tc.Spec.APIEndpoint == memberAPIEndpoint && tc.Labels["namespace"] == memberOperatorNamespace {
			return &tc
		}
	}
	return nil
}
