package assets

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	"github.com/kubesaw/ksctl/pkg/configuration"

	templatev1 "github.com/openshift/api/template/v1"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// interface that matches all the methods provided by embed.FS
type FS interface {
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
}

var decoder runtime.Decoder

func init() {
	decoder = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
}

type FilenameMatcher func(string) bool

func GetSandboxEnvironmentConfig(sandboxConfigFile string) (*SandboxEnvironmentConfig, error) {
	content, err := os.ReadFile(sandboxConfigFile)
	if err != nil {
		return nil, err
	}
	config := &SandboxEnvironmentConfig{}
	if err := yaml.Unmarshal(content, config); err != nil {
		return nil, err
	}
	return config, nil
}

func GetRoles(f fs.ReadFileFS, clusterType configuration.ClusterType) ([]runtimeclient.Object, error) {
	return GetSetupTemplateObjects(f, fmt.Sprintf("roles/%s.yaml", clusterType))
}

func GetSetupTemplateObjects(f fs.ReadFileFS, filePath string) ([]runtimeclient.Object, error) {
	return ParseTemplate(f, fmt.Sprintf("setup/%s", filePath))
}

func ParseTemplate(f fs.ReadFileFS, fileName string) ([]runtimeclient.Object, error) {
	content, err := f.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	configTemplate := &templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, configTemplate)
	if err != nil {
		return nil, err
	}
	parameters := map[string]string{}

	return template.NewProcessor(scheme.Scheme).Process(configTemplate, parameters)
}
