package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStatusCmdWhenIsReady(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(ToBeReady())
	newClient, newRESTClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	output := term.Output()
	assert.Contains(t, output, "Current ToolchainStatus CR - Condition: Ready, Status: True, Reason: AllComponentsReady")
	assert.NotContains(t, output, "cool-token")
}

func TestStatusCmdWhenIsNotReady(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(ToBeNotReady())
	newClient, newRESTClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	output := term.Output()
	assert.Contains(t, output, "Current ToolchainStatus CR - Condition: Ready, Status: False, Reason: ComponentsNotReady, Message: components not ready: [members]")
	assert.NotContains(t, output, "cool-token")
}

func TestStatusCmdWhenConditionNotFound(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, newRESTClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	output := term.Output()
	assert.Contains(t, output, "Current ToolchainStatus CR - Condition Ready wasn't found!")
	assert.NotContains(t, output, "cool-token")
}

func TestStatusCmdWithInsufficientPermissions(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, newRESTClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host(NoToken()))
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.Error(t, err)
	output := term.Output()
	assert.NotContains(t, output, "Current ToolchainStatus CR")
	assert.NotContains(t, output, "cool-token")
}

func TestStatusCmdWhenGetFailed(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, newRESTClient, fakeClient := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		return fmt.Errorf("some error")
	}
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.Error(t, err)
	output := term.Output()
	assert.NotContains(t, output, "Current ToolchainStatus CR")
	assert.NotContains(t, output, "cool-token")
}

func NewToolchainStatus(cond toolchainv1alpha1.Condition) *toolchainv1alpha1.ToolchainStatus {
	return &toolchainv1alpha1.ToolchainStatus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "toolchain-status",
			Namespace: test.HostOperatorNs,
		},
		Status: toolchainv1alpha1.ToolchainStatusStatus{
			Conditions: []toolchainv1alpha1.Condition{
				cond,
			},
		},
	}
}

func ToBeReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
	}
}

func ToBeNotReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
		Message: "components not ready: [members]",
	}
}
