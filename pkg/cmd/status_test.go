package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
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
	newClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	assert.Contains(t, buffy.String(), "Current ToolchainStatus - Condition: Ready, Status: True, Reason: AllComponentsReady")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestStatusCmdWhenIsNotReady(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(ToBeNotReady())
	newClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	assert.Contains(t, buffy.String(), "Current ToolchainStatus - Condition: Ready, Status: False, Reason: ComponentsNotReady, Message: components not ready: [members]")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestStatusCmdWhenConditionNotFound(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.NoError(t, err)
	assert.Contains(t, buffy.String(), "Current ToolchainStatus - Condition Ready not found")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestStatusCmdWithInsufficientPermissions(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, _ := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host(NoToken()))
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.Error(t, err)
	// output := term.Output()
	assert.NotContains(t, buffy.String(), "Current ToolchainStatus CR")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestStatusCmdWhenGetFailed(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(toolchainv1alpha1.Condition{})
	newClient, fakeClient := NewFakeClients(t, toolchainStatus)
	SetFileConfig(t, Host())
	fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		return fmt.Errorf("some error")
	}
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Status(ctx)

	// then
	require.Error(t, err)
	// output := term.Output()
	assert.NotContains(t, buffy.String(), "Current ToolchainStatus CR")
	assert.NotContains(t, buffy.String(), "cool-token")
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
