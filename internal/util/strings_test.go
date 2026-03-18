package util

import (
	"testing"
)

// T7: ShortID
func TestShortID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "standard ULID", input: "01ARYZ6S41YY1R5GJ1V96XG000", want: "96xg000"},
		{name: "7 char input", input: "ABCDEFG", want: "abcdefg"},
		{name: "less than 7", input: "ABC", want: "abc"},
		{name: "empty string", input: "", want: ""},
		{name: "exactly 8 chars", input: "12345678", want: "2345678"},
		{name: "already lowercase", input: "01aryz6s41yy1r5gj1v96xg000", want: "96xg000"},
		{name: "single char", input: "X", want: "x"},
		{name: "sha256 hex", input: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", want: "2efcde9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortID(tt.input)
			if got != tt.want {
				t.Errorf("ShortID(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify always lowercase
			for _, c := range got {
				if c >= 'A' && c <= 'Z' {
					t.Errorf("ShortID(%q) contains uppercase: %q", tt.input, got)
					break
				}
			}
		})
	}
}
