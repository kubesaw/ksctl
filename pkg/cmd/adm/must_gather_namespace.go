package adm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewMustGatherNamespaceCmd() *cobra.Command {
	var destDir string
	var kubeconfig string
	cmd := &cobra.Command{
		Use:          "must-gather-namespace <namespace-name> --kubeconfig <path/to/kubeconfig> --dest-dir <output-directory>",
		Short:        "Dump all resources from a namespace",
		Long:         "Dump all resources from a namespace into the destination directory, one resource per file",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := log.New(cmd.OutOrStdout())
			kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			kubeconfig.Timeout = 60 * time.Second
			// These fields need to be set when using the REST client ¯\_(ツ)_/¯
			kubeconfig.ContentConfig = restclient.ContentConfig{
				GroupVersion:         &authv1.SchemeGroupVersion,
				NegotiatedSerializer: scheme.Codecs,
			}
			if err != nil {
				return err
			}
			return MustGatherNamespace(logger, kubeconfig, args[0], destDir)
		},
	}
	cmd.Flags().StringVar(&destDir, "dest-dir", "", "Gather information with a specific local folder to copy to")
	flags.MustMarkRequired(cmd, "dest-dir")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file")
	flags.MustMarkRequired(cmd, "kubeconfig")
	return cmd
}

func MustGatherNamespace(logger *log.Logger, kubeconfig *restclient.Config, namespace, destDir string) error {
	// verify that the destDir exists, otherwise, create it

	//  If path is already a directory, MkdirAll does nothing and returns nil.
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		logger.Errorf("The '%s' dest-dir is not empty. Aborting.", destDir)
		return nil
	}

	// find the API for the given resource type
	rcl, err := restclient.RESTClientFor(kubeconfig)
	if err != nil {
		return err
	}
	dcl := discovery.NewDiscoveryClient(rcl)

	logger.Info("fetching the list of API resources on the cluster...")
	apiResourceLists, err := dcl.ServerPreferredNamespacedResources()
	if err != nil {
		return err
	}

	logger.Infof("gathering all resources from the '%s' namespace...", namespace)
	cl, err := runtimeclient.New(kubeconfig, runtimeclient.Options{})
	if err != nil {
		return err
	}

	decoder := serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()

	for _, apiResourceList := range apiResourceLists {
		gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			return err
		}
		for _, r := range apiResourceList.APIResources {
			if !r.Namespaced {
				continue
			}
			if !can(r.Verbs, "list") {
				continue
			}
			// we don't need to collect these `PackageManifest` resources, most of them are
			// cluster-wide manifests.
			// See https://olm.operatorframework.io/docs/tasks/list-operators-available-to-install/#using-the-packagemanifest-api
			if gv.Group == "packages.operators.coreos.com" && r.Kind == "PackageManifest" {
				// let's skip this kind of resources...
				continue
			}
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   gv.Group,
				Version: gv.Version,
				Kind:    r.Kind,
			})
			if err := cl.List(context.Background(), list, runtimeclient.InNamespace(namespace)); err != nil {
				// log the error but continue so we can collect the remaining resources
				logger.Errorf("failed to list %s/%s: %v", strings.ToLower(gv.String()), strings.ToLower(r.Kind), err)
				continue
			}
			for _, item := range list.Items {
				logger.Infof("found %s/%s", item.GetKind(), item.GetName())
				filename := filepath.Join(destDir, fmt.Sprintf("%s-%s.yaml", strings.ToLower(item.GetKind()), item.GetName()))
				data, err := yaml.Marshal(item.Object)
				if err != nil {
					logger.Errorf("failed to marshal %s/%s %s: %v", strings.ToLower(gv.String()), strings.ToLower(r.Kind), item.GetName(), err)
					continue
				}
				if err := writeToFile(filename, data); err != nil {
					logger.Errorf("failed to save contents of %s/%s %s: %v", strings.ToLower(gv.String()), strings.ToLower(r.Kind), item.GetName(), err)
					return err
				}
				// also, for pods, gather process names and logs from all containers
				if item.GetAPIVersion() == "v1" && item.GetKind() == "Pod" {
					pod := &corev1.Pod{}
					if _, _, err := decoder.Decode(data, nil, pod); err != nil {
						logger.Errorf("failed to decode %s/%s '%s': %v", strings.ToLower(gv.String()), strings.ToLower(r.Kind), item.GetName(), err)
						continue
					}
					for _, cs := range pod.Status.ContainerStatuses {
						if cs.Started != nil && *cs.Started {
							if err := gatherContainerLogs(logger, rcl, destDir, namespace, pod.Name, cs.Name); err != nil {
								logger.Errorf("failed to collect logs from container '%s' in pod '%s': %v", cs.Name, pod.Name, err)
								// ignore error, continue to next container
								continue
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func gatherContainerLogs(logger *log.Logger, rcl *restclient.RESTClient, destDir, namespace, podName, containerName string) error {
	logger.Infof("collecting logs from %s/%s", podName, containerName)
	p := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", namespace, podName)
	result := rcl.Get().AbsPath(p).Param("container", containerName).Do(context.Background())
	if err := result.Error(); err != nil {
		return err
	}
	filename := filepath.Join(destDir, fmt.Sprintf("pod-%s-%s.logs", podName, containerName))
	data, err := result.Raw()
	if err != nil {
		return err
	}
	if err := writeToFile(filename, data); err != nil {
		return err
	}
	return nil
}

func writeToFile(filename string, data []byte) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprint(f, string(data))
	return err
}

func can(verbs metav1.Verbs, verb string) bool {
	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}
