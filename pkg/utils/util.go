package utils

// Contains checks if the given slice of strings contains the given string
func Contains(slice []string, value string) bool {
	for _, role := range slice {
		if role == value {
			return true
		}
	}
	return false
}
