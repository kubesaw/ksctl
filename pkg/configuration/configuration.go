package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/pkg/utils"

	"github.com/mitchellh/go-homedir"
	errs "github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var (
	ConfigFileFlag string
	Verbose        bool
)

type KsctlConfig struct {
	ClusterAccessDefinitions `yaml:",inline"`
	Name                     string `yaml:"name"`
}

// Load reads in config file and ENV variables if set.
func Load(term ioutils.Terminal) (KsctlConfig, error) {
	path := ConfigFileFlag
	if path == "" {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			return KsctlConfig{}, errs.Wrap(err, "unable to read home directory")
		}
		path = filepath.Join(home, ".ksctl.yaml")

		if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
			if _, err := os.Stat(filepath.Join(home, ".sandbox.yaml")); err != nil && !os.IsNotExist(err) {
				return KsctlConfig{}, err
			} else if err == nil {
				path = filepath.Join(home, ".sandbox.yaml")
				term.Println("The default location of ~/.sandbox.yaml file is deprecated. Rename it to ~/.ksctl.yaml")
			}
		} else if err != nil {
			return KsctlConfig{}, err
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return KsctlConfig{}, errs.Wrapf(err, "unable to read the file '%s'", path)
	}
	if info.IsDir() {
		return KsctlConfig{}, fmt.Errorf("the '%s' is not file but a directory", path)
	}

	if Verbose {
		term.Printlnf("Using config file: '%s'", path)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return KsctlConfig{}, err
	}
	ksctlConfig := KsctlConfig{}
	if err := yaml.Unmarshal(bytes, &ksctlConfig); err != nil {
		return KsctlConfig{}, err
	}
	return ksctlConfig, nil
}

const HostName = "host"

type ClusterType string

var Host ClusterType = "host"
var Member ClusterType = "member"
var ClusterTypes = []ClusterType{Host, Member}

func (cluster ClusterType) String() string {
	return string(cluster)
}

func (cluster ClusterType) TheOtherType() ClusterType {
	if cluster == Host {
		return Member
	}
	return Host
}

func (cluster ClusterType) AsSuffix(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, cluster)
}

type ClusterAccessDefinitions map[string]ClusterAccessDefinition

type ClusterDefinition struct {
	ClusterType ClusterType `yaml:"clusterType"`
	ServerAPI   string      `yaml:"serverAPI"`
	ServerName  string      `yaml:"serverName"`
}

type ClusterAccessDefinition struct {
	ClusterDefinition `yaml:",inline"`
	Token             string `yaml:"token"`
}

type ClusterNamespaces map[string]string

// LoadClusterAccessDefinition loads ClusterAccessDefinition object from the config file and checks that all required parameters are set
func LoadClusterAccessDefinition(term ioutils.Terminal, clusterName string) (ClusterAccessDefinition, error) {
	ksctlConfig, err := Load(term)
	if err != nil {
		return ClusterAccessDefinition{}, err
	}
	return loadClusterAccessDefinition(ksctlConfig, clusterName)
}

func loadClusterAccessDefinition(ksctlConfig KsctlConfig, clusterName string) (ClusterAccessDefinition, error) {
	// try converted to camel case if kebab case is provided
	clusterDef, ok := ksctlConfig.ClusterAccessDefinitions[utils.KebabToCamelCase(clusterName)]
	if !ok {
		// if not found, then also try original format (to cover situation when camel case is used)
		if clusterDef, ok = ksctlConfig.ClusterAccessDefinitions[clusterName]; !ok {
			return ClusterAccessDefinition{}, fmt.Errorf("the provided cluster-name '%s' is not present in your ksctl.yaml file. The available cluster names are\n"+
				"------------------------\n%s\n"+
				"------------------------", clusterName, strings.Join(getAllClusterNames(ksctlConfig), "\n"))
		}
	}
	if clusterDef.ClusterType == "" {
		return ClusterAccessDefinition{}, fmt.Errorf("ksctl command failed: 'cluster type' is not set for cluster '%s'", clusterName)
	}
	if clusterDef.ServerAPI == "" {
		return ClusterAccessDefinition{}, fmt.Errorf("ksctl command failed: The server API is not set for the cluster %s", clusterName)
	}
	if clusterDef.ServerName == "" {
		return ClusterAccessDefinition{}, fmt.Errorf("ksctl command failed: The server name is not set for the cluster %s", clusterName)
	}
	return clusterDef, nil
}

func getAllClusterNames(config KsctlConfig) []string {
	var clusterNames []string
	for clusterName := range config.ClusterAccessDefinitions {
		clusterNames = append(clusterNames, utils.CamelCaseToKebabCase(clusterName))
	}
	sort.Strings(clusterNames)
	return clusterNames
}

// ClusterConfig contains all parameters of a cluster loaded from KsctlConfig
// plus all cluster names defined in the KsctlConfig
type ClusterConfig struct {
	ClusterAccessDefinition
	AllClusterNames   []string
	ClusterName       string
	Token             string
	OperatorNamespace string // namespace where either the host-operator or the member-operator is deployed (depends on the cluster context)
}

// LoadClusterConfig loads ClusterConfig object from the config file and checks that all required parameters are set
// as well as the token for the given name
func LoadClusterConfig(term ioutils.Terminal, clusterName string) (ClusterConfig, error) {
	ksctlConfig, err := Load(term)
	if err != nil {
		return ClusterConfig{}, err
	}
	clusterDef, err := loadClusterAccessDefinition(ksctlConfig, clusterName)
	if err != nil {
		return ClusterConfig{}, err
	}
	if clusterDef.Token == "" {
		return ClusterConfig{}, fmt.Errorf("ksctl command failed: the token in your ksctl.yaml file is missing")
	}
	var operatorNamespace string
	if clusterName == HostName {
		operatorNamespace = os.Getenv("HOST_OPERATOR_NAMESPACE")
		if operatorNamespace == "" {
			operatorNamespace = "toolchain-host-operator"
		}
	} else {
		operatorNamespace = os.Getenv("MEMBER_OPERATOR_NAMESPACE")
		if operatorNamespace == "" {
			operatorNamespace = "toolchain-member-operator"
		}
	}

	if Verbose {
		term.Printlnf("Using '%s' configuration for '%s' cluster running at '%s' and in namespace '%s'\n",
			clusterName, clusterDef.ServerName, clusterDef.ServerAPI, operatorNamespace)
	}
	return ClusterConfig{
		ClusterAccessDefinition: clusterDef,
		AllClusterNames:         getAllClusterNames(ksctlConfig),
		ClusterName:             clusterName,
		Token:                   clusterDef.Token,
		OperatorNamespace:       operatorNamespace,
	}, nil
}

// GetServerParam returns the `--server=` param along with its actual value
func (c ClusterConfig) GetServerParam() string {
	return "--server=" + c.ServerAPI
}

// GetNamespaceParam returns the `--namespace=` param along with its actual value
func (c ClusterConfig) GetNamespaceParam() string {
	return "--namespace=" + c.OperatorNamespace
}

// GetMemberClusterName returns the full name of the member cluster (used in ToolchainCluster CRs)
// for the provided shot cluster name such as member-1 (used in ksctl.yaml)
func GetMemberClusterName(term ioutils.Terminal, ctlClusterName string) (string, error) {
	memberClusterConfig, err := LoadClusterConfig(term, ctlClusterName)
	if err != nil {
		return "", err
	}
	// target cluster must have 'member' cluster type
	if memberClusterConfig.ClusterType != Member {
		return "", fmt.Errorf("expected target cluster to have clusterType '%s', actual: '%s'", Member, memberClusterConfig.ClusterType)
	}
	return "member-" + memberClusterConfig.ServerName, nil
}
