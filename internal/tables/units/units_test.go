package units

import (
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		raw     string
		wantVal float64
		wantUnit string
		wantNil  bool
	}{
		{"50 dB", 50, "dB", false},
		{"5V", 5, "V", false},
		{"1300V", 1300, "V", false},
		{"0.1A", 0.1, "A", false},
		{"5,5 MHz", 5.5, "MHz", false},
		{"-40 °C", -40, "°C", false},
		{"TBD", 0, "", true},
		{"", 0, "", true},
		{"-", 0, "", true},
	}
	for _, c := range cases {
		got := Parse(c.raw)
		if c.wantNil {
			if got != nil {
				t.Errorf("Parse(%q) = %v, want nil", c.raw, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("Parse(%q) = nil, want value", c.raw)
			continue
		}
		if got.Value != c.wantVal {
			t.Errorf("Parse(%q).Value = %v, want %v", c.raw, got.Value, c.wantVal)
		}
		if got.Unit != c.wantUnit {
			t.Errorf("Parse(%q).Unit = %q, want %q", c.raw, got.Unit, c.wantUnit)
		}
	}
}
