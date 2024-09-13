package generate

import (
	"fmt"
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

type adminManifestsFlags struct {
	kubeSawAdminsFile, outDir, hostRootDir, memberRootDir, idpName string
	singleCluster                                                  bool
}

func NewAdminManifestsCmd() *cobra.Command {
	f := adminManifestsFlags{}
	command := &cobra.Command{
		Use: "admin-manifests --kubesaw-admins=<path-to-kubesaw-admins-file> --out-dir <path-to-out-dir>",
		Example: `ksctl generate admin-manifests ./path/to/kubesaw.openshiftapps.com/kubesaw-admins.yaml --out-dir ./components/auth/kubesaw-production
ksctl generate admin-manifests ./path/to/kubesaw-stage.openshiftapps.com/kubesaw-admins.yaml --out-dir ./components/auth/kubesaw-staging -s`,
		Short: "Generates user-management manifests",
		Long:  `Reads the kubesaw-admins.yaml file and based on the content it generates user-management RBAC and manifests.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			return adminManifests(term, resources.Resources, f)
		},
	}
	command.Flags().StringVarP(&f.kubeSawAdminsFile, "kubesaw-admins", "c", "", "Use the given kubesaw-admin file")
	command.Flags().StringVarP(&f.outDir, "out-dir", "o", "", "Directory where generated manifests should be stored")
	command.Flags().BoolVarP(&f.singleCluster, "single-cluster", "s", false, "If host and member are deployed to the same cluster. Cannot be used with separateKustomizeComponent set in one of the members.")
	command.Flags().StringVar(&f.hostRootDir, "host-root-dir", "host", "The root directory name for host manifests")
	command.Flags().StringVar(&f.memberRootDir, "member-root-dir", "member", "The root directory name for member manifests")
	command.Flags().StringVar(&f.idpName, "idp-name", "KubeSaw", "Identity provider name to be used in Identity CRs")

	flags.MustMarkRequired(command, "kubesaw-admins")
	flags.MustMarkRequired(command, "out-dir")

	return command
}

func adminManifests(term ioutils.Terminal, files assets.FS, flags adminManifestsFlags) error {
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
	if flags.singleCluster {
		for _, memberCluster := range kubeSawAdmins.Clusters.Members {
			if memberCluster.SeparateKustomizeComponent {
				return fmt.Errorf("--single-cluster flag cannot be used with separateKustomizeComponent set in one of the members (%s)", memberCluster.Name)
			}
		}
	}

	if defaultSAsNamespace(kubeSawAdmins, configuration.Host) == defaultSAsNamespace(kubeSawAdmins, configuration.Member) {
		return fmt.Errorf("the default ServiceAccounts namespace has the same name for host cluster as for the member clusters (%s), they have to be different", defaultSAsNamespace(kubeSawAdmins, configuration.Host))
	}
	err = os.RemoveAll(flags.outDir)
	if err != nil {
		return err
	}
	ctx := &adminManifestsContext{
		Terminal:            term,
		kubeSawAdmins:       kubeSawAdmins,
		adminManifestsFlags: flags,
		files:               files,
	}
	objsCache := objectsCache{}
	if err := ensureCluster(ctx, configuration.Host, objsCache, ""); err != nil {
		return err
	}
	if err := ensureCluster(ctx, configuration.Member, objsCache, ""); err != nil {
		return err
	}

	for _, memberCluster := range kubeSawAdmins.Clusters.Members {
		if memberCluster.SeparateKustomizeComponent {
			if err := ensureCluster(ctx, configuration.Member, objsCache, memberCluster.Name); err != nil {
				return err
			}
		}
	}
	return objsCache.writeManifests(ctx)
}

type adminManifestsContext struct {
	ioutils.Terminal
	adminManifestsFlags
	kubeSawAdmins *assets.KubeSawAdmins
	files         assets.FS
}

func ensureCluster(ctx *adminManifestsContext, clusterType configuration.ClusterType, cache objectsCache, specificKMemberName string) error {
	if specificKMemberName == "" {
		ctx.PrintContextSeparatorf("Generating manifests for %s cluster type", clusterType)
	} else {
		ctx.PrintContextSeparatorf("Generating manifests for %s cluster type in the separate Kustomize component: %s", clusterType, specificKMemberName)
	}

	clusterCtx := &clusterContext{
		adminManifestsContext: ctx,
		clusterType:           clusterType,
		specificKMemberName:   specificKMemberName,
	}

	if err := ensureServiceAccounts(clusterCtx, cache); err != nil {
		return err
	}
	return ensureUsers(clusterCtx, cache)

}
