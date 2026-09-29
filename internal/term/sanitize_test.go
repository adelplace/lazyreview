package term

import "testing"

func TestSanitize(t *testing.T) {
	tests := map[string]string{
		"plain":           "plain",
		"a\x1b[31mred":    "a␛[31mred",
		"keep\ttab\nnl":   "keep\ttab\nnl",
		"bell\x07del\x7f": "bell␇del␡",
		"c1\u009b31m":     "c1�31m",
		"crlf\r":          "crlf␍",
		"unicode é 日本 ✓":  "unicode é 日本 ✓",
	}
	for in, want := range tests {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Line("a\nb\x1b"); got != "a b␛" {
		t.Errorf("Line: got %q", got)
	}
}
