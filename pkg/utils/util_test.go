package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetToolchainClusterName(t *testing.T) {
	type Params struct {
		ServerAPI   string
		ClusterType string
		Ordinal     int
	}
	for expectedClusterName, params := range map[string]Params{
		"member-api-prefix-dropped": {
			ClusterType: "member",
			ServerAPI:   "https://api.api-prefix-dropped",
			Ordinal:     0,
		},
		"member-path-dropped": {
			ClusterType: "member",
			ServerAPI:   "https://path-dropped:6443/some-path",
			Ordinal:     0,
		},
		"member-port-dropped": {
			ClusterType: "member",
			ServerAPI:   "https://port-dropped:6443",
			Ordinal:     0,
		},
		"member-trailing-slash-dropped": {
			ClusterType: "member",
			ServerAPI:   "https://trailing-slash-dropped/",
			Ordinal:     0,
		},
		"member-cluster-name-and-ordinal.elb.us-east-1.amazonaws.com1": {
			ClusterType: "member",
			ServerAPI:   "socks5://cluster-name-and-ordinal.elb.us-east-1.amazonaws.com:123",
			Ordinal:     1,
		},
		"member-like-reeeeeeaaaaaalllllly-llllloooooooonnnnnnnnggggggg1": {
			ClusterType: "member",
			ServerAPI:   "http://like-reeeeeeaaaaaalllllly-llllloooooooonnnnnnnnggggggg-cluster-name.elb.us-east-1.amazonaws.com",
			Ordinal:     1,
		},
		"member-cluster-name-no-ordinal.elb.us-east-1.amazonaws.com": {
			ClusterType: "member",
			ServerAPI:   "https://cluster-name-no-ordinal.elb.us-east-1.amazonaws.com",
			Ordinal:     0,
		},
		"member-61-characters-long--012345678901234567890123456789012341": {
			ClusterType: "member",
			ServerAPI:   "https://61-characters-long--01234567890123456789012345678901234567890",
			Ordinal:     0,
		},
		"member-62-characters-long--012345678901234567890123456789012341": {
			ClusterType: "member",
			ServerAPI:   "https://62-characters-long--012345678901234567890123456789012345678901",
			Ordinal:     0,
		},
		"member-63-characters-long--012345678901234567890123456789012341": {
			ClusterType: "member",
			ServerAPI:   "https://63-characters-long--0123456789012345678901234567890123456789012",
			Ordinal:     0,
		},
		"member-54-characters-long--0123456789012345678901234567890123": {
			ClusterType: "member",
			ServerAPI:   "https://54-characters-long--0123456789012345678901234567890123",
			Ordinal:     0,
		},
		"member-55-characters-long--012345678901234567890123456789012341": {
			ClusterType: "member",
			ServerAPI:   "https://55-characters-long--01234567890123456789012345678901234",
			Ordinal:     0,
		},
		"member-56-characters-long--012345678901234567890123456789012341": {
			ClusterType: "member",
			ServerAPI:   "https://56-characters-long--012345678901234567890123456789012345",
			Ordinal:     0,
		},
		"member-54-characters-long--01234567890123456789012345678901239": {
			ClusterType: "member",
			ServerAPI:   "https://54-characters-long--0123456789012345678901234567890123",
			Ordinal:     9,
		},
		"member-54-characters-long--01234567890123456789012345678901420": {
			ClusterType: "member",
			ServerAPI:   "https://54-characters-long--0123456789012345678901234567890123",
			Ordinal:     420,
		},
	} {
		t.Run(expectedClusterName, func(t *testing.T) {
			actualClusterName, err := GetToolchainClusterName(params.ClusterType, params.ServerAPI, params.Ordinal)
			require.NoError(t, err)
			assert.Equal(t, expectedClusterName, actualClusterName)
		})
	}
}
