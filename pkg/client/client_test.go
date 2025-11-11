package client_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewClientOK(t *testing.T) {
	// given
	t.Cleanup(gock.OffAll)
	gock.New("https://some-dummy-example.com").
		Get("api").
		Persist().
		Reply(200).
		BodyString("{}")

	// when
	cl, err := client.NewClientWithTransport("cool-token", "https://some-dummy-example.com", gock.DefaultTransport)

	// then
	require.NoError(t, err)
	assert.NotNil(t, cl)
}

func TestNewClientFromRestConfigError(t *testing.T) {
	cl, err := client.NewClientFromRestConfig(nil)
	require.EqualError(t, err, "cannot create client: must provide non-nil rest.Config to client.New")
	require.Nil(t, cl)
}

func TestNewClientFail(t *testing.T) {
	// when
	cl, err := client.NewClient("cool-token", "https://fail-cluster.com")
	require.NoError(t, err)
	assert.NotNil(t, cl)
	// then
	testObj := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "john-doe",
			Namespace: "default",
		},
	}
	// no specific reason to check if object is namespaced, ANY request to the actual api would trigger error indicating incorrect configuration of client
	_, err = cl.IsObjectNamespaced(testObj)
	require.Error(t, err)
	// actual error is "failed to get restmapping: failed to get server groups: Get \"https://fail-cluster.com/api?timeout=1m0s\": dial tcp: lookup fail-cluster.com: no such host"
	require.ErrorContains(t, err, "dial tcp: lookup fail-cluster.com")
	// for ci the error message is dial tcp: lookup fail-cluster.com on 127.0.0.53:53: no such host, could go the regex way or string submatching with wildcards
	// having two separate checks for different substrings is an easy fix
	require.ErrorContains(t, err, "no such host")
}

func TestPatchUserSignup(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("update is successful", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return true, nil
		}, "updated")

		// then
		require.NoError(t, err)
		states.SetApprovedManually(userSignup, true)
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("UserSignup should not be updated", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return false, nil
		}, "updated")

		// then
		require.NoError(t, err)
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("change UserSignup func returns error", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return false, fmt.Errorf("some error")
		}, "updated")

		// then
		require.EqualError(t, err, "some error")
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("get of UserSignup fails", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return true, nil
		}, "updated")

		// then
		require.EqualError(t, err, "some error")
	})

	t.Run("update of UserSignup fails", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockPatch = func(ctx context.Context, obj runtimeclient.Object, patch runtimeclient.Patch, opts ...runtimeclient.PatchOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return true, nil
		}, "updated")

		// then
		require.EqualError(t, err, "some error")
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("client creation fails", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		fakeClient := commontest.NewFakeClient(t, userSignup)
		term := NewFakeTerminal()
		newClient := func(_, _ string) (runtimeclient.Client, error) {
			return nil, fmt.Errorf("some error")
		}
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
			states.SetApprovedManually(signup, true)
			return true, nil
		}, "updated")

		// then
		require.EqualError(t, err, "some error")
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})
}

func TestUpdateUserSignupLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()))

	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := client.PatchUserSignup(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup) (bool, error) {
		states.SetApprovedManually(signup, true)
		return true, nil
	}, "updated")

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertUserSignupSpec(t, fakeClient, userSignup)
}

func TestNewKubeClientFromKubeConfig(t *testing.T) {
	// given
	t.Cleanup(gock.OffAll)
	gock.New("https://cool-server.com").
		Get("api").
		Persist().
		Reply(200).
		BodyString("{}")

	t.Run("success", func(j *testing.T) {
		// when
		cl, err := client.NewKubeClientFromKubeConfig(PersistKubeConfigFile(t, HostKubeConfig()))

		// then
		require.NoError(t, err)
		assert.NotNil(t, cl)
	})

	t.Run("error", func(j *testing.T) {
		// when
		cl, err := client.NewKubeClientFromKubeConfig("/invalid/kube/config")

		// then
		require.Error(t, err)
		assert.Nil(t, cl)
	})
}
