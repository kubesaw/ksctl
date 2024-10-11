package context_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/require"
)

func TestLoadClusterConfig(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("success", func(t *testing.T) {
		buffy := bytes.NewBuffer(nil)
		logger := log.New(buffy)

		// when
		_, err := configuration.LoadClusterConfig(logger, "host")

		// then
		require.NoError(t, err)
	})

	t.Run("fail", func(t *testing.T) {
		// given
		SetFileConfig(t, Host(NoToken()), Member(NoToken()))
		buffy := bytes.NewBuffer(nil)
		logger := log.New(buffy)

		// when
		_, err := configuration.LoadClusterConfig(logger, "host")

		// then
		require.Error(t, err)
	})
}
