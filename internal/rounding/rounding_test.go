package rounding

import (
	"testing"
	"time"
)

func TestRound(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		mode Mode
		want time.Duration
	}{
		// Off: never changes the duration.
		{"off leaves duration untouched", 7 * time.Minute, ModeOff, 7 * time.Minute},
		{"off keeps odd seconds", 7*time.Minute + 30*time.Second, ModeOff, 7*time.Minute + 30*time.Second},

		// 5m increments.
		{"5m rounds down below half", 12 * time.Minute, Mode5m, 10 * time.Minute},
		{"5m rounds up at half", 12*time.Minute + 30*time.Second, Mode5m, 15 * time.Minute},
		{"5m exact boundary stays", 15 * time.Minute, Mode5m, 15 * time.Minute},

		// 6m increments.
		{"6m rounds down", 8 * time.Minute, Mode6m, 6 * time.Minute},
		{"6m rounds up at half", 9 * time.Minute, Mode6m, 12 * time.Minute},
		{"6m exact boundary stays", 12 * time.Minute, Mode6m, 12 * time.Minute},

		// 15m increments (the default).
		{"15m rounds down below half", 20 * time.Minute, Mode15m, 15 * time.Minute},
		{"15m rounds up at half", 22*time.Minute + 30*time.Second, Mode15m, 30 * time.Minute},
		{"15m exact boundary stays", 30 * time.Minute, Mode15m, 30 * time.Minute},
		{"15m small duration rounds to zero", 3 * time.Minute, Mode15m, 0},
		{"15m just over half rounds to one increment", 8 * time.Minute, Mode15m, 15 * time.Minute},

		// Zero is stable across modes.
		{"zero stays zero", 0, Mode15m, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Round(tt.in, tt.mode); got != tt.want {
				t.Errorf("Round(%v, %v) = %v, want %v", tt.in, tt.mode, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Mode
	}{
		{"off", ModeOff},
		{"5m", Mode5m},
		{"6m", Mode6m},
		{"15m", Mode15m},
		{"  15M ", Mode15m}, // trimmed and case-insensitive
		{"", DefaultMode},
		{"nonsense", DefaultMode},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := Parse(tt.in); got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultModeIs15m(t *testing.T) {
	if DefaultMode != Mode15m {
		t.Errorf("DefaultMode = %v, want Mode15m", DefaultMode)
	}
}
