package drill

import (
	"testing"
	"time"
)

func TestIsDue(t *testing.T) {
	weekly := 7 * 24 * time.Hour

	// Never run → always due.
	if !IsDue(time.Time{}, weekly) {
		t.Error("expected IsDue=true for zero lastRun")
	}

	// Just ran → not due.
	if IsDue(time.Now(), weekly) {
		t.Error("expected IsDue=false when lastRun is now")
	}

	// Ran more than a week ago → due.
	old := time.Now().Add(-8 * 24 * time.Hour)
	if !IsDue(old, weekly) {
		t.Error("expected IsDue=true when lastRun is 8 days ago")
	}
}

func TestParseCadence(t *testing.T) {
	cases := []struct {
		s    string
		want time.Duration
		ok   bool
	}{
		{"daily", 24 * time.Hour, true},
		{"weekly", 7 * 24 * time.Hour, true},
		{"monthly", 30 * 24 * time.Hour, true},
		{"hourly", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseCadence(c.s)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseCadence(%q) = (%v, %v), want (%v, %v)", c.s, got, ok, c.want, c.ok)
		}
	}
}

func TestFormatCadence(t *testing.T) {
	if got := FormatCadence(7 * 24 * time.Hour); got != "weekly" {
		t.Errorf("want 'weekly', got %q", got)
	}
	if got := FormatCadence(30 * 24 * time.Hour); got != "monthly" {
		t.Errorf("want 'monthly', got %q", got)
	}
}
