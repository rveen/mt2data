// Package units parses and normalises physical unit strings.
package units

import (
	"regexp"
	"strconv"
	"strings"
)

// Quantity holds a parsed numeric value and its canonical unit.
type Quantity struct {
	Value float64
	Unit  string
	Raw   string
}

// canonical unit aliases.
var unitAliases = map[string]string{
	"volts": "V", "volt": "V", "mv": "mV", "kv": "kV",
	"amps": "A", "amperes": "A", "ampere": "A", "ma": "mA",
	"ohms": "Ohm", "ohm": "Ohm", "ω": "Ohm", "kohm": "kOhm", "mohm": "MOhm",
	"hz": "Hz", "khz": "kHz", "mhz": "MHz", "ghz": "GHz",
	"db": "dB", "dbc": "dBc",
	"°c": "°C", "degc": "°C", "degrees c": "°C",
	"°f": "°F",
	"ms": "ms", "us": "µs", "µs": "µs", "ns": "ns",
	"w": "W", "kw": "kW", "mw": "mW",
	"pf": "pF", "nf": "nF", "uf": "µF", "µf": "µF",
	"nh": "nH", "uh": "µH", "µh": "µH", "mh": "mH",
	"km": "km", "m": "m", "mm": "mm",
	"%": "%",
}

var reQuantity = regexp.MustCompile(
	`^([+-]?\d+(?:[.,]\d+)?(?:[eE][+-]?\d+)?)\s*([a-zA-Z°µΩ%][a-zA-Z°µΩ/²³\-²³]*)?$`)

// Parse attempts to parse a raw string into a Quantity.
// Returns nil if the string cannot be parsed as a number+unit.
func Parse(raw string) *Quantity {
	s := strings.TrimSpace(raw)
	if s == "" || s == "-" || strings.EqualFold(s, "tbd") || strings.EqualFold(s, "n/a") {
		return nil
	}

	m := reQuantity.FindStringSubmatch(s)
	if m == nil {
		return nil
	}

	numStr := strings.ReplaceAll(m[1], ",", ".") // handle European decimal comma
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil
	}

	unit := normalizeUnit(m[2])
	return &Quantity{Value: val, Unit: unit, Raw: raw}
}

func normalizeUnit(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if canonical, ok := unitAliases[lower]; ok {
		return canonical
	}
	return raw
}
