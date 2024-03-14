package adm

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes/scheme"
	ktypes "sigs.k8s.io/kustomize/api/types"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type objectsCache map[string]runtimeclient.Object

func (c objectsCache) storeObject(ctx *clusterContext, obj runtimeclient.Object) error {
	return c.ensureObject(ctx, obj, nil)
}

func (c objectsCache) ensureObject(ctx *clusterContext, toEnsure runtimeclient.Object, updateExisting func(runtimeclient.Object) (bool, error)) error {
	obj := toEnsure.DeepCopyObject().(runtimeclient.Object)
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}
	if len(gvks) > 1 {
		return fmt.Errorf("multiple versions of a single GK not supported but found multiple for object %v", obj)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	path, theOtherTypePath, basePath, err := filePaths(ctx, obj)
	if err != nil {
		return err
	}
	if ctx.singleCluster {
		if _, exists := c[basePath]; exists {
			path = basePath
		} else if existing, exists := c[theOtherTypePath]; exists {
			c[basePath] = existing
			delete(c, theOtherTypePath)
			path = basePath
		}
	}
	if existing, exists := c[path]; exists {
		if updateExisting == nil {
			// "the file already exists and is not supposed to be updated
			return nil
		}
		toUpdate := existing.DeepCopyObject().(runtimeclient.Object)
		if modified, err := updateExisting(toUpdate); !modified || err != nil {
			return err
		}
		c[path] = toUpdate
		return nil
	}
	c[path] = obj
	return nil
}

func (c objectsCache) writeManifests(ctx *setupContext) error {
	for path, object := range c {
		if err := writeManifest(ctx, path, object); err != nil {
			return err
		}
	}
	return nil
}

func writeManifest(ctx *setupContext, filePath string, obj runtimeclient.Object) error {
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0744); err != nil {
		return err
	}

	content, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	if err := writeFile(filePath, content); err != nil {
		return err
	}
	if err := ensureKustomization(ctx, dirPath, filepath.Base(filePath)); err != nil {
		return err
	}
	return nil
}

func filePaths(ctx *clusterContext, obj runtimeclient.Object) (string, string, string, error) {
	kind := obj.GetObjectKind().GroupVersionKind()
	plural, _ := meta.UnsafeGuessKindToResource(kind)
	// if kind is not defined in the given object, then fail
	if plural.Resource == "" {
		return "", "", "", fmt.Errorf("missing kind in the manifest %s of the type %s", obj.GetName(), reflect.TypeOf(obj).Elem().Name())
	}
	if obj.GetName() == "" {
		return "", "", "", fmt.Errorf("missing name in the manifest of the type %s", reflect.TypeOf(obj).Elem().Name())
	}

	defaultPath := filePath(rootDir(ctx.setupContext, ctx.clusterType), obj, plural.Resource)
	theOtherTypePath := filePath(rootDir(ctx.setupContext, ctx.clusterType.TheOtherType()), obj, plural.Resource)
	basePath := filePath(baseDirectory(ctx.outDir), obj, plural.Resource)

	return defaultPath, theOtherTypePath, basePath, nil
}

func rootDir(ctx *setupContext, clusterType configuration.ClusterType) string {
	if clusterType == configuration.Host {
		return filepath.Join(ctx.outDir, ctx.hostRootDir)
	}
	return filepath.Join(ctx.outDir, ctx.memberRootDir)
}

func filePath(rootDir string, obj runtimeclient.Object, resource string) string {
	dirPath := filepath.Join(rootDir, "namespace-scoped", obj.GetNamespace(), strings.ToLower(resource))
	if obj.GetNamespace() == "" {
		dirPath = filepath.Join(rootDir, "cluster-scoped", strings.ToLower(resource))
	}
	path := filepath.Join(dirPath, fmt.Sprintf("%s.yaml", obj.GetName()))
	// make sure that `path` does not contain `:` characters
	path = strings.ReplaceAll(path, ":", "-")
	return path
}

const header = `# ----------------------------------------------------------------
# Generated by cli - DO NOT EDIT
# ----------------------------------------------------------------

`

func writeFile(filePath string, content []byte) error {
	// https://github.com/kubernetes/kubernetes/issues/67610
	contentString := strings.ReplaceAll(string(content), "\n  creationTimestamp: null", "")
	contentString = strings.ReplaceAll(contentString, "\nuser: {}", "")
	contentString = fmt.Sprintf("%s%s", header, contentString)
	return os.WriteFile(filePath, []byte(contentString), 0600)
}

func baseDirectory(outDir string) string {
	return filepath.Join(outDir, "base")
}

func ensureKustomization(ctx *setupContext, dirPath, item string) error {
	kustomization := &ktypes.Kustomization{
		TypeMeta: ktypes.TypeMeta{
			APIVersion: ktypes.KustomizationVersion,
			Kind:       ktypes.KustomizationKind,
		},
	}
	kFilePath := filepath.Join(dirPath, "kustomization.yaml")
	if _, err := os.Stat(kFilePath); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		file, err := os.ReadFile(kFilePath)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(file, kustomization); err != nil {
			return err
		}
	}
	if utils.Contains(kustomization.Resources, item) {
		return nil
	}
	kustomization.Resources = append(kustomization.Resources, item)
	sort.Strings(kustomization.Resources)
	content, err := yaml.Marshal(kustomization)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirPath, 0744); err != nil {
		return err
	}
	if err := writeFile(kFilePath, content); err != nil {
		return err
	}
	parentDir := filepath.Dir(dirPath)
	if ctx.outDir == parentDir {
		if dirPath == baseDirectory(ctx.outDir) {
			return ensureKustomization(ctx, rootDir(ctx, configuration.Member), "../base")
		}
		return nil
	}
	return ensureKustomization(ctx, parentDir, filepath.Base(dirPath))
}

func sreLabelsWithUsername(username string) map[string]string {
	labels := sreLabels()
	labels["username"] = username
	return labels
}

func sreLabels() map[string]string {
	return map[string]string{
		"provider": "sandbox-sre",
	}
}

func sandboxSRENamespace(clusterType configuration.ClusterType) string {
	sandboxSRENamespace := "sandbox-sre-host"
	if clusterType == configuration.Member {
		sandboxSRENamespace = "sandbox-sre-member"
	}
	return sandboxSRENamespace
}
