// Package rounding rounds durations to billing increments. It is pure.
package rounding

import "time"

// Mode controls how durations are rounded before push.
type Mode int

const (
	ModeOff Mode = iota
	Mode5m
	Mode6m
	Mode15m
)

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
