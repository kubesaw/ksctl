package generate

import (
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeready-toolchain/toolchain-common/pkg/template/nstemplatetiers"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewNSTemplateTiersCmd() *cobra.Command {
	var source, outDir, hostNs string
	command := &cobra.Command{
		Use:   "nstemplatetiers --source=<path-to-nstemplatetiers> --out-dir=<folder-to-store-generated-manifests>",
		Short: "Generates NSTemplateTiers and TierTemplates",
		Long:  `Reads files from the provided directory, and generates the NSTemplateTier & TierTemplates to the out-dir`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			return NSTemplateTiers(term, source, outDir, hostNs)
		},
	}
	command.Flags().StringVarP(&source, "source", "s", "", "The source of NSTemplateTier templates")
	command.Flags().StringVarP(&outDir, "out-dir", "o", "", "Directory where generated manifests should be stored")
	command.Flags().StringVar(&hostNs, "host-namespace", "toolchain-host-operator", "Host operator namespace to be set in the generated manifests")

	flags.MustMarkRequired(command, "source")
	flags.MustMarkRequired(command, "out-dir")

	return command
}

func NSTemplateTiers(term ioutils.Terminal, source, outDir, hostNs string) error {
	if err := client.AddToScheme(); err != nil {
		return err
	}

	metadata := map[string]string{}
	templates := map[string][]byte{}
	err := filepath.Walk(source, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tmplPath := filepath.Join(filepath.Base(filepath.Dir(path)), info.Name())
		templates[tmplPath] = file
		checksum := crc32.Checksum(file, crc32.IEEETable)
		metadata[strings.TrimSuffix(tmplPath, ".yaml")] = fmt.Sprint(checksum)
		return nil
	})
	if err != nil {
		return err
	}

	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	ctx := manifestStoreContext{
		outDir:                absOutDir,
		manageOutDirKustomize: true,
	}
	err = nstemplatetiers.GenerateTiers(scheme.Scheme, func(toEnsure runtimeclient.Object, canUpdate bool, tierName string) (bool, error) {
		if err := setGVK(toEnsure); err != nil {
			return false, err
		}
		kind := toEnsure.GetObjectKind().GroupVersionKind().Kind
		path := filepath.Join(absOutDir, tierName, fmt.Sprintf("%s-%s.yaml", strings.ToLower(kind), toEnsure.GetName()))
		term.Printlnf("Storing %s with name %s in %s", kind, toEnsure.GetName(), path)
		return true, writeManifest(ctx, path, toEnsure)
	}, hostNs, metadata, templates)
	if err != nil {
		return err
	}
	term.Println("")
	term.Println(`Generation of the NSTemplateTiers has finished. 
Make sure that the old TierTemplates are still present in the folders (you didn't delete them before/after running the command). They are necessary for running proper updates of the existing Spaces (Namespaces).`)
	return nil
}
