package context_test

import (
	"testing"

	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPath(t *testing.T) {

	// given
	clusterConfigParams := []ClusterDefinitionWithName{
		Host(ServerName("host-cluster")),
		Member(ServerName("member-cluster"))}
	SetFileConfig(t, clusterConfigParams...)

	for _, clusterConfigParam := range clusterConfigParams {

		term := NewFakeTerminal()
		clusterName := clusterConfigParam.ClusterName
		cfg, err := configuration.LoadClusterConfig(term, clusterName)
		require.NoError(t, err)

		t.Run(string(cfg.ClusterType), func(t *testing.T) {

			t.Run("with explicit clusterName", func(t *testing.T) {
				// given
				ctx := clicontext.NewClusterConfigCommandContext(term, cfg, nil, resources.Resources, "custom_path")

				// when
				path, err := cfg.ConfigurePath(ctx, ctx.ClusterConfigName, "component")

				// then
				require.NoError(t, err)
				assert.Equal(t, "custom_path/configure/"+cfg.ClusterType.String()+"/component", path)
			})

			t.Run("without explicit clusterName", func(t *testing.T) {
				// given
				ctx := clicontext.NewClusterConfigCommandContext(term, cfg, nil, resources.Resources, "") // default path

				// when
				path, err := cfg.ConfigurePath(ctx, ctx.ClusterConfigName, "component")

				// then
				require.NoError(t, err)
				assert.Equal(t, "host-cluster/configure/"+cfg.ClusterType.String()+"/component", path) // no matter if the cluster is the Host or a Member
			})
		})
	}
}
