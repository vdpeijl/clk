package cmd

import (
	"testing"
	"time"
)

func TestPeriodRange(t *testing.T) {
	// Saturday 2026-06-13 14:30 local-as-UTC for determinism.
	now := time.Date(2026, 6, 13, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		period string
		start  time.Time
		end    time.Time
	}{
		{
			period: "today",
			start:  time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),
			end:    time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			period: "yesterday",
			start:  time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
			end:    time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			// Week starts Monday 2026-06-08.
			period: "week",
			start:  time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC),
			end:    time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			period: "month",
			start:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			end:    time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			start, end, err := periodRange(tt.period, now)
			if err != nil {
				t.Fatalf("periodRange: %v", err)
			}
			if !start.Equal(tt.start) {
				t.Errorf("start = %v, want %v", start, tt.start)
			}
			if !end.Equal(tt.end) {
				t.Errorf("end = %v, want %v", end, tt.end)
			}
		})
	}
}

func TestPeriodRangeUnknown(t *testing.T) {
	if _, _, err := periodRange("decade", time.Now()); err == nil {
		t.Fatal("expected error for unknown period")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{28 * time.Minute, "28m"},
		{time.Hour + 5*time.Minute, "1h05m"},
		{2 * time.Hour, "2h00m"},
		{90 * time.Second, "2m"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
