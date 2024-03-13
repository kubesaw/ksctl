package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	// given
	content := []string{"1", "2", "3"}

	t.Run("Contains", func(t *testing.T) {

		// when
		ok := Contains(content, "2")

		// then
		assert.True(t, ok)
	})

	t.Run("does not contain", func(t *testing.T) {

		// when
		ok := Contains(content, "4")

		// then
		assert.False(t, ok)
	})
}
