package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/pkg/utils"
	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/pointer"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type generateFlags struct {
	kubeSawAdminsFile, outDir string
	dev                       bool
	kubeconfigs               []string
}

func NewCliConfigsCmd() *cobra.Command {
	f := generateFlags{}
	command := &cobra.Command{
		Use:   "cli-configs --kubesaw-admins=<path-to-kubesaw-admins-file>",
		Short: "Generate ksctl.yaml files",
		Long:  `Generate ksctl.yaml files, that is used by ksctl, for every ServiceAccount defined in the given kubesaw-admins.yaml file`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			return generate(term, f, DefaultNewExternalClientFromConfig)
		},
	}
	command.Flags().StringVarP(&f.kubeSawAdminsFile, "kubesaw-admins", "c", "", "Use the given kubesaw-admin file")
	flags.MustMarkRequired(command, "kubesaw-admins")
	command.Flags().BoolVarP(&f.dev, "dev", "d", false, "If running in a dev cluster")

	configDirPath := fmt.Sprintf("%s/src/github.com/kubesaw/ksctl/out/config", os.Getenv("GOPATH"))
	command.Flags().StringVarP(&f.outDir, "out-dir", "o", configDirPath, "Directory where generated ksctl.yaml files should be stored")

	defaultKubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	command.Flags().StringSliceVarP(&f.kubeconfigs, "kubeconfig", "k", []string{defaultKubeconfigPath}, "Kubeconfig(s) for managing multiple clusters and the access to them - paths should be comma separated when using multiple of them. "+
		"In dev mode, the first one has to represent the host cluster.")

	return command
}

type NewRESTClientFromConfigFunc func(config *rest.Config) (*rest.RESTClient, error)

type NewClientFromConfigFunc func(config *rest.Config, options runtimeclient.Options) (runtimeclient.Client, error)

var DefaultNewExternalClientFromConfig = func(config *rest.Config) (*rest.RESTClient, error) {
	if config.GroupVersion == nil {
		config.GroupVersion = &authv1.SchemeGroupVersion
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = scheme.Codecs
	}
	return rest.RESTClientFor(config)
}

func generate(term ioutils.Terminal, flags generateFlags, newExternalClient NewRESTClientFromConfigFunc) error {
	if err := client.AddToScheme(); err != nil {
		return err
	}

	// Get the unmarshalled version of kubesaw-admins.yaml
	kubeSawAdmins, err := assets.GetKubeSawAdminsConfig(flags.kubeSawAdminsFile)
	if err != nil {
		return errs.Wrapf(err, "unable get kubesaw-admins.yaml file from %s", flags.kubeSawAdminsFile)
	}

	ctx := &generateContext{
		Terminal:        term,
		newRESTClient:   newExternalClient,
		kubeSawAdmins:   kubeSawAdmins,
		kubeconfigPaths: flags.kubeconfigs,
	}

	// ksctlConfigsPerName contains all ksctlConfig objects that will be marshalled to ksctl.yaml files
	ksctlConfigsPerName := map[string]configuration.KsctlConfig{}

	// use host API either from the kubesaw-admins.yaml or from kubeconfig if --dev flag was used
	hostSpec := kubeSawAdmins.Clusters.Host
	if flags.dev {
		term.Printlnf("Using kubeconfig located at '%s' for retrieving the host cluster information...", flags.kubeconfigs[0])
		kubeconfig, err := clientcmd.BuildConfigFromFlags("", flags.kubeconfigs[0])
		if err != nil {
			return errs.Wrapf(err, "unable to build kubeconfig")
		}
		hostSpec.API = kubeconfig.Host
	}

	// firstly generate for the host cluster
	if err := generateForCluster(ctx, configuration.Host, "host", hostSpec, ksctlConfigsPerName); err != nil {
		return err
	}

	// and then based on the data from kubesaw-admins.yaml files generate also all members
	for _, member := range kubeSawAdmins.Clusters.Members {

		// use either the member API from kubesaw-admins.yaml file or use the same as API as for host if --dev flag was used
		memberSpec := member.ClusterConfig
		if flags.dev {
			memberSpec.API = hostSpec.API
		}

		if err := generateForCluster(ctx, configuration.Member, member.Name, memberSpec, ksctlConfigsPerName); err != nil {
			return err
		}
	}

	return writeKsctlConfigs(term, flags.outDir, ksctlConfigsPerName)
}

func serverName(API string) string {
	return strings.Split(strings.Split(API, "api.")[1], ":")[0]
}

// writeKsctlConfigs marshals the given KsctlConfig objects and stored them in sandbox-sre/out/config/<name>/ directories
func writeKsctlConfigs(term ioutils.Terminal, configDirPath string, ksctlConfigsPerName map[string]configuration.KsctlConfig) error {
	if err := os.RemoveAll(configDirPath); err != nil {
		return err
	}
	for name, ksctlConfig := range ksctlConfigsPerName {
		pathDir := fmt.Sprintf("%s/%s", configDirPath, name)
		if err := os.MkdirAll(pathDir, 0744); err != nil {
			return err
		}
		content, err := yaml.Marshal(ksctlConfig)
		if err != nil {
			return err
		}
		path := pathDir + "/ksctl.yaml"
		if err := os.WriteFile(path, content, 0600); err != nil {
			return err
		}
		term.Printlnf("ksctl.yaml file for %s was stored in %s", name, path)
	}
	return nil
}

type generateContext struct {
	ioutils.Terminal
	newRESTClient   NewRESTClientFromConfigFunc
	kubeSawAdmins   *assets.KubeSawAdmins
	kubeconfigPaths []string
}

// contains tokens mapped by SA name
type tokenPerSA map[string]string

func generateForCluster(ctx *generateContext, clusterType configuration.ClusterType, clusterName string, clusterSpec assets.ClusterConfig, ksctlConfigsPerName map[string]configuration.KsctlConfig) error {
	ctx.PrintContextSeparatorf("Generating the content of the ksctl.yaml files for %s cluster running at %s", clusterName, clusterSpec.API)

	// find config we can build client for the cluster from
	externalClient, err := buildClientFromKubeconfigFiles(ctx, clusterSpec.API, ctx.kubeconfigPaths, sandboxSRENamespace(clusterType))
	if err != nil {
		return err
	}

	clusterDef := configuration.ClusterDefinition{
		ClusterType: clusterType,
		ServerName:  serverName(clusterSpec.API),
		ServerAPI:   clusterSpec.API,
	}

	tokenPerSAName := tokenPerSA{}

	for _, sa := range ctx.kubeSawAdmins.ServiceAccounts {
		for saClusterType := range sa.PermissionsPerClusterType {
			if saClusterType != clusterType.String() {
				continue
			}
			saNamespace := sandboxSRENamespace(clusterType)
			if sa.Namespace != "" {
				saNamespace = sa.Namespace
			}
			ctx.Printlnf("Getting token for SA '%s' in namespace '%s'", sa.Name, saNamespace)
			token, err := getServiceAccountToken(externalClient, types.NamespacedName{
				Namespace: saNamespace,
				Name:      sa.Name})
			if token == "" || err != nil {
				return err
			}
			tokenPerSAName[sa.Name] = token
		}
	}

	addToKsctlConfigs(clusterDef, clusterName, ksctlConfigsPerName, tokenPerSAName)

	return nil
}

// buildClientFromKubeconfigFiles goes through the list of kubeconfigs and tries to build the runtimeclient.Client & rest.RESTClient.
// As soon as the build is successful, then it returns the built instances. If the build fails for all of the kubeconfig files, then it returns an error.
func buildClientFromKubeconfigFiles(ctx *generateContext, API string, kubeconfigPaths []string, saNamespace string) (*rest.RESTClient, error) {
	for _, kubeconfigPath := range kubeconfigPaths {
		kubeconfig, err := clientcmd.BuildConfigFromFlags(API, kubeconfigPath)
		if err != nil {
			ctx.Printlnf("Unable to build config from kubeconfig file located at '%s' for the cluster '%s': %s", kubeconfigPath, API, err.Error())
			ctx.Printlnf("trying next one...")
			continue
		}
		externalCl, err := ctx.newRESTClient(kubeconfig)
		if err != nil {
			ctx.Printlnf("Unable to build config from kubeconfig file located at '%s' for the cluster '%s': %s", kubeconfigPath, API, err.Error())
			ctx.Printlnf("trying next one...")
			continue
		}
		sas := &v1.ServiceAccountList{}
		if err := externalCl.Get().
			AbsPath(fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/", saNamespace)).
			Do(context.TODO()).Into(sas); err != nil {
			ctx.Printlnf("Unable to use restclient built with kubeconfig file located at '%s' for the cluster '%s': %s", kubeconfigPath, API, err.Error())
			ctx.Printlnf("trying next one...")
			continue
		}
		ctx.Printlnf("Using kubeconfig file located at '%s' for the cluster '%s'", kubeconfigPath, API)
		return externalCl, nil
	}
	return nil, fmt.Errorf("could not setup client from any of the provided kubeconfig files for the '%s' cluster", API)
}

// getServiceAccountToken returns the SA's token or returns an error if none was found.
// NOTE: due to a changes in OpenShift 4.11, tokens are not listed as `secrets` in ServiceAccounts.
// The recommended solution is to use the TokenRequest API when server version >= 4.11
// (see https://docs.openshift.com/container-platform/4.11/release_notes/ocp-4-11-release-notes.html#ocp-4-11-notable-technical-changes)
func getServiceAccountToken(cl *rest.RESTClient, namespacedName types.NamespacedName) (string, error) {
	tokenRequest := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			ExpirationSeconds: pointer.Int64(int64(365 * 24 * 60 * 60)), // token will be valid for 1 year
		},
	}
	result := &authv1.TokenRequest{}
	if err := cl.Post().
		AbsPath(fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/%s/token", namespacedName.Namespace, namespacedName.Name)).
		Body(tokenRequest).
		Do(context.TODO()).
		Into(result); err != nil {
		return "", err
	}
	return result.Status.Token, nil
}

// addToKsctlConfigs adds to ksctlConfig objects information about the cluster as well as the SA token
func addToKsctlConfigs(clusterDev configuration.ClusterDefinition, clusterName string, ksctlConfigsPerName map[string]configuration.KsctlConfig, tokensPerSA tokenPerSA) {
	for name, token := range tokensPerSA {
		if _, ok := ksctlConfigsPerName[name]; !ok {
			ksctlConfigsPerName[name] = configuration.KsctlConfig{
				Name:                     name,
				ClusterAccessDefinitions: map[string]configuration.ClusterAccessDefinition{},
			}
		}
		clusterName := utils.KebabToCamelCase(clusterName)
		ksctlConfigsPerName[name].ClusterAccessDefinitions[clusterName] = configuration.ClusterAccessDefinition{
			ClusterDefinition: clusterDev,
			Token:             token,
		}
	}
}
