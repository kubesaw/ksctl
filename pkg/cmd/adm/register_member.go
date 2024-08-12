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
		Short: "Executes add-cluster.sh script",
		Long:  `Downloads the 'add-cluster.sh' script from the 'toolchain-cicd' repo and calls it twice: once to register the Host cluster in the Member cluster and once to register the Member cluster in the host cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := newExtendedCommandContext(term, client.DefaultNewClientFromRestConfig)
			return registerMemberCluster(ctx, commandArgs)
		},
	}

	// keep these values in sync with the values in defaultRegisterMemberArgs() function in the tests.
	defaultTimeout := 2 * time.Minute
	defaultLetsEncrypt := true
	defaultNameSuffix := ""
	defaultHostNs := "toolchain-host-operator"
	defaultMemberNs := "toolchain-member-operator"

	cmd.Flags().StringVar(&commandArgs.hostKubeConfig, "host-kubeconfig", "", "Path to the kubeconfig file of the host cluster")
	flags.MustMarkRequired(cmd, "host-kubeconfig")
	cmd.Flags().StringVar(&commandArgs.memberKubeConfig, "member-kubeconfig", "", "Path to the kubeconfig file of the member cluster")
	flags.MustMarkRequired(cmd, "member-kubeconfig")
	cmd.Flags().BoolVar(&commandArgs.useLetsEncrypt, "lets-encrypt", defaultLetsEncrypt, fmt.Sprintf("Whether to use Let's Encrypt certificates or rely on the cluster certs (default: %t)", defaultLetsEncrypt))
	cmd.Flags().StringVar(&commandArgs.nameSuffix, "name-suffix", defaultNameSuffix, fmt.Sprintf("The suffix to append to the member name used when there are multiple members in a single cluster (default: '%s')", defaultNameSuffix))
	cmd.Flags().StringVar(&commandArgs.hostNamespace, "host-ns", defaultHostNs, fmt.Sprintf("The namespace of the host operator in the host cluster (default: '%s')", defaultHostNs))
	cmd.Flags().StringVar(&commandArgs.memberNamespace, "member-ns", defaultMemberNs, fmt.Sprintf("The namespace of the member operator in the member cluster (default: '%s')", defaultMemberNs))
	cmd.Flags().DurationVar(&commandArgs.waitForReadyTimeout, "timeout", defaultTimeout, fmt.Sprintf("The max timeout used when waiting for each of the computations to be completed. (default: '%d')", defaultTimeout))
	return cmd
}

func registerMemberCluster(ctx *extendedCommandContext, args registerMemberArgs) error {
	data, err := dataFromArgs(ctx, args)
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

// joiningCluster is the cluster that we want to connect to the joinToCluster.
// for example when we connect the member cluster to the host cluster the joiningCluster in this case is the member one, while the joinToCluster is the host cluster.
type joiningCluster struct {
	APIEndpoint, Name, KubeConfigPath, OperatorNamespace string
}

// clusterJoinTo is the cluster on which we create the secret and toolchaincluster cr for the joining cluster.
type clusterJoinTo struct {
	APIEndpoint, Name, OperatorNamespace string
	KubeClient                           runtimeclient.Client
}

func (v *registerMemberValidated) getJoiningAndJoinToClusters(joiningClusterType configuration.ClusterType) (*joiningCluster, *clusterJoinTo) {
	joiningC := &joiningCluster{}
	joinTo := &clusterJoinTo{}
	if joiningClusterType == configuration.Member {
		// in this case we are connecting the member cluster to the host cluster,
		// thus we fill joiningCluster variables with member cluster details
		// and clusterJoinTo with host details.
		joiningC.APIEndpoint = v.memberApiEndpoint
		joiningC.Name = v.memberToolchainClusterName
		joiningC.KubeConfigPath = v.args.memberKubeConfig
		joiningC.OperatorNamespace = v.args.memberNamespace
		joinTo.APIEndpoint = v.hostApiEndpoint
		joinTo.Name = v.hostToolchainClusterName
		joinTo.OperatorNamespace = v.args.hostNamespace
		joinTo.KubeClient = v.hostClusterClient
	} else {
		// in this case we are connecting the host cluster to the member cluster,
		// thus we fill joiningCluster variables with host cluster details
		// and clusterJoinTo with member details.
		joiningC.APIEndpoint = v.hostApiEndpoint
		joiningC.Name = v.hostToolchainClusterName
		joiningC.KubeConfigPath = v.args.hostKubeConfig
		joiningC.OperatorNamespace = v.args.hostNamespace
		joinTo.APIEndpoint = v.memberApiEndpoint
		joinTo.Name = v.memberToolchainClusterName
		joinTo.OperatorNamespace = v.args.memberNamespace
		joinTo.KubeClient = v.memberClusterClient
	}

	return joiningC, joinTo
}

// addCluster creates a secret and a ToolchainCluster resource on the `clusterJoinTo`.
// This ToolchainCluster CR stores a reference to the secret which contains the kubeconfig of the `joiningCluster`. Thus enables the `clusterJoinTo` to interact with the `joiningCluster`.
// - `clusterJoinTo` is the cluster where we create the ToolchainCluster resource and the secret
// - `joiningCluster` is the cluster referenced in the kubeconfig/ToolchainCluster of the `clusterJoinTo`
func (v *registerMemberValidated) addCluster(term ioutils.Terminal, SANamespacedName runtimeclient.ObjectKey, joiningClusterType configuration.ClusterType) error {
	if !term.AskForConfirmation(ioutils.WithMessagef("register the %s cluster by creating a ToolchainCluster CR, a Secret and a new ServiceAccount resource?", joiningClusterType)) {
		return nil
	}

	joiningClusterDetails, clusterJoinToDetails := v.getJoiningAndJoinToClusters(joiningClusterType)
	// joining cluster details
	term.Printlnf("API endpoint retrieved: %s", joiningClusterDetails.APIEndpoint)
	term.Printlnf("joining cluster name: %s", joiningClusterDetails.Name)

	// cluster join to details
	term.Printlnf("API endpoint of the cluster it is joining to: %s", clusterJoinToDetails.APIEndpoint)
	term.Printlnf("the cluster name it is joining to: %s", clusterJoinToDetails.Name)

	// generate a token that will be used for the kubeconfig
	joiningRestClient, err := newRestClient(joiningClusterDetails.KubeConfigPath)
	if err != nil {
		return err
	}
	token, err := generate.GetServiceAccountToken(joiningRestClient, SANamespacedName, TokenExpirationDays)
	if err != nil {
		return err
	}
	var insecureSkipTLSVerify bool
	if v.args.useLetsEncrypt {
		term.Printlnf("using let's encrypt certificate")
		insecureSkipTLSVerify = false
	} else {
		term.Printlnf("setting insecure skip tls verification flags")
		insecureSkipTLSVerify = true
	}
	// generate the kubeconfig that can be used by clusterJoinTo to interact with the joiningCluster
	generatedKubeConfig := generateKubeConfig(token, joiningClusterDetails.APIEndpoint, joiningClusterDetails.OperatorNamespace, insecureSkipTLSVerify)
	generatedKubeConfigFormatted, err := clientcmd.Write(*generatedKubeConfig)
	if err != nil {
		return err
	}

	// Create or Update the secret on the clusterJoinTo
	secretName := secretName(SANamespacedName, joiningClusterDetails.OperatorNamespace, joiningClusterDetails.Name)
	term.Printlnf("creating %s secret with name %s/%s", joiningClusterType, clusterJoinToDetails.OperatorNamespace, secretName)
	kubeConfigSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: clusterJoinToDetails.OperatorNamespace}}
	op, err := controllerutil.CreateOrUpdate(context.TODO(), clusterJoinToDetails.KubeClient, kubeConfigSecret, func() error {

		// update the secret label
		labels := kubeConfigSecret.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[toolchainv1alpha1.ToolchainClusterLabel] = joiningClusterDetails.Name
		kubeConfigSecret.Labels = labels

		// update the kubeconfig data
		kubeConfigSecret.StringData = map[string]string{
			"kubeconfig": string(generatedKubeConfigFormatted),
		}

		return nil
	})

	if err != nil {
		return err
	}
	term.Println("Secret successfully reconciled")
	term.Printlnf("operation", op)

	// TODO -- temporary logic
	// The creation of the toolchaincluster is just temporary until we implement https://issues.redhat.com/browse/KUBESAW-44,
	// the creation logic will be moved to the toolchaincluster_resource controller in toolchain-common and will be based on the secret created above.
	//
	// create/update toolchaincluster on the clusterJoinTo
	term.Printlnf("creating ToolchainCluster representation of %s in %s:", joiningClusterType, clusterJoinToDetails.Name)
	toolchainClusterCR := &toolchainv1alpha1.ToolchainCluster{ObjectMeta: metav1.ObjectMeta{Name: joiningClusterDetails.Name, Namespace: clusterJoinToDetails.OperatorNamespace}}
	op, err = controllerutil.CreateOrUpdate(context.TODO(), clusterJoinToDetails.KubeClient, toolchainClusterCR, func() error {

		// update the tc label
		labels := toolchainClusterCR.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["namespace"] = joiningClusterDetails.OperatorNamespace
		if joiningClusterType == "member" {
			labels["cluster-role.toolchain.dev.openshift.com/tenant"] = ""
		}

		toolchainClusterCR.Labels = labels
		toolchainClusterCR.Spec.APIEndpoint = joiningClusterDetails.APIEndpoint
		toolchainClusterCR.Spec.SecretRef.Name = secretName
		if insecureSkipTLSVerify {
			toolchainClusterCR.Spec.DisabledTLSValidations = []toolchainv1alpha1.TLSValidation{toolchainv1alpha1.TLSAll}
		}

		return nil
	})

	if err != nil {
		return err
	}
	term.Println("Toolchaincluster successfully reconciled")
	term.Printlnf("operation", op)
	// -- end temporary logic

	return err
}

func secretName(SANamespacedName runtimeclient.ObjectKey, joiningOperatorNamespace string, joiningClusterName string) string {
	secretName := SANamespacedName.Name + "-" + joiningOperatorNamespace + "-" + joiningClusterName
	return secretName
}

func newRestClient(kubeConfigPath string) (*rest.RESTClient, error) {
	restClientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
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

func generateKubeConfig(token, APIEndpoint, namespace string, insecureSkipTLSVerify bool) *clientcmdapi.Config {
	// create apiConfig based on the secret content
	clusters := make(map[string]*clientcmdapi.Cluster, 1)
	clusters["cluster"] = &clientcmdapi.Cluster{
		Server:                APIEndpoint,
		InsecureSkipTLSVerify: insecureSkipTLSVerify,
	}

	contexts := make(map[string]*clientcmdapi.Context, 1)
	contexts["ctx"] = &clientcmdapi.Context{
		Cluster:   "cluster",
		Namespace: namespace,
		AuthInfo:  "auth",
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo, 1)
	authinfos["auth"] = &clientcmdapi.AuthInfo{
		Token: token,
	}

	clientConfig := &clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "ctx",
		AuthInfos:      authinfos,
	}
	return clientConfig
}

// waitForToolchainClusterSA waits for the toolchaincluster service account to be present
func waitForToolchainClusterSA(ctx *clicontext.CommandContext, cl runtimeclient.Client, toolchainClusterKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for ToolchainCluster SA %s to become ready", toolchainClusterKey)
		tc := &v1.ServiceAccount{}
		if err := cl.Get(ctx, toolchainClusterKey, tc); err != nil {
			return false, err
		}

		return true, nil
	})
}

func waitUntilToolchainClusterReady(ctx *clicontext.CommandContext, cl runtimeclient.Client, toolchainClusterKey runtimeclient.ObjectKey, waitForReadyTimeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, waitForReadyTimeout, func() (bool, error) {
		ctx.Printlnf("waiting for ToolchainCluster %s to become ready", toolchainClusterKey)
		tc := &toolchainv1alpha1.ToolchainCluster{}
		if err := cl.Get(ctx, toolchainClusterKey, tc); err != nil {
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

type registerMemberData struct {
	hostClusterClient   runtimeclient.Client
	memberClusterClient runtimeclient.Client
	hostApiEndpoint     string
	memberApiEndpoint   string
	args                registerMemberArgs
}

type registerMemberValidated struct {
	registerMemberData
	hostToolchainClusterName   string
	memberToolchainClusterName string
	warnings                   []string
	errors                     []string
}

func dataFromArgs(ctx *extendedCommandContext, args registerMemberArgs) (*registerMemberData, error) {
	hostApiEndpoint, hostClusterClient, err := getApiEndpointAndClient(ctx, args.hostKubeConfig)
	if err != nil {
		return nil, err
	}

	memberApiEndpoint, memberClusterClient, err := getApiEndpointAndClient(ctx, args.memberKubeConfig)
	if err != nil {
		return nil, err
	}

	return &registerMemberData{
		args:                args,
		hostApiEndpoint:     hostApiEndpoint,
		memberApiEndpoint:   memberApiEndpoint,
		hostClusterClient:   hostClusterClient,
		memberClusterClient: memberClusterClient,
	}, nil
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

func (d *registerMemberData) validate(ctx *extendedCommandContext) (*registerMemberValidated, error) {
	hostToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Host), d.hostApiEndpoint, "")
	if err != nil {
		return nil, err
	}

	// figure out the name that will be given to our new ToolchainCluster representing the member in the host cluster.
	// This is the same name that the add-cluster.sh script will deduce and use.
	membersInHost, err := getToolchainClustersWithHostname(ctx, d.hostClusterClient, d.memberApiEndpoint, d.args.hostNamespace)
	if err != nil {
		return nil, err
	}
	memberToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Member), d.memberApiEndpoint, d.args.nameSuffix)
	if err != nil {
		return nil, err
	}

	hostsInMember := &toolchainv1alpha1.ToolchainClusterList{}
	if err = d.memberClusterClient.List(ctx, hostsInMember, runtimeclient.InNamespace(d.args.memberNamespace)); err != nil {
		return nil, err
	}

	var warnings []string
	var errors []string

	if len(hostsInMember.Items) > 1 {
		errors = append(errors, fmt.Sprintf("member misconfigured: the member cluster (%s) is already registered with more than 1 host in namespace %s", d.memberApiEndpoint, d.args.memberNamespace))
	} else if len(hostsInMember.Items) == 1 {
		if hostsInMember.Items[0].Spec.APIEndpoint != d.hostApiEndpoint {
			errors = append(errors, fmt.Sprintf("the member is already registered with another host (%s) so registering it with the new one (%s) would result in an invalid configuration", hostsInMember.Items[0].Spec.APIEndpoint, d.hostApiEndpoint))
		}
		if hostsInMember.Items[0].Name != hostToolchainClusterName {
			errors = append(errors, fmt.Sprintf("the host is already in the member namespace using a ToolchainCluster object with the name '%s' but the new registration would use a ToolchainCluster with the name '%s' which would lead to an invalid configuration", hostsInMember.Items[0].Name, hostToolchainClusterName))
		}
	}
	existingMemberToolchainCluster := findToolchainClusterForMember(membersInHost, d.memberApiEndpoint, d.args.memberNamespace)
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
	// wait for the toolchaincluster-member SA to be ready
	toolchainClusterSAKey := runtimeclient.ObjectKey{
		Name:      "toolchaincluster-" + string(configuration.Member),
		Namespace: v.args.memberNamespace,
	}
	if err := waitForToolchainClusterSA(ctx.CommandContext, v.memberClusterClient, toolchainClusterSAKey, v.args.waitForReadyTimeout); err != nil {
		ctx.Println("The toolchaincluster-member ServiceAccount in the member cluster is not present.")
		ctx.Printlnf("Please check the %s ToolchainCluster ServiceAccount in the %s member cluster.", toolchainClusterSAKey, v.memberApiEndpoint)
		return err
	}
	// add the host entry to the member cluster first. We assume that there is just 1 toolchain cluster entry in the member
	// cluster (i.e. it just points back to the host), so there's no need to determine the number of entries with the same
	// API endpoint.
	if err := v.addCluster(ctx, toolchainClusterSAKey, configuration.Host); err != nil {
		return err
	}
	hostToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      v.hostToolchainClusterName,
		Namespace: v.args.memberNamespace,
	}
	if err := waitUntilToolchainClusterReady(ctx.CommandContext, v.memberClusterClient, hostToolchainClusterKey, v.args.waitForReadyTimeout); err != nil {
		ctx.Println("The ToolchainCluster resource representing the host in the member cluster has not become ready.")
		ctx.Printlnf("Please check the %s ToolchainCluster resource in the %s member cluster.", hostToolchainClusterKey, v.memberApiEndpoint)
		return err
	}

	// wait for the toolchaincluster-host SA to be ready
	toolchainClusterSAKey = runtimeclient.ObjectKey{
		Name:      "toolchaincluster-" + string(configuration.Host),
		Namespace: v.args.hostNamespace,
	}
	if err := waitForToolchainClusterSA(ctx.CommandContext, v.hostClusterClient, toolchainClusterSAKey, v.args.waitForReadyTimeout); err != nil {
		ctx.Println("The toolchaincluster-host ServiceAccount in the host cluster is not present.")
		ctx.Printlnf("Please check the %s ToolchainCluster ServiceAccount in the %s host cluster.", toolchainClusterSAKey, v.hostApiEndpoint)
		return err
	}
	// add the member entry in the host cluster
	memberToolchainClusterKey := runtimeclient.ObjectKey{
		Name:      v.memberToolchainClusterName,
		Namespace: v.args.hostNamespace,
	}
	if err := v.addCluster(ctx, toolchainClusterSAKey, configuration.Member); err != nil {
		return err
	}

	if err := waitUntilToolchainClusterReady(ctx.CommandContext, v.hostClusterClient, memberToolchainClusterKey, v.args.waitForReadyTimeout); err != nil {
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
