package tables

import "testing"

func TestParseQuantityStr_unitFromHint(t *testing.T) {
	cases := []struct {
		val      string
		hint     string
		wantUnit string
		wantRaw  string
	}{
		{"0", "V DC", "V", "0"},
		{"1300", "V DC", "V", "1300"},
		{"85", "°C", "°C", "85"},
		{"10", "kHz", "kHz", "10"},
		{"500", "ms", "ms", "500"},
		{"1300 V", "", "V", "1300 V"}, // unit embedded in value
	}
	for _, c := range cases {
		v := c.val
		got := parseQuantityStr(&v, c.hint)
		if got == nil {
			t.Errorf("parseQuantityStr(%q, %q) = nil", c.val, c.hint)
			continue
		}
		if got.Unit != c.wantUnit {
			t.Errorf("parseQuantityStr(%q, %q).Unit = %q, want %q", c.val, c.hint, got.Unit, c.wantUnit)
		}
		if got.Raw != c.wantRaw {
			t.Errorf("parseQuantityStr(%q, %q).Raw = %q, want %q", c.val, c.hint, got.Raw, c.wantRaw)
		}
	}
}

func TestCanonicalUnit(t *testing.T) {
	cases := []struct{ hint, want string }{
		{"V DC", "V"},
		{"V", "V"},
		{"°C", "°C"},
		{"kHz rms", "kHz"},
		{"ms", "ms"},
		{"unknownUnit", "unknownUnit"},
	}
	for _, c := range cases {
		got := canonicalUnit(c.hint)
		if got != c.want {
			t.Errorf("canonicalUnit(%q) = %q, want %q", c.hint, got, c.want)
		}
	}
}
