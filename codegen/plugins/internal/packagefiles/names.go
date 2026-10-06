package packagefiles

import (
	"strings"
	"unicode"
)

func Stem(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Applet"
	}

	var b strings.Builder
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Applet"
	}
	return b.String()
}
