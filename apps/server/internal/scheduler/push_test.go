package scheduler

import (
	"testing"
	"time"
)

func TestMissedAt(t *testing.T) {
	last := time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC)
	period := 24*time.Hour + 5*time.Minute

	for _, tc := range []struct {
		name   string
		now    time.Time
		want   time.Time
		missed bool
	}{
		{"just pinged", last.Add(time.Minute), last.Add(period), false},
		{"inside grace", last.Add(24*time.Hour + 4*time.Minute), last.Add(period), false},
		{"at the deadline", last.Add(period), last.Add(period), true},
		{"past the deadline", last.Add(period + time.Hour), last.Add(period), true},
		{"long silence skips ahead", last.Add(3*period + time.Minute), last.Add(3 * period), true},
	} {
		got, missed := missedAt(last, tc.now, period)
		if !got.Equal(tc.want) || missed != tc.missed {
			t.Errorf("%s: missedAt = %v, %v; want %v, %v", tc.name, got, missed, tc.want, tc.missed)
		}
	}

	// Two owners reading the same last result agree on the slot even if
	// their clocks differ by a little.
	a, _ := missedAt(last, last.Add(period+time.Second), period)
	b, _ := missedAt(last, last.Add(period+3*time.Second), period)
	if !a.Equal(b) {
		t.Errorf("owners disagree: %v vs %v", a, b)
	}
}

func TestShortDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		24 * time.Hour:               "24h",
		24*time.Hour + 5*time.Minute: "24h5m",
		90 * time.Second:             "1m30s",
		2 * time.Minute:              "2m",
		time.Hour + 30*time.Second:   "1h0m30s",
		1500 * time.Millisecond:      "1.5s",
	} {
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
