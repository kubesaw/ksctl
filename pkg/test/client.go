package test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFakeClients(t *testing.T, initObjs ...runtime.Object) (clicontext.NewClientFunc, clicontext.NewRESTClientFunc, *test.FakeClient) {
	fakeClient := test.NewFakeClient(t, initObjs...)
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		stringDataToData(obj)
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
	fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		stringDataToData(obj)
		return fakeClient.Client.Update(ctx, obj, opts...)
	}
	return func(token, apiEndpoint string) (runtimeclient.Client, error) {
			assert.Equal(t, "cool-token", token)
			assert.Contains(t, apiEndpoint, "http")
			assert.Contains(t, apiEndpoint, "://")
			assert.Contains(t, apiEndpoint, ".com")
			return fakeClient, nil
		},
		func(token string, apiEndpoint string) (*rest.RESTClient, error) {
			return NewFakeExternalClient(t, token, apiEndpoint), nil
		},
		fakeClient
}

func NewFakeExternalClient(t *testing.T, token string, apiEndpoint string) *rest.RESTClient {
	cl, err := client.NewRESTClient(token, apiEndpoint)
	require.NoError(t, err)
	// override the underlying client's transport with Gock to intercep requests
	cl.Client.Transport = gock.DefaultTransport
	return cl
}

func stringDataToData(obj runtimeclient.Object) {
	if obj.GetObjectKind().GroupVersionKind().Kind == "Secret" {
		secret := obj.(*corev1.Secret)
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		for key, value := range secret.StringData {
			secret.Data[key] = []byte(value)
		}
	}
}

func AssertArgsEqual(expArgs ...string) ArgsAssertion {
	return func(t *testing.T, actualArgs ...string) {
		assert.Equal(t, expArgs, actualArgs)
	}
}

func AssertFirstArgPrefixRestEqual(firstArgPrefix string, toEqual ...string) ArgsAssertion {
	return func(t *testing.T, actualArgs ...string) {
		assert.Regexp(t, firstArgPrefix, actualArgs[0])
		assert.Equal(t, toEqual, actualArgs[1:])
	}
}

type ArgsAssertion func(*testing.T, ...string)

func NewCommandCreator(t *testing.T, cmd string, expCmd string, assertArgs ArgsAssertion) client.CommandCreator {
	return func(name string, actualArgs ...string) *exec.Cmd {
		assert.Equal(t, expCmd, name)
		assertArgs(t, actualArgs...)
		return exec.Command(cmd, name)
	}
}
