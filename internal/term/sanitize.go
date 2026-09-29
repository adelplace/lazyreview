// Package term holds helpers to safely display untrusted text in a terminal.
package term

import "strings"

// Sanitize replaces control characters with visible symbols so that text
// coming from GitHub (file contents, titles, comments) cannot emit terminal
// escape sequences. Newlines and tabs are kept.
func Sanitize(s string) string {
	if !hasControl(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20:
			b.WriteRune(0x2400 + r) // ␀ … ␟, e.g. ␛ for ESC
		case r == 0x7f:
			b.WriteRune('␡')
		case r >= 0x80 && r < 0xa0:
			b.WriteRune('�')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Line sanitizes s and folds it to a single line.
func Line(s string) string {
	return strings.ReplaceAll(Sanitize(s), "\n", " ")
}

func hasControl(s string) bool {
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r < 0xa0) {
			return true
		}
	}
	return false
}
