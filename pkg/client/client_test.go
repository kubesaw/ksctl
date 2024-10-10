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
	routev1 "github.com/openshift/api/route/v1"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	_, err = cl.IsObjectNamespaced(testObj)
	require.Error(t, err)
	// actual error is "failed to get restmapping: failed to get server groups: Get \"https://fail-cluster.com/api?timeout=1m0s\": dial tcp: lookup fail-cluster.com: no such host"
	require.ErrorContains(t, err, "dial tcp: lookup fail-cluster.com: no such host")
}

func TestPatchUserSignup(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("update is successful", func(t *testing.T) {
		//given
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
		//given
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
		//given
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
		//given
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
		//given
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
		//given
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

func TestEnsure(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	subs := newSubscription("cool-op", "staging")

	t.Run("successful", func(t *testing.T) {

		t.Run("when creating", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t)
			term := NewFakeTerminalWithResponse("Y")
			actual := subs.DeepCopy()

			// when
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.NoError(t, err)
			assert.True(t, applied)
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionHasSpec(t, fakeClient, namespacedName, subs.Spec)
			output := term.Output()
			assert.NotContains(t, output, "!!!  DANGER ZONE  !!!")
			assert.Contains(t, output, "Are you sure that you want to create the Subscription resource with the name toolchain-host-operator/cool-subs ?")
			assert.Contains(t, output, "The 'toolchain-host-operator/cool-subs' Subscription has been created")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when updating", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, newSubscription("other-operator", "prod"))
			term := NewFakeTerminalWithResponse("Y")

			// when
			actual := subs.DeepCopy()
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.NoError(t, err)
			assert.True(t, applied)
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionHasSpec(t, fakeClient, namespacedName, subs.Spec)
			output := term.Output()
			assert.Contains(t, output, "!!!  DANGER ZONE  !!!")
			assert.Contains(t, output, "Are you sure that you want to update the Subscription with the hard-coded version?")
			assert.Contains(t, output, "The 'cool-subs' Subscription has been updated")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when N is answered", func(t *testing.T) {
			// given
			existing := newSubscription("other-operator", "prod")
			fakeClient := commontest.NewFakeClient(t, existing)
			term := NewFakeTerminalWithResponse("N")

			// when
			actual := subs.DeepCopy()
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.NoError(t, err)
			assert.False(t, applied)
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionHasSpec(t, fakeClient, namespacedName, existing.Spec)
			output := term.Output()
			assert.Contains(t, output, "!!!  DANGER ZONE  !!!")
			assert.Contains(t, output, "Are you sure that you want to update the Subscription with the hard-coded version?")
			assert.NotContains(t, output, "The 'cool-subs' Subscription has been updated")
			assert.NotContains(t, output, "cool-token")
		})
	})

	t.Run("failed", func(t *testing.T) {

		t.Run("when get fails", func(t *testing.T) {
			// given
			existing := newSubscription("other-operator", "prod")
			fakeClient := commontest.NewFakeClient(t, newSubscription("other-operator", "prod"))
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("some error")
			}
			term := NewFakeTerminalWithResponse("Y")

			// when
			actual := subs.DeepCopy()
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.Error(t, err)
			assert.False(t, applied)
			fakeClient.MockGet = nil
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionHasSpec(t, fakeClient, namespacedName, existing.Spec)
			output := term.Output()
			assert.NotContains(t, output, "!!!  DANGER ZONE  !!!")
			assert.NotContains(t, output, "Are you sure that you want to update the Subscription with the hard-coded version?")
			assert.NotContains(t, output, "The 'cool-subs' Subscription has been updated")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when create fails", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t)
			fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				return fmt.Errorf("some error")
			}
			term := NewFakeTerminalWithResponse("Y")

			// when
			actual := subs.DeepCopy()
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.Error(t, err)
			assert.False(t, applied)
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionDoesNotExist(t, fakeClient, namespacedName)
			output := term.Output()
			assert.NotContains(t, output, "!!!  DANGER ZONE  !!!")
			assert.Contains(t, output, "Are you sure that you want to create the Subscription resource with the name toolchain-host-operator/cool-subs ?")
			assert.NotContains(t, output, "The 'toolchain-host-operator/cool-subs' Subscription has been created")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when update fails", func(t *testing.T) {
			// given
			existing := newSubscription("other-operator", "prod")
			fakeClient := commontest.NewFakeClient(t, newSubscription("other-operator", "prod"))
			fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				return fmt.Errorf("some error")
			}
			term := NewFakeTerminalWithResponse("Y")

			// when
			actual := subs.DeepCopy()
			applied, err := client.Ensure(term, fakeClient, actual)

			// then
			require.Error(t, err)
			assert.False(t, applied)
			namespacedName := commontest.NamespacedName(subs.Namespace, subs.Name)
			AssertSubscriptionHasSpec(t, fakeClient, namespacedName, existing.Spec)
			output := term.Output()
			assert.Contains(t, output, "!!!  DANGER ZONE  !!!")
			assert.Contains(t, output, "Are you sure that you want to update the Subscription with the hard-coded version?")
			assert.NotContains(t, output, "The 'cool-subs' Subscription has been updated")
			assert.NotContains(t, output, "cool-token")
		})
	})
}

func TestCreate(t *testing.T) {

	t.Run("create", func(t *testing.T) {

		t.Run("if it does not exist yet", func(t *testing.T) {
			// given
			namespacedName := commontest.NamespacedName("openshift-customer-monitoring", "openshift-customer-monitoring")
			fakeClient := commontest.NewFakeClient(t)
			term := NewFakeTerminalWithResponse("Y")
			operatorGroup := newOperatorGroup(namespacedName, map[string]string{"provider": "ksctl"})

			// when
			err := client.Create(term, fakeClient, operatorGroup)

			// then
			require.NoError(t, err)
			AssertOperatorGroupHasLabels(t, fakeClient, namespacedName, map[string]string{"provider": "ksctl"})
			output := term.Output()
			assert.Contains(t, output, "The 'openshift-customer-monitoring/openshift-customer-monitoring' OperatorGroup has been created")
		})
	})

	t.Run("do not create", func(t *testing.T) {

		t.Run("if it already exists", func(t *testing.T) {
			// given
			namespacedName := commontest.NamespacedName("openshift-customer-monitoring", "openshift-customer-monitoring")
			fakeClient := commontest.NewFakeClient(t, newOperatorGroup(namespacedName, map[string]string{"provider": "osd"}))
			term := NewFakeTerminalWithResponse("Y")
			operatorGroup := newOperatorGroup(namespacedName, map[string]string{"provider": "ksctl"})

			// when
			err := client.Create(term, fakeClient, operatorGroup)

			// then
			require.NoError(t, err)
			AssertOperatorGroupHasLabels(t, fakeClient, namespacedName, map[string]string{"provider": "osd"})
			output := term.Output()
			assert.Contains(t, output, "The 'openshift-customer-monitoring/openshift-customer-monitoring' OperatorGroup already exists")
		})

		t.Run("when error occurs on client.Get", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, newSubscription("other-operator", "prod"))
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("get failed")
			}
			term := NewFakeTerminalWithResponse("Y")
			namespacedName := commontest.NamespacedName("openshift-customer-monitoring", "openshift-customer-monitoring")
			operatorGroup := newOperatorGroup(namespacedName, map[string]string{"provider": "ksctl"})

			// when
			err := client.Create(term, fakeClient, operatorGroup)

			// then
			require.Error(t, err)
			require.EqualError(t, err, "get failed")
		})

		t.Run("when error occurs on client.Create", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, newSubscription("other-operator", "prod"))
			fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				return fmt.Errorf("create failed")
			}
			term := NewFakeTerminalWithResponse("Y")
			namespacedName := commontest.NamespacedName("openshift-customer-monitoring", "openshift-customer-monitoring")
			operatorGroup := newOperatorGroup(namespacedName, map[string]string{"provider": "ksctl"})

			// when
			err := client.Create(term, fakeClient, operatorGroup)

			// then
			require.Error(t, err)
			require.EqualError(t, err, "create failed")
		})
	})
}

func TestGetRoute(t *testing.T) {

	// given
	require.NoError(t, client.AddToScheme())
	term := NewFakeTerminalWithResponse("Y")

	t.Run("success", func(t *testing.T) {

		t.Run("route with TLS enabled", func(t *testing.T) {
			// given
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-monitoring",
					Name:      "thanos-querier",
				},
				Spec: routev1.RouteSpec{
					Host: "prometheus-dev",
					Path: "graph",
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationReencrypt,
					},
				},
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Host: "prometheus-dev/graph",
						},
					},
				},
			}
			fakeClient := commontest.NewFakeClient(t, route)

			// when
			r, err := client.GetRouteURL(term, fakeClient, types.NamespacedName{
				Namespace: "openshift-monitoring",
				Name:      "thanos-querier",
			})
			// then
			require.NoError(t, err)
			assert.Equal(t, "https://prometheus-dev/graph", r)
		})

		t.Run("route with TLS not enabled", func(t *testing.T) {
			// given
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-monitoring",
					Name:      "thanos-querier",
				},
				Spec: routev1.RouteSpec{
					Host: "prometheus-dev",
					Path: "graph",
				},
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Host: "prometheus-dev/graph",
						},
					},
				},
			}
			fakeClient := commontest.NewFakeClient(t, route)

			// when
			r, err := client.GetRouteURL(term, fakeClient, types.NamespacedName{
				Namespace: "openshift-monitoring",
				Name:      "thanos-querier",
			})
			// then
			require.NoError(t, err)
			assert.Equal(t, "http://prometheus-dev/graph", r)
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("client error", func(t *testing.T) {
			// given
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-monitoring",
					Name:      "thanos-querier",
				},
				Spec: routev1.RouteSpec{},
				// no status will cause a timeout
			}
			fakeClient := commontest.NewFakeClient(t, route)
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("mock error")
			}
			// when
			_, err := client.GetRouteURL(term, fakeClient, types.NamespacedName{
				Namespace: "openshift-monitoring",
				Name:      "thanos-querier",
			})
			// then
			require.Error(t, err)
			require.EqualError(t, err, "unable to get route to openshift-monitoring/thanos-querier: mock error")

		})

		t.Run("timeout", func(t *testing.T) {
			// given
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-monitoring",
					Name:      "thanos-querier",
				},
				Spec: routev1.RouteSpec{},
				// no status will cause a timeout
			}
			fakeClient := commontest.NewFakeClient(t, route)

			// when
			_, err := client.GetRouteURL(term, fakeClient, types.NamespacedName{
				Namespace: "openshift-monitoring",
				Name:      "thanos-querier",
			})
			// then
			require.Error(t, err)
			require.EqualError(t, err, "unable to get route to openshift-monitoring/thanos-querier: timed out waiting for the condition")
		})
	})
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

func newSubscription(pkg, channel string) *olmv1alpha1.Subscription {
	return &olmv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "Subscription",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: commontest.HostOperatorNs,
			Name:      "cool-subs",
		},
		Spec: &olmv1alpha1.SubscriptionSpec{
			Channel:                channel,
			InstallPlanApproval:    olmv1alpha1.ApprovalAutomatic,
			Package:                pkg,
			CatalogSource:          "cool-subs",
			CatalogSourceNamespace: commontest.HostOperatorNs,
		},
	}
}

func newOperatorGroup(namespacedName types.NamespacedName, labels map[string]string) *olmv1.OperatorGroup {
	return &olmv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
			Labels:    labels,
		},
		Spec: olmv1.OperatorGroupSpec{
			TargetNamespaces: []string{
				namespacedName.Namespace,
			},
		},
	}
}
