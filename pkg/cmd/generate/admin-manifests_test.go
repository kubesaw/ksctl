package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	uuid "github.com/google/uuid"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetup(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	kubeSawAdmins := NewKubeSawAdmins(
		Clusters(HostServerAPI).
			AddMember("member1", Member1ServerAPI).
			AddMember("member2", Member2ServerAPI),
		ServiceAccounts(
			Sa("john", "",
				HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("admin")),
				MemberRoleBindings("toolchain-member-operator", Role("install-operator"), ClusterRole("admin"))),
			Sa("bob", "",
				HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("edit")),
				MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("edit")))),
		Users(
			User("john-user", []string{"12345"}, "crtadmins-view",
				HostRoleBindings("toolchain-host-operator", Role("register-cluster"), ClusterRole("edit")),
				MemberRoleBindings("toolchain-member-operator", Role("register-cluster"), ClusterRole("edit"))),
			User("bob-crtadmin", []string{"67890"}, "crtadmins-exec",
				HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("admin")),
				MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("admin")))))

	kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
	require.NoError(t, err)

	configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
	files := newDefaultFiles(t)

	t.Run("all created", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("in single-cluster mode", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), singleCluster())

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("in custom host root directory", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), hostRootDir("host-cluster"))

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("in custom member root directory", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile), memberRootDir("member-clusters"))

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("previous deleted", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		storeDummySA(t, outTempDir)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("if out dir doesn't exist then it creates", func(t *testing.T) {
		// given
		outTempDir := filepath.Join(os.TempDir(), fmt.Sprintf("setup-cli-test-%s", uuid.NewString()))
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile(configFile))

		// when
		err = Setup(term, files, flags)

		// then
		require.NoError(t, err)
		verifyFiles(t, flags)
	})

	t.Run("fails for non-existing kubesaw-admins.yaml file", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "setup-cli-test-")
		require.NoError(t, err)
		term := NewFakeTerminalWithResponse("Y")
		term.Tee(os.Stdout)
		flags := newSetupFlags(outDir(outTempDir), kubeSawAdminsFile("does/not/exist"))

		// when
		err = Setup(term, files, flags)

		// then
		require.Error(t, err)
	})
}

func storeDummySA(t *testing.T, outDir string) {
	ctx := newSetupContextWithDefaultFiles(t, nil)
	ctx.outDir = outDir
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "User",
			APIVersion: userv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "dummy-namespace",
			Name:      "dummy-name",
		},
	}
	err := writeManifest(ctx, filePath(filepath.Join(outDir, "dummy"), sa, "ServiceAccount"), sa)
	require.NoError(t, err)
}

func verifyFiles(t *testing.T, flags setupFlags) {
	dirEntries, err := os.ReadDir(flags.outDir)
	require.NoError(t, err)
	var dirNames []string
	if !flags.singleCluster {
		assert.Len(t, dirEntries, 2)
		dirNames = []string{dirEntries[0].Name(), dirEntries[1].Name()}
	} else {
		assert.Len(t, dirEntries, 3)
		dirNames = []string{dirEntries[0].Name(), dirEntries[1].Name(), dirEntries[2].Name()}
	}

	for _, clusterType := range configuration.ClusterTypes {
		ns := commontest.HostOperatorNs
		expectedRootDir := flags.hostRootDir
		if clusterType == configuration.Member {
			ns = commontest.MemberOperatorNs
			expectedRootDir = flags.memberRootDir
		}
		assert.Contains(t, dirNames, expectedRootDir)
		verifyServiceAccounts(t, flags.outDir, expectedRootDir, clusterType, ns)
		verifyUsers(t, flags.outDir, expectedRootDir, clusterType, ns, flags.singleCluster)
	}
}

func verifyServiceAccounts(t *testing.T, outDir, expectedRootDir string, clusterType configuration.ClusterType, roleNs string) {
	saNs := fmt.Sprintf("sandbox-sre-%s", clusterType)

	inKStructure(t, outDir, expectedRootDir).
		assertSa(saNs, "john").
		hasRole(roleNs, clusterType.AsSuffix("install-operator"), clusterType.AsSuffix("install-operator-john")).
		hasNsClusterRole(roleNs, "admin", clusterType.AsSuffix("clusterrole-admin-john"))

	inKStructure(t, outDir, expectedRootDir).
		assertSa(saNs, "bob").
		hasRole(roleNs, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob")).
		hasNsClusterRole(roleNs, "edit", clusterType.AsSuffix("clusterrole-edit-bob"))
}

func verifyUsers(t *testing.T, outDir, expectedRootDir string, clusterType configuration.ClusterType, ns string, singleCluster bool) {
	rootDir := expectedRootDir
	if singleCluster {
		rootDir = "base"
	}

	inKStructure(t, outDir, rootDir).
		assertUser("john-user").
		hasIdentity("12345").
		belongsToGroups(groups("crtadmins-view"), extraGroupsUserIsNotPartOf("crtadmins-exec"))

	storageAssertion := inKStructure(t, outDir, expectedRootDir).storageAssertionImpl
	newPermissionAssertion(storageAssertion, "", "john-user", "User").
		hasRole(ns, clusterType.AsSuffix("register-cluster"), clusterType.AsSuffix("register-cluster-john-user")).
		hasNsClusterRole(ns, "edit", clusterType.AsSuffix("clusterrole-edit-john-user"))

	inKStructure(t, outDir, rootDir).
		assertUser("bob-crtadmin").
		hasIdentity("67890").
		belongsToGroups(groups("crtadmins-exec"), extraGroupsUserIsNotPartOf("crtadmins-view"))

	newPermissionAssertion(storageAssertion, "", "bob-crtadmin", "User").
		hasRole(ns, clusterType.AsSuffix("restart-deployment"), clusterType.AsSuffix("restart-deployment-bob-crtadmin")).
		hasNsClusterRole(ns, "admin", clusterType.AsSuffix("clusterrole-admin-bob-crtadmin"))
}

func createKubeconfigFiles(t *testing.T, contents ...string) []string {
	var fileNames []string
	for _, content := range contents {
		tempFile, err := os.CreateTemp("", "sandbox-sre-kubeconfig-")
		require.NoError(t, err)

		err = os.WriteFile(tempFile.Name(), []byte(content), os.FileMode(0755))
		require.NoError(t, err)

		require.NoError(t, tempFile.Close())
		fileNames = append(fileNames, tempFile.Name())
	}
	return fileNames
}

const ksctlKubeconfigContent = `
apiVersion: v1
clusters:
- cluster:
    server: https://api.sandbox.host.openshiftapps.com:6443
  name: api-sandbox-host-openshiftapps-com:6443
- cluster:
    server: https://api.sandbox.member1.openshiftapps.com:6443
  name: api-sandbox-member1-openshiftapps-com:6443
contexts:
- context:
    cluster: api-sandbox-host-openshiftapps-com:6443
    namespace: toolchain-host-operator
    user: dedicatedadmin
  name: host
- context:
    cluster: api-sandbox-member1-openshiftapps-com:6443
    namespace: toolchain-member-operator
    user: dedicatedadmin
  name: member1
current-context: host
kind: Config
preferences: {}
users:
- name: dedicatedadmin
  user:
    token: my-cool-token
`

const ksctlKubeconfigContentMember2 = `
apiVersion: v1
clusters:
- cluster:
    server: https://api.sandbox.member2.openshiftapps.com:6443
  name: api-sandbox-member2-openshiftapps-com:6443
contexts:
- context:
    cluster: api-sandbox-member2-openshiftapps-com:6443
    namespace: toolchain-member-operator
    user: dedicatedadmin
  name: member2
current-context: member2
kind: Config
preferences: {}
users:
- name: dedicatedadmin
  user:
    token: my-cool-token
`

type setupFlagsOption func(*setupFlags)

func newSetupFlags(setupFlagsOptions ...setupFlagsOption) setupFlags {
	flags := setupFlags{
		hostRootDir:   "host",
		memberRootDir: "member",
	}
	for _, applyOption := range setupFlagsOptions {
		applyOption(&flags)
	}
	return flags
}

func kubeSawAdminsFile(configName string) setupFlagsOption {
	return func(flags *setupFlags) {
		flags.kubeSawAdminsFile = configName
	}
}

func outDir(outDir string) setupFlagsOption {
	return func(flags *setupFlags) {
		flags.outDir = outDir
	}
}

func hostRootDir(hostRootDir string) setupFlagsOption {
	return func(flags *setupFlags) {
		flags.hostRootDir = hostRootDir
	}
}

func memberRootDir(memberRootDir string) setupFlagsOption {
	return func(flags *setupFlags) {
		flags.memberRootDir = memberRootDir
	}
}

func singleCluster() setupFlagsOption {
	return func(flags *setupFlags) {
		flags.singleCluster = true
	}
}
