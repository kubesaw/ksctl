package utils

import (
	"fmt"
	"math"
	"net/url"
	"strings"
)

// Contains checks if the given slice of strings contains the given string
func Contains(slice []string, value string) bool {
	for _, role := range slice {
		if role == value {
			return true
		}
	}
	return false
}

// GetToolchainClusterName produces a name for ToolchainCluster object that is both deterministic and "reasonably unique".
// The `ordinal` must be greater than 0 if there are multiple ToolchainCluster objects pointing to the same cluster. This
// needs to be determined by the caller prior to calling this method.
func GetToolchainClusterName(clusterType, serverAPIEndpoint string, ordinal int) (string, error) {
	// NOTE: this function is ported from the original add-cluster.sh script to produce the same names during the transition
	// period to the new operator-based approach to the member registration.
	// Since add-cluster.sh was a bash script with a long history, the logic is a bit convoluted at places (especially in
	// handling the numerical suffix).

	// we need to make sure that:
	// 1) the name is at most 63 characters long
	// 2) the variable part is (a part of) the cluster hostname
	// 3) it ends with a digit (supplied by the ordinal param) if it was shortened

	var ordinalLength int
	if ordinal > 0 {
		ordinalLength = int(math.Log10(float64(ordinal))) + 1
	} else {
		ordinalLength = 0
	}

	// the name always contains the cluster type, a hypen between the cluster type and the numerical suffix (if needed)
	// Interestingly, this is computed BEFORE we determine if we need the numerical suffix at all, but that's the logic
	// in the original script.
	fixedLength := len(clusterType) + ordinalLength + 1

	maxAllowedClusterHostNameLen := 62 - fixedLength // I think 62 is here, because we might default the ordinal to 1 later on and we could theoretically

	clusterHostName, err := sanitizeEnpointForUsageAsName(serverAPIEndpoint)
	if err != nil {
		return "", fmt.Errorf("failed to sanitize the endpoint for naming purposes: %w", err)
	}
	if len(clusterHostName) >= maxAllowedClusterHostNameLen {
		clusterHostName = clusterHostName[0:maxAllowedClusterHostNameLen]
		if ordinal <= 0 {
			ordinal = 1
		}
	}

	if ordinal <= 0 {
		return fmt.Sprintf("%s-%s", clusterType, clusterHostName), nil
	}
	return fmt.Sprintf("%s-%s%d", clusterType, clusterHostName, ordinal), nil
}

func sanitizeEnpointForUsageAsName(apiEndpoint string) (string, error) {
	// This logic is again taken from add-cluster.sh
	url, err := url.Parse(apiEndpoint)
	if err != nil {
		return "", fmt.Errorf("could not parse the API endpoint '%s' as url: %w", apiEndpoint, err)
	}

	hostName := url.Hostname()
	if len(hostName) == 0 {
		hostName = url.Path
	}
	if strings.HasPrefix(hostName, "api.") && len(hostName) > 4 {
		hostName = hostName[4:]
	}

	return hostName, nil
}
