package booking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSlotTaken is what a Store returns when someone else won the race for a
// bay-hour between our availability read and our write.
var ErrSlotTaken = errors.New("bay-hour already held")

// Store persists reservations and the bay-hours they occupy.
type Store interface {
	// Occupied reports which bays are busy in which hours of a night,
	// keyed hour -> bay.
	Occupied(ctx context.Context, date string) (map[int]map[int]bool, error)
	// Hold writes a reservation and claims every bay-hour it covers,
	// atomically. It returns ErrSlotTaken if any of those bay-hours was
	// claimed in the meantime.
	Hold(ctx context.Context, r Reservation) error
	// Get loads a reservation by id.
	Get(ctx context.Context, id string) (Reservation, error)
	// Release drops a reservation and frees its bay-hours.
	Release(ctx context.Context, id string) error
}

// Service is the booking rulebook: who fits, when we are open, which bay is free.
type Service struct {
	store    Store
	sched    Schedule
	bays     int
	maxParty int
	window   int // how many nights ahead the strip shows
	lead     time.Duration
	now      func() time.Time
}

// Options configures a Service. Zero fields fall back to house defaults.
type Options struct {
	Schedule Schedule
	Bays     int
	MaxParty int
	Window   int
	Lead     time.Duration
	Now      func() time.Time
}

// NewService builds the booking service.
func NewService(store Store, opts Options) *Service {
	s := &Service{
		store:    store,
		sched:    opts.Schedule,
		bays:     opts.Bays,
		maxParty: opts.MaxParty,
		window:   opts.Window,
		lead:     opts.Lead,
		now:      opts.Now,
	}
	if s.sched == nil {
		s.sched = DefaultSchedule
	}
	if s.bays <= 0 {
		s.bays = 3
	}
	if s.maxParty <= 0 {
		s.maxParty = 6
	}
	if s.window <= 0 {
		s.window = 7
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// BayCount is how many sim bays the house runs.
func (s *Service) BayCount() int { return s.bays }

// MaxParty is the largest party a single bay seats.
func (s *Service) MaxParty() int { return s.maxParty }

// Availability renders the seven-night strip for a given party and duration.
func (s *Service) Availability(ctx context.Context, party, hours int) (Availability, error) {
	if party <= 0 {
		party = 2
	}
	if hours <= 0 {
		hours = 1
	}
	if party > s.maxParty {
		return Availability{}, fmt.Errorf("%w: %d", ErrPartyTooBig, party)
	}

	today := s.today()
	out := Availability{Party: party, Hours: hours, BayCount: s.bays}
	for i := 0; i < s.window; i++ {
		day := today.AddDate(0, 0, i)
		night := Night{
			Date:    day.Format(DateLayout),
			Weekday: day.Weekday().String()[:3],
			DayOfMo: day.Day(),
			Label:   day.Format("Monday, Jan 2"),
		}
		if i == 0 {
			night.Label = "Tonight"
		}

		starts := s.sched.StartHours(day, hours)
		if len(starts) == 0 {
			night.Closed = true
			out.Nights = append(out.Nights, night)
			continue
		}

		occupied, err := s.store.Occupied(ctx, night.Date)
		if err != nil {
			return Availability{}, fmt.Errorf("reading bay-hours for %s: %w", night.Date, err)
		}
		for _, t := range starts {
			night.Slots = append(night.Slots, Slot{
				Hour:  t,
				Label: FormatHour(t),
				Taken: len(s.freeBays(occupied, t, hours)) == 0 || s.tooLate(day, t),
			})
		}
		// Every tee time gone (or gone by) still means a dark night to the guest.
		night.Closed = !anyOpen(night.Slots)
		out.Nights = append(out.Nights, night)
	}
	return out, nil
}

// Hold claims the first free bay for a request, or explains why it cannot.
func (s *Service) Hold(ctx context.Context, req Request) (Reservation, error) {
	day, err := ParseDate(req.Date)
	if err != nil {
		return Reservation{}, fmt.Errorf("%w: date %q", ErrBadRequest, req.Date)
	}
	if req.Party <= 0 || req.Hours <= 0 {
		return Reservation{}, fmt.Errorf("%w: party and hours must be positive", ErrBadRequest)
	}
	if req.Party > s.maxParty {
		return Reservation{}, fmt.Errorf("%w: %d", ErrPartyTooBig, req.Party)
	}
	if day.Before(s.today()) || day.After(s.today().AddDate(0, 0, s.window-1)) {
		return Reservation{}, fmt.Errorf("%w: we only hold bays %d nights out", ErrBadRequest, s.window)
	}
	if _, open := s.sched.Window(day); !open {
		return Reservation{}, ErrClosed
	}
	if !s.sched.Allows(day, req.StartHour, req.Hours) {
		return Reservation{}, ErrOutsideHours
	}
	if s.tooLate(day, req.StartHour) {
		return Reservation{}, fmt.Errorf("%w: that tee time has already passed", ErrOutsideHours)
	}

	// One retry per bay: a conditional-write loss means that bay filled up
	// under us, so try the next one rather than failing the guest.
	for attempt := 0; attempt < s.bays; attempt++ {
		occupied, err := s.store.Occupied(ctx, req.Date)
		if err != nil {
			return Reservation{}, fmt.Errorf("reading bay-hours: %w", err)
		}
		free := s.freeBays(occupied, req.StartHour, req.Hours)
		if len(free) == 0 {
			return Reservation{}, ErrNoBay
		}

		res := Reservation{
			ID:        newID(),
			Code:      newCode(),
			Date:      req.Date,
			StartHour: req.StartHour,
			Hours:     req.Hours,
			Bay:       free[0],
			Party:     req.Party,
			GuestName: strings.TrimSpace(req.GuestName),
			Contact:   strings.TrimSpace(req.Contact),
			CreatedAt: s.now().UTC(),
		}
		switch err := s.store.Hold(ctx, res); {
		case err == nil:
			return res, nil
		case errors.Is(err, ErrSlotTaken):
			continue
		default:
			return Reservation{}, fmt.Errorf("holding bay %d: %w", res.Bay, err)
		}
	}
	return Reservation{}, ErrNoBay
}

// Get loads a reservation.
func (s *Service) Get(ctx context.Context, id string) (Reservation, error) {
	return s.store.Get(ctx, id)
}

// Release cancels a reservation.
func (s *Service) Release(ctx context.Context, id string) error {
	return s.store.Release(ctx, id)
}

// freeBays lists bays idle for every hour of the stretch, lowest first.
func (s *Service) freeBays(occupied map[int]map[int]bool, startHour, hours int) []int {
	var free []int
	for bay := 1; bay <= s.bays; bay++ {
		clear := true
		for h := startHour; h < startHour+hours; h++ {
			if occupied[h][bay] {
				clear = false
				break
			}
		}
		if clear {
			free = append(free, bay)
		}
	}
	return free
}

// tooLate reports whether a tee time is already behind us, allowing for the
// lead time the bar needs to pour and set up.
func (s *Service) tooLate(day time.Time, startHour int) bool {
	tee := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).
		Add(time.Duration(startHour) * time.Hour)
	return tee.Before(s.now().Add(s.lead))
}

// today is midnight of the current night, in the house's timezone.
func (s *Service) today() time.Time {
	n := s.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

func anyOpen(slots []Slot) bool {
	for _, sl := range slots {
		if !sl.Taken {
			return true
		}
	}
	return false
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func newID() string { return randomString(16, idAlphabet) }

// newCode is what the guest reads back at the door.
func newCode() string { return randomString(6, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789") }

func randomString(n int, alphabet string) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("booking: no randomness available: %v", err))
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}
