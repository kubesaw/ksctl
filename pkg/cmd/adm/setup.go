package adm

import (
	"os"
	"path/filepath"

	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/kubesaw/ksctl/resources"
	errs "github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type setupFlags struct {
	sandboxConfigFile, outDir, hostRootDir, memberRootDir string
	singleCluster                                         bool
}

func NewSetupCmd() *cobra.Command {
	f := setupFlags{}
	command := &cobra.Command{
		Use: "setup --sandbox-config=<path-to-sandbox-config-file> --out-dir <path-to-out-dir>",
		Example: `sandbox-cli adm setup ./path/to/sandbox.openshiftapps.com/sandbox-config.yaml --out-dir ./components/auth/devsandbox-production
sandbox-cli adm setup ./path/to/sandbox-stage.openshiftapps.com/sandbox-config.yaml --out-dir ./components/auth/devsandbox-staging -s`,
		Short: "Generates user-management manifests",
		Long:  `Reads the sandbox-config.yaml file and based on the content it generates user-management RBAC and manifests.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			return Setup(term, resources.Resources, f)
		},
	}
	command.Flags().StringVarP(&f.sandboxConfigFile, "sandbox-config", "c", "", "Use the given sandbox config file")
	command.Flags().StringVarP(&f.outDir, "out-dir", "o", "", "Directory where generated manifests should be stored")
	command.Flags().BoolVarP(&f.singleCluster, "single-cluster", "s", false, "If host and member are deployed to the same cluster")
	command.Flags().StringVar(&f.hostRootDir, "host-root-dir", "host", "The root directory name for host manifests")
	command.Flags().StringVar(&f.memberRootDir, "member-root-dir", "member", "The root directory name for member manifests")

	flags.MustMarkRequired(command, "sandbox-config")
	flags.MustMarkRequired(command, "out-dir")

	return command
}

func Setup(term ioutils.Terminal, files assets.FS, flags setupFlags) error {
	if err := client.AddToScheme(); err != nil {
		return err
	}
	abs, err := filepath.Abs(flags.outDir)
	if err != nil {
		return err
	}
	flags.outDir = abs

	// Get the unmarshalled version of sandbox-config.yaml
	sandboxEnvConfig, err := assets.GetSandboxEnvironmentConfig(flags.sandboxConfigFile)
	if err != nil {
		return errs.Wrapf(err, "unable get sandbox-config.yaml file from %s", flags.sandboxConfigFile)
	}
	err = os.RemoveAll(flags.outDir)
	if err != nil {
		return err
	}
	ctx := &setupContext{
		Terminal:         term,
		sandboxEnvConfig: sandboxEnvConfig,
		setupFlags:       flags,
		files:            files,
	}
	objsCache := objectsCache{}
	if err := ensureCluster(ctx, configuration.Host, objsCache); err != nil {
		return err
	}
	if err := ensureCluster(ctx, configuration.Member, objsCache); err != nil {
		return err
	}
	return objsCache.writeManifests(ctx)
}

type setupContext struct {
	ioutils.Terminal
	setupFlags
	sandboxEnvConfig *assets.SandboxEnvironmentConfig
	files            assets.FS
}

func ensureCluster(ctx *setupContext, clusterType configuration.ClusterType, cache objectsCache) error {
	ctx.PrintContextSeparatorf("Generating manifests for %s cluster type", clusterType)

	clusterCtx := &clusterContext{
		setupContext: ctx,
		clusterType:  clusterType,
	}

	if err := ensureServiceAccounts(clusterCtx, cache); err != nil {
		return err
	}
	return ensureUsers(clusterCtx, cache)

}
