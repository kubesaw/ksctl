package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamelCaseToKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"already-kebab", "already-kebab"},
		{"A", "a"},
		{"AA", "aa"},
		{"AaAa", "aa-aa"},
		{"someValue", "some-value"},
		{"someCoolValue", "some-cool-value"},
		{"member1", "member-1"},
	}
	for _, test := range tests {
		assert.Equal(t, test.expected, CamelCaseToKebabCase(test.input))
	}
}

func TestKebabCaseToCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"alreadyCamel", "alreadyCamel"},
		{"a", "a"},
		{"AA", "AA"},
		{"aa-aa", "aaAa"},
		{"some-value", "someValue"},
		{"some-cool-value", "someCoolValue"},
		{"member-1", "member1"},
	}
	for _, test := range tests {
		assert.Equal(t, test.expected, KebabToCamelCase(test.input))
	}
}
