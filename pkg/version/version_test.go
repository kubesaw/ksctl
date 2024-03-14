package version_test

import (
	"testing"

	"github.com/kubesaw/ksctl/pkg/version"
	"github.com/stretchr/testify/assert"
)

func TestVars(t *testing.T) {
	// simply verify that the vars exist.
	// They will be populated by the `-ldflags="-X ..."` at build time
	assert.Equal(t, "unknown", version.Commit)
	assert.Equal(t, "unknown", version.BuildTime)
}

func TestVersionMessage(t *testing.T) {
	assert.Equal(t, "commit: 'unknown', build time: 'unknown'", version.NewMessage())
}
