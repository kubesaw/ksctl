package test

import (
	"os/exec"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFakeClients(t *testing.T, initObjs ...runtime.Object) (clicontext.NewClientFunc, *test.FakeClient) {
	fakeClient := test.NewFakeClient(t, initObjs...)
	return func(token, apiEndpoint string) (runtimeclient.Client, error) {
			t.Helper()
			assert.Equal(t, "cool-token", token)
			assert.Contains(t, apiEndpoint, "http")
			assert.Contains(t, apiEndpoint, "://")
			assert.Contains(t, apiEndpoint, ".com")
			return fakeClient, nil
		},
		fakeClient
}

func AssertFirstArgPrefixRestEqual(firstArgPrefix string, toEqual ...string) ArgsAssertion {
	return func(t *testing.T, actualArgs ...string) {
		t.Helper()
		assert.Regexp(t, firstArgPrefix, actualArgs[0])
		assert.Equal(t, toEqual, actualArgs[1:])
	}
}

type ArgsAssertion func(*testing.T, ...string)

func NewCommandCreator(t *testing.T, cmd string, expCmd string, assertArgs ArgsAssertion) client.CommandCreator {
	return func(name string, actualArgs ...string) *exec.Cmd {
		t.Helper()
		assert.Equal(t, expCmd, name)
		assertArgs(t, actualArgs...)
		return exec.Command(cmd, name)
	}
}
