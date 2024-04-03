package generate

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
	kubeSawAdminsFile, outDir, hostRootDir, memberRootDir string
	singleCluster                                         bool
}

func NewAdminManifestsCmd() *cobra.Command {
	f := setupFlags{}
	command := &cobra.Command{
		Use: "admin-manifests --kubesaw-admins=<path-to-kubesaw-admins-file> --out-dir <path-to-out-dir>",
		Example: `ksctl generate admin-manifests ./path/to/kubesaw.openshiftapps.com/kubesaw-admins.yaml --out-dir ./components/auth/kubesaw-production
ksctl generate admin-manifests ./path/to/kubesaw-stage.openshiftapps.com/kubesaw-admins.yaml --out-dir ./components/auth/kubesaw-staging -s`,
		Short: "Generates user-management manifests",
		Long:  `Reads the kubesaw-admins.yaml file and based on the content it generates user-management RBAC and manifests.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			return Setup(term, resources.Resources, f)
		},
	}
	command.Flags().StringVarP(&f.kubeSawAdminsFile, "kubesaw-admins", "c", "", "Use the given kubesaw-admin file")
	command.Flags().StringVarP(&f.outDir, "out-dir", "o", "", "Directory where generated manifests should be stored")
	command.Flags().BoolVarP(&f.singleCluster, "single-cluster", "s", false, "If host and member are deployed to the same cluster")
	command.Flags().StringVar(&f.hostRootDir, "host-root-dir", "host", "The root directory name for host manifests")
	command.Flags().StringVar(&f.memberRootDir, "member-root-dir", "member", "The root directory name for member manifests")

	flags.MustMarkRequired(command, "kubesaw-admins")
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

	// Get the unmarshalled version of kubesaw-admins.yaml
	kubeSawAdmins, err := assets.GetKubeSawAdminsConfig(flags.kubeSawAdminsFile)
	if err != nil {
		return errs.Wrapf(err, "unable get kubesaw-admins.yaml file from %s", flags.kubeSawAdminsFile)
	}
	err = os.RemoveAll(flags.outDir)
	if err != nil {
		return err
	}
	ctx := &setupContext{
		Terminal:      term,
		kubeSawAdmins: kubeSawAdmins,
		setupFlags:    flags,
		files:         files,
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
	kubeSawAdmins *assets.KubeSawAdmins
	files         assets.FS
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
