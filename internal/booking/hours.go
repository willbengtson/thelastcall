package booking

import (
	"fmt"
	"time"
)

// Window is a night of service, expressed as hours from midnight of the
// calendar day the night belongs to. LastCall may exceed 24: Friday's
// "26" means 2AM Saturday, but it is still Friday night.
type Window struct {
	Open     int `json:"open"`
	LastCall int `json:"lastCall"`
}

// Schedule maps a weekday to its service window. A missing weekday is dark.
type Schedule map[time.Weekday]Window

// DefaultSchedule mirrors the hours published on the Hours page.
var DefaultSchedule = Schedule{
	time.Sunday:    {Open: 16, LastCall: 23},
	time.Wednesday: {Open: 18, LastCall: 24},
	time.Thursday:  {Open: 18, LastCall: 25},
	time.Friday:    {Open: 17, LastCall: 26},
	time.Saturday:  {Open: 16, LastCall: 26},
}

// Window returns the service window for a date, and whether we are open at all.
func (s Schedule) Window(day time.Time) (Window, bool) {
	w, ok := s[day.Weekday()]
	return w, ok
}

// StartHours lists the bookable tee times for a night at the given duration.
// A bay has to be off the tee half an hour before last call.
func (s Schedule) StartHours(day time.Time, durationHours int) []int {
	w, open := s.Window(day)
	if !open || durationHours <= 0 {
		return nil
	}
	var out []int
	for t := w.Open; float64(t+durationHours) <= float64(w.LastCall)-0.5; t++ {
		out = append(out, t)
	}
	return out
}

// Allows reports whether a booking of durationHours starting at startHour
// fits inside the night's window.
func (s Schedule) Allows(day time.Time, startHour, durationHours int) bool {
	for _, t := range s.StartHours(day, durationHours) {
		if t == startHour {
			return true
		}
	}
	return false
}

// FormatHour renders an absolute night hour the way the bar says it out loud:
// 16 -> "4PM", 25 -> "1AM".
func FormatHour(h int) string {
	hr := ((h % 24) + 24) % 24
	suffix := "AM"
	if hr >= 12 {
		suffix = "PM"
	}
	display := hr % 12
	if display == 0 {
		display = 12
	}
	return fmt.Sprintf("%d%s", display, suffix)
}
