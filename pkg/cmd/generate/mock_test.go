package generate

import (
	"fmt"
	"os"
	"testing"

	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	HostServerAPI    = "https://api.sandbox.host.openshiftapps.com:6443"
	Member1ServerAPI = "https://api.sandbox.member1.openshiftapps.com:6443"
	Member2ServerAPI = "https://api.sandbox.member2.openshiftapps.com:6443"
)

// files part

func newDefaultFiles(t *testing.T, fakeFiles ...test.FakeFileCreator) assets.FS {
	roles := []runtime.Object{installOperatorRole, restartDeploymentRole, editDeploymentRole, registerClusterRole}

	files := test.NewFakeFiles(t,
		append(fakeFiles,
			test.FakeTemplate("setup/roles/host.yaml", roles...),
			test.FakeTemplate("setup/roles/member.yaml", roles...))...,
	)
	return files
}

func createKubeSawAdminsFile(t *testing.T, dirPrefix string, content []byte) string { //nolint:unparam
	configTempDir, err := os.MkdirTemp("", dirPrefix+"-")
	require.NoError(t, err)
	configFile := fmt.Sprintf("%s/kubesaw-admins.yaml", configTempDir)
	err = os.WriteFile(configFile, content, 0600)
	require.NoError(t, err)
	return configFile
}

// setupContext part

func newSetupContextWithDefaultFiles(t *testing.T, config *assets.KubeSawAdmins) *setupContext { //nolint:unparam
	return newSetupContext(t, config, newDefaultFiles(t))
}

func newSetupContext(t *testing.T, config *assets.KubeSawAdmins, files assets.FS) *setupContext {
	fakeTerminal := test.NewFakeTerminal()
	fakeTerminal.Tee(os.Stdout)
	require.NoError(t, client.AddToScheme())
	temp, err := os.MkdirTemp("", "cli-tests-")
	require.NoError(t, err)
	return &setupContext{
		Terminal:      fakeTerminal,
		kubeSawAdmins: config,
		files:         files,
		setupFlags: setupFlags{
			outDir:        temp,
			memberRootDir: "member",
			hostRootDir:   "host",
		},
	}
}

// ClusterContext part

func newFakeClusterContext(setupContext *setupContext, clusterType configuration.ClusterType) *clusterContext {
	return &clusterContext{
		setupContext: setupContext,
		clusterType:  clusterType,
	}
}
