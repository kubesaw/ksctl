package test

import (
	"os"
	"testing"

	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ClusterDefinitionWithName struct {
	configuration.ClusterAccessDefinition
	ClusterName string
}

// ConfigOption an option on the configuration generated for a test
type ConfigOption func(*ClusterDefinitionWithName)

// NoToken deletes the default token set for the cluster
func NoToken() ConfigOption {
	return func(content *ClusterDefinitionWithName) {
		content.Token = ""
	}
}

// ServerAPI specifies the ServerAPI to use (default is `https://cool-server.com`)
func ServerAPI(serverAPI string) ConfigOption {
	return func(content *ClusterDefinitionWithName) {
		content.ServerAPI = serverAPI
	}
}

// ClusterName specifies the name of the server (default is `host` or `member1`)
func ClusterName(clusterName string) ConfigOption {
	return func(content *ClusterDefinitionWithName) {
		content.ClusterName = clusterName
	}
}

// ServerName specifies the name of the server (default is `cool-server.com`)
func ServerName(serverName string) ConfigOption {
	return func(content *ClusterDefinitionWithName) {
		content.ServerName = serverName
	}
}

// ClusterType specifies the cluster type (`host` or `member`)
func ClusterType(clusterType string) ConfigOption {
	return func(content *ClusterDefinitionWithName) {
		content.ClusterType = configuration.ClusterType(clusterType)
	}
}

// Host defines the configuration for the host cluster
func Host(options ...ConfigOption) ClusterDefinitionWithName {
	clusterDef := ClusterDefinitionWithName{
		ClusterName: "host",
		ClusterAccessDefinition: configuration.ClusterAccessDefinition{
			ClusterDefinition: configuration.ClusterDefinition{
				ServerAPI:   "https://cool-server.com",
				ServerName:  "cool-server.com",
				ClusterType: configuration.Host,
			},
			Token: "cool-token",
		},
	}
	return WithValues(clusterDef, options...)
}

func HostKubeConfig() *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()

	cluster := clientcmdapi.NewCluster()
	cfg.Clusters["host"] = cluster
	cluster.Server = "https://cool-server.com"

	context := clientcmdapi.NewContext()
	cfg.Contexts["host"] = context
	context.Cluster = "host"
	context.Namespace = "toolchain-host-operator"

	cfg.CurrentContext = "host"

	return cfg
}

// Member defines the configuration for a member cluster
func Member(options ...ConfigOption) ClusterDefinitionWithName {
	clusterDef := ClusterDefinitionWithName{
		ClusterName: "member1",
		ClusterAccessDefinition: configuration.ClusterAccessDefinition{
			ClusterDefinition: configuration.ClusterDefinition{
				ServerAPI:   "https://cool-server.com",
				ServerName:  "cool-server.com",
				ClusterType: configuration.Member,
			},
			Token: "cool-token",
		},
	}
	return WithValues(clusterDef, options...)
}

func MemberKubeConfig() *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()

	cluster := clientcmdapi.NewCluster()
	cfg.Clusters["member"] = cluster
	cluster.Server = "https://cool-server.com"

	context := clientcmdapi.NewContext()
	cfg.Contexts["member"] = context
	context.Cluster = "member"
	context.Namespace = "toolchain-member-operator"

	cfg.CurrentContext = "member"

	return cfg
}

// WithValues applies the options on the given parameters
func WithValues(clusterDef ClusterDefinitionWithName, options ...ConfigOption) ClusterDefinitionWithName {
	for _, modify := range options {
		modify(&clusterDef)
	}
	return clusterDef
}

// NewKsctlConfig creates KsctlConfig object with the given cluster definitions
func NewKsctlConfig(clusterDefs ...ClusterDefinitionWithName) configuration.KsctlConfig {
	ksctlConfig := configuration.KsctlConfig{
		Name:                     "john",
		ClusterAccessDefinitions: map[string]configuration.ClusterAccessDefinition{},
	}
	for _, clusterDefWithName := range clusterDefs {
		ksctlConfig.ClusterAccessDefinitions[clusterDefWithName.ClusterName] = clusterDefWithName.ClusterAccessDefinition
	}
	return ksctlConfig
}

// SetFileConfig generates the configuration file to use during a test
// The file is automatically cleanup at the end of the test.
func SetFileConfig(t *testing.T, clusterDefs ...ClusterDefinitionWithName) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "configFile-*.yaml")
	require.NoError(t, err)
	fileName := tmpFile.Name()
	t.Cleanup(func() {
		err := os.Remove(fileName)
		require.NoError(t, err)
		configuration.ConfigFileFlag = ""
	})

	ksctlConfig := NewKsctlConfig(clusterDefs...)
	out, err := yaml.Marshal(ksctlConfig)
	require.NoError(t, err)
	err = os.WriteFile(fileName, out, 0600)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	configuration.ConfigFileFlag = fileName
	t.Logf("config file: %s: \n%s", fileName, string(out))
}

func PersistKubeConfigFile(t *testing.T, config *clientcmdapi.Config) string {
	tmpFile, err := os.CreateTemp(os.TempDir(), "kubeconfig-*.yaml")
	require.NoError(t, err)
	// it is important to use clientcmd.WriteToFile instead of just YAML marshalling,
	// because clientcmd uses custom encoders and decoders for the config object.
	require.NoError(t, clientcmd.WriteToFile(*config, tmpFile.Name()))

	return tmpFile.Name()
}
