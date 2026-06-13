// Package rounding rounds durations to billing increments. It is pure.
package rounding

import (
	"strings"
	"time"
)

// Mode controls how durations are rounded before push.
type Mode int

const (
	ModeOff Mode = iota
	Mode5m
	Mode6m
	Mode15m
)

// DefaultMode is the rounding applied when none is configured: nearest 15m.
const DefaultMode = Mode15m

// Parse maps a config value ("off", "5m", "6m", "15m") to a Mode. An empty or
// unrecognized value falls back to DefaultMode so a missing or typo'd setting
// still rounds sensibly rather than silently disabling rounding.
func Parse(s string) Mode {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "off":
		return ModeOff
	case "5m":
		return Mode5m
	case "6m":
		return Mode6m
	case "15m":
		return Mode15m
	default:
		return DefaultMode
	}
}

// Round rounds d to the nearest increment specified by m.
func Round(d time.Duration, m Mode) time.Duration {
	var increment time.Duration
	switch m {
	case Mode5m:
		increment = 5 * time.Minute
	case Mode6m:
		increment = 6 * time.Minute
	case Mode15m:
		increment = 15 * time.Minute
	default:
		return d
	}
	return ((d + increment/2) / increment) * increment
}
