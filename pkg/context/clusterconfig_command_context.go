package context

import (
	"path/filepath"

	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"
)

// ClusterConfigCommandContext the context of the admin command to run
type ClusterConfigCommandContext struct {
	CommandContext
	Files             assets.FS
	ClusterConfig     configuration.ClusterConfig
	ClusterConfigName string
}

// NewClusterConfigCommandContext returns the context of the admin command to run
func NewClusterConfigCommandContext(term ioutils.Terminal, cfg configuration.ClusterConfig, newClient NewClientFunc, files assets.FS, clusterConfigName string) *ClusterConfigCommandContext {
	return &ClusterConfigCommandContext{
		CommandContext: CommandContext{
			Terminal:  term,
			NewClient: newClient,
		},
		Files:             files,
		ClusterConfig:     cfg,
		ClusterConfigName: clusterConfigName,
	}
}

func (ctx *ClusterConfigCommandContext) GetFileContent(path ...string) ([]byte, error) {
	p := filepath.Join(path...)
	return ctx.Files.ReadFile(p)

}
