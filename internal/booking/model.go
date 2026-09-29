package booking

import (
	"errors"
	"time"
)

// DateLayout is the wire format for a night: the calendar date the night starts on.
const DateLayout = "2006-01-02"

// Reservation is a held bay.
type Reservation struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Date      string    `json:"date"`
	StartHour int       `json:"startHour"`
	Hours     int       `json:"hours"`
	Bay       int       `json:"bay"`
	Party     int       `json:"party"`
	GuestName string    `json:"guestName,omitempty"`
	Contact   string    `json:"contact,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Request is an attempt to hold a bay.
type Request struct {
	Date      string `json:"date"`
	StartHour int    `json:"startHour"`
	Hours     int    `json:"hours"`
	Party     int    `json:"party"`
	GuestName string `json:"guestName,omitempty"`
	Contact   string `json:"contact,omitempty"`
}

// Slot is one tee time on the grid.
type Slot struct {
	Hour  int    `json:"hour"`
	Label string `json:"label"`
	Taken bool   `json:"taken"`
}

// Night is a bookable (or dark) date on the seven-day strip.
type Night struct {
	Date    string `json:"date"`
	Weekday string `json:"weekday"`
	DayOfMo int    `json:"dayOfMonth"`
	Label   string `json:"label"`
	Closed  bool   `json:"closed"`
	Slots   []Slot `json:"slots"`
}

// Availability is everything the booking screen needs to render.
type Availability struct {
	Party    int     `json:"party"`
	Hours    int     `json:"hours"`
	BayCount int     `json:"bayCount"`
	Nights   []Night `json:"nights"`
}

// Domain errors the HTTP layer translates into status codes.
var (
	ErrClosed       = errors.New("we are dark that night")
	ErrOutsideHours = errors.New("that tee time is outside the night's window")
	ErrNoBay        = errors.New("every bay is taken for that stretch")
	ErrPartyTooBig  = errors.New("that party is larger than a bay holds")
	ErrBadRequest   = errors.New("that booking does not make sense")
	ErrNotFound     = errors.New("no such reservation")
)

// ParseDate reads a wire date as a local calendar day.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, s, time.Local)
}
