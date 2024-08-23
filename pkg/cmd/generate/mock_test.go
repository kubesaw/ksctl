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
			test.FakeTemplate("roles/host.yaml", roles...),
			test.FakeTemplate("roles/member.yaml", roles...))...,
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

// adminManifestsContext part

func newAdminManifestsContextWithDefaultFiles(t *testing.T, config *assets.KubeSawAdmins) *adminManifestsContext { //nolint:unparam
	return newAdminManifestsContext(t, config, newDefaultFiles(t))
}

func newAdminManifestsContext(t *testing.T, config *assets.KubeSawAdmins, files assets.FS) *adminManifestsContext {
	fakeTerminal := test.NewFakeTerminal()
	fakeTerminal.Tee(os.Stdout)
	require.NoError(t, client.AddToScheme())
	temp, err := os.MkdirTemp("", "cli-tests-")
	require.NoError(t, err)
	return &adminManifestsContext{
		Terminal:      fakeTerminal,
		kubeSawAdmins: config,
		files:         files,
		adminManifestsFlags: adminManifestsFlags{
			outDir:        temp,
			memberRootDir: "member",
			hostRootDir:   "host",
		},
	}
}

// ClusterContext part

func newFakeClusterContext(adminManifestsContext *adminManifestsContext, clusterType configuration.ClusterType, options ...fakeClusterContextOption) *clusterContext {
	ctx := &clusterContext{
		adminManifestsContext: adminManifestsContext,
		clusterType:           clusterType,
	}
	for _, modify := range options {
		modify(ctx)
	}
	return ctx
}

type fakeClusterContextOption func(ctx *clusterContext)

func withSpecialKMemberName(specialKMemberName string) fakeClusterContextOption {
	return func(ctx *clusterContext) {
		ctx.specificKMemberName = specialKMemberName
	}
}
