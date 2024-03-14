package utils

import (
	"regexp"
	"strings"
)

// KebabToCamelCase converts kebab case to camel case
func KebabToCamelCase(kebab string) (camelCase string) {
	isToUpper := false
	for _, runeValue := range kebab {
		if isToUpper {
			camelCase += strings.ToUpper(string(runeValue))
			isToUpper = false
		} else {
			if runeValue == '-' {
				isToUpper = true
			} else {
				camelCase += string(runeValue)
			}
		}
	}
	return
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCapAndNumbers = regexp.MustCompile("([a-z])([A-Z0-9])")

func CamelCaseToKebabCase(str string) string {
	kebab := matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	kebab = matchAllCapAndNumbers.ReplaceAllString(kebab, "${1}-${2}")
	return strings.ToLower(kebab)
}
