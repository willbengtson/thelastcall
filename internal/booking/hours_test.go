package booking

import (
	"testing"
	"time"
)

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return d
}

func TestStartHours(t *testing.T) {
	tests := []struct {
		name     string
		date     string // 2026-09-27 is a Sunday
		duration int
		want     []int
	}{
		// Sunday opens 4PM, last call 11PM: a 1hr bay must be done by 10:30,
		// so the last start is 9PM.
		{"sunday one hour", "2026-09-27", 1, []int{16, 17, 18, 19, 20, 21}},
		{"sunday three hours", "2026-09-27", 3, []int{16, 17, 18, 19}},
		// Friday runs to 2AM.
		{"friday one hour", "2026-10-02", 1, []int{17, 18, 19, 20, 21, 22, 23, 24}},
		{"monday is dark", "2026-09-28", 1, nil},
		{"tuesday is dark", "2026-09-29", 1, nil},
		{"zero duration", "2026-09-27", 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultSchedule.StartHours(date(t, tt.date), tt.duration)
			if len(got) != len(tt.want) {
				t.Fatalf("StartHours = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("StartHours = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestFormatHour(t *testing.T) {
	for _, tt := range []struct {
		hour int
		want string
	}{
		{0, "12AM"}, {11, "11AM"}, {12, "12PM"}, {16, "4PM"},
		{23, "11PM"}, {24, "12AM"}, {25, "1AM"}, {26, "2AM"},
	} {
		if got := FormatHour(tt.hour); got != tt.want {
			t.Errorf("FormatHour(%d) = %q, want %q", tt.hour, got, tt.want)
		}
	}
}

func TestAllows(t *testing.T) {
	sunday := date(t, "2026-09-27")
	if !DefaultSchedule.Allows(sunday, 16, 1) {
		t.Error("4PM one-hour on Sunday should be allowed")
	}
	if DefaultSchedule.Allows(sunday, 22, 1) {
		t.Error("10PM one-hour on Sunday runs past last call")
	}
	if DefaultSchedule.Allows(sunday, 15, 1) {
		t.Error("3PM is before Sunday's doors open")
	}
	if DefaultSchedule.Allows(date(t, "2026-09-28"), 18, 1) {
		t.Error("Monday is dark")
	}
}
