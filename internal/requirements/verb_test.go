package requirements

import (
	"testing"
)

func TestNormalizeVerb(t *testing.T) {
	cases := []struct{ in, want string }{
		{"shall", "MUST"},
		{"Shall", "MUST"},
		{"SHALL", "MUST"},
		{"must", "MUST"},
		{"required", "MUST"},
		{"should", "SHOULD"},
		{"recommended", "SHOULD"},
		{"may", "MAY"},
		{"optional", "MAY"},
		{"must not", "MUST NOT"},
		{"shall not", "MUST NOT"},
		{"should not", "SHOULD NOT"},
		{"unknown", "UNKNOWN"},
	}
	for _, c := range cases {
		if got := NormalizeVerb(c.in); got != c.want {
			t.Errorf("NormalizeVerb(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDetectVerb(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"The PDU shall close the contactors.", "MUST"},
		{"The system must not exceed 5V.", "MUST NOT"},
		{"The contractor should reveal alternatives.", "SHOULD"},
		{"The PDU shall not close when fault is active.", "MUST NOT"},
		{"Pure description with no imperative.", ""},
		{"This may be implemented optionally.", "MAY"},
		// "maybe" must not match "may"
		{"It is maybe useful.", ""},
	}
	for _, c := range cases {
		got := DetectVerb(c.text)
		if got != c.want {
			t.Errorf("DetectVerb(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
