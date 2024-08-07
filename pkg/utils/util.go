package utils

import (
	"fmt"
	"net/url"
	"strings"
)

const K8sLabelWithoutSuffixMaxLength = 62

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
// The `suffix` must be non-empty if there are multiple ToolchainCluster objects pointing to the same cluster. This
// needs to be determined by the caller prior to calling this method.
func GetToolchainClusterName(clusterType, serverAPIEndpoint, suffix string) (string, error) {
	// NOTE: this function is ported from the original add-cluster.sh script to produce the same names during the transition
	// period to the new operator-based approach to the member registration.
	// Since add-cluster.sh was a bash script with a long history, the logic is a bit convoluted at places (especially in
	// handling the numerical suffix).

	// we need to make sure that:
	// 1) the name is at most 63 characters long
	// 2) the variable part is (a part of) the cluster hostname
	// 3) it ends with a digit (supplied by the suffix param) if it was shortened

	suffix = strings.TrimSpace(suffix)

	// the name always contains the cluster type, a hypen between the cluster type and the name and finally the suffix (if needed)
	// Interestingly, this is computed BEFORE we determine if we need the suffix at all, but that's the logic
	// in the original script.
	fixedLength := len(clusterType) + len(suffix) + 1

	maxAllowedClusterHostNameLen := K8sLabelWithoutSuffixMaxLength - fixedLength // I think 62 is here, because we might default the suffix to "1" later on

	clusterHostName, err := GetClusterHostName(serverAPIEndpoint, maxAllowedClusterHostNameLen, suffix)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s", clusterType, clusterHostName), nil
}

func GetClusterHostName(serverAPIEndpoint string, maxAllowedClusterHostNameLen int, suffix string) (string, error) {
	clusterHostName, err := sanitizeEndpointForUsageAsName(serverAPIEndpoint)
	if err != nil {
		return "", fmt.Errorf("failed to sanitize the endpoint for naming purposes: %w", err)
	}
	if len(clusterHostName) >= maxAllowedClusterHostNameLen {
		clusterHostName = clusterHostName[0:maxAllowedClusterHostNameLen]
		// the original script uses this approach to ensure that the name ends with an alphanumeric
		// character (i.e. that the name doesn't end with a '.' after shortening the hostname)
		if len(suffix) == 0 {
			suffix = "1"
		}
	}
	return fmt.Sprintf("%s%s", clusterHostName, suffix), nil
}

func sanitizeEndpointForUsageAsName(apiEndpoint string) (string, error) {
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
