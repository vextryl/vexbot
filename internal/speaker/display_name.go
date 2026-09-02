package speaker

import (
	"strings"
	"unicode"
)

func Normalize(displayName string) string {
	return strings.Join(strings.FieldsFunc(displayName, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}), " ")
}

func Resolve(userID string, displayNames map[string]string) string {
	if displayName := Normalize(displayNames[userID]); displayName != "" {
		return displayName
	}
	return userID
}
