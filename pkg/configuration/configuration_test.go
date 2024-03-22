package configuration_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadClusterConfig(t *testing.T) {
	testCases := []struct {
		caseName  string
		transform func(string) string
	}{
		{"kebab", utils.CamelCaseToKebabCase},
		{"camel", utils.KebabToCamelCase},
	}
	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("when using %s case", testCase.caseName), func(t *testing.T) {

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when values are set for "+clusterConfigParam.ClusterName, func(t *testing.T) {

					// reset after tests below finished
					v := configuration.Verbose
					defer func() {
						configuration.Verbose = v
					}()

					t.Run("with verbose logs", func(t *testing.T) {
						// given
						SetFileConfig(t, clusterConfigParam)
						namespaceName := fmt.Sprintf("toolchain-%s-operator", clusterConfigParam.ClusterType)
						term := NewFakeTerminal()
						term.Tee(os.Stdout)
						configuration.Verbose = true

						// when
						cfg, err := configuration.LoadClusterConfig(term, clusterName)

						// then
						require.NoError(t, err)
						assert.Equal(t, namespaceName, cfg.OperatorNamespace)
						assert.Equal(t, "--namespace="+namespaceName, cfg.GetNamespaceParam())
						assert.Equal(t, clusterConfigParam.ClusterType, cfg.ClusterType)
						assert.Equal(t, "cool-token", cfg.Token)
						assert.Equal(t, "https://cool-server.com", cfg.ServerAPI)
						assert.Equal(t, "--server=https://cool-server.com", cfg.GetServerParam())
						assert.Equal(t, "cool-server.com", cfg.ServerName)
						assert.Len(t, cfg.AllClusterNames, 1)
						assert.Contains(t, cfg.AllClusterNames, utils.CamelCaseToKebabCase(clusterName))
						assert.Contains(t, term.Output(), fmt.Sprintf("Using config file: '%s'", configuration.ConfigFileFlag))
						assert.Contains(t, term.Output(), fmt.Sprintf("Using '%s' configuration for '%s' cluster running at '%s' and in namespace '%s'",
							cfg.ClusterName, cfg.ServerName, cfg.ServerAPI, cfg.OperatorNamespace))
					})

					t.Run("without verbose logs", func(t *testing.T) {
						// given
						SetFileConfig(t, clusterConfigParam)
						term := NewFakeTerminal()
						configuration.Verbose = false

						// when
						cfg, err := configuration.LoadClusterConfig(term, clusterName)

						// then
						require.NoError(t, err)
						// don't repeat assertions above, just check that logs do NOT contain the following messages
						assert.NotContains(t, term.Output(), fmt.Sprintf("Using config file: '%s'", configuration.ConfigFileFlag))
						assert.NotContains(t, term.Output(), fmt.Sprintf("Using '%s' configuration for '%s' cluster running at '%s' and in namespace '%s'",
							cfg.ClusterType, cfg.ServerName, cfg.ServerAPI, cfg.OperatorNamespace))
					})
				})
			}

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when kubeSaw namespace is set via global variable for "+clusterConfigParam.ClusterName, func(t *testing.T) {
					// given
					restore := test.SetEnvVarAndRestore(t, strings.ToUpper(clusterConfigParam.ClusterType.String())+"_OPERATOR_NAMESPACE", "custom-namespace")
					t.Cleanup(restore)
					SetFileConfig(t, WithValues(clusterConfigParam))
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.NoError(t, err, "ksctl command failed: The kubeSaw namespace is not set for the cluster "+clusterConfigParam.ClusterName)
					assert.Equal(t, "custom-namespace", cfg.OperatorNamespace)
				})
			}

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when clusterType is not set for "+clusterConfigParam.ClusterName, func(t *testing.T) {
					// given
					SetFileConfig(t, WithValues(clusterConfigParam, ClusterType("")))
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.EqualError(t, err, "ksctl command failed: 'cluster type' is not set for cluster '"+clusterConfigParam.ClusterName+"'")
					assert.Empty(t, cfg.ClusterType)
				})
			}

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(NoToken()), Member(NoToken())} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when token is not set for "+clusterName, func(t *testing.T) {
					// given
					SetFileConfig(t, clusterConfigParam)
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
					assert.Empty(t, cfg.Token)
				})
			}

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when server api is not set for "+clusterName, func(t *testing.T) {
					// given
					SetFileConfig(t, WithValues(clusterConfigParam, ServerAPI("")))
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.EqualError(t, err, "ksctl command failed: The server API is not set for the cluster "+clusterName)
					assert.Empty(t, cfg.ServerAPI)
				})
			}

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when server name is not set for "+clusterName, func(t *testing.T) {
					// given
					SetFileConfig(t, WithValues(clusterConfigParam, ServerName("")))
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.EqualError(t, err, "ksctl command failed: The server name is not set for the cluster "+clusterName)
					assert.Empty(t, cfg.ServerName)
				})
			}

			t.Run("when no cluster name is defined", func(t *testing.T) {
				// given
				SetFileConfig(t)
				term := NewFakeTerminal()

				// when
				_, err := configuration.LoadClusterConfig(term, "dummy")

				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "the provided cluster-name 'dummy' is not present in your ksctl.yaml file. The available cluster names are")
			})

			for _, clusterConfigParam := range []ClusterDefinitionWithName{Host(), Member()} {
				clusterConfigParam.ClusterName = testCase.transform(clusterConfigParam.ClusterName)
				clusterName := clusterConfigParam.ClusterName

				t.Run("when multiple cluster names are defined", func(t *testing.T) {
					// given
					SetFileConfig(t, Host(), Member(), Member(ClusterName("member2")))
					term := NewFakeTerminal()

					// when
					cfg, err := configuration.LoadClusterConfig(term, clusterName)

					// then
					require.NoError(t, err)
					assert.Len(t, cfg.AllClusterNames, 3)
					assert.Contains(t, cfg.AllClusterNames, "host")
					assert.Contains(t, cfg.AllClusterNames, "member-1")
					assert.Contains(t, cfg.AllClusterNames, "member-2")
				})
			}
		})
	}
}

func TestLoadingClusterConfigWithNonexistentClusterName(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminal()

	// when
	cfg, err := configuration.LoadClusterConfig(term, "dummy")

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the provided cluster-name 'dummy' is not present in your ksctl.yaml file. The available cluster names are")
	assert.Contains(t, err.Error(), "host")
	assert.Contains(t, err.Error(), "member-1")
	assert.Empty(t, cfg.OperatorNamespace)
}

func TestLoad(t *testing.T) {

	t.Run("with verbose messages", func(t *testing.T) {
		// given
		term := NewFakeTerminal()
		SetFileConfig(t, Host(), Member())
		configuration.Verbose = true

		// when
		ksctlConfig, err := configuration.Load(term)

		// then
		require.NoError(t, err)
		expectedConfig := NewKsctlConfig(Host(), Member())
		assert.Equal(t, expectedConfig, ksctlConfig)
		assert.Contains(t, term.Output(), "Using config file")
	})

	t.Run("without verbose messages", func(t *testing.T) {
		// given
		term := NewFakeTerminal()
		SetFileConfig(t, Host(), Member())
		configuration.Verbose = false

		// when
		ksctlConfig, err := configuration.Load(term)

		// then
		require.NoError(t, err)
		expectedConfig := NewKsctlConfig(Host(), Member())
		assert.Equal(t, expectedConfig, ksctlConfig)
		assert.NotContains(t, term.Output(), "Using config file")
	})

}

func TestLoadFails(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		// given
		term := NewFakeTerminal()
		configuration.ConfigFileFlag = "/tmp/should-not-exist.yaml"

		// when
		_, err := configuration.Load(term)

		// then
		require.Error(t, err)
	})

	t.Run("file is directory", func(t *testing.T) {
		// given
		term := NewFakeTerminal()
		configuration.ConfigFileFlag = os.TempDir()

		// when
		_, err := configuration.Load(term)

		// then
		require.Error(t, err)
	})
}

func TestAsSuffix(t *testing.T) {
	t.Run("host type", func(t *testing.T) {
		// given
		prefix := "prefix"

		// when
		result := configuration.Host.AsSuffix(prefix)

		// then
		assert.Equal(t, "prefix-host", result)
	})

	t.Run("member type", func(t *testing.T) {
		// given
		prefix := "prefix"

		// when
		result := configuration.Member.AsSuffix(prefix)

		// then
		assert.Equal(t, "prefix-member", result)
	})
}
