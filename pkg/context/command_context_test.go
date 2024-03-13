package context_test

import (
	"testing"

	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/require"
)

func TestLoadClusterConfig(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("success", func(t *testing.T) {
		newClient, newRESTClient, _ := NewFakeClients(t)
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		_, err := configuration.LoadClusterConfig(ctx, "host")

		// then
		require.NoError(t, err)
	})

	t.Run("fail", func(t *testing.T) {
		// given
		SetFileConfig(t, Host(NoToken()), Member(NoToken()))
		newClient, newRESTClient, _ := NewFakeClients(t)
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		_, err := configuration.LoadClusterConfig(ctx, "host")

		// then
		require.Error(t, err)
	})
}
