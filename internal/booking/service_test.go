package booking

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeStore is a minimal in-test Store. It mirrors the real memory store but
// lives here so the domain package tests do not import a backend.
type fakeStore struct {
	mu       sync.Mutex
	res      map[string]Reservation
	occupied map[string]map[int]map[int]bool
	holds    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		res:      map[string]Reservation{},
		occupied: map[string]map[int]map[int]bool{},
	}
}

func (f *fakeStore) Occupied(_ context.Context, date string) (map[int]map[int]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]map[int]bool{}
	for h, bays := range f.occupied[date] {
		out[h] = map[int]bool{}
		for b := range bays {
			out[h][b] = true
		}
	}
	return out, nil
}

func (f *fakeStore) Hold(_ context.Context, r Reservation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.holds++
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		if f.occupied[r.Date][h][r.Bay] {
			return ErrSlotTaken
		}
	}
	if f.occupied[r.Date] == nil {
		f.occupied[r.Date] = map[int]map[int]bool{}
	}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		if f.occupied[r.Date][h] == nil {
			f.occupied[r.Date][h] = map[int]bool{}
		}
		f.occupied[r.Date][h][r.Bay] = true
	}
	f.res[r.ID] = r
	return nil
}

func (f *fakeStore) Get(_ context.Context, id string) (Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.res[id]
	if !ok {
		return Reservation{}, ErrNotFound
	}
	return r, nil
}

func (f *fakeStore) Release(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.res[id]
	if !ok {
		return ErrNotFound
	}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		delete(f.occupied[r.Date][h], r.Bay)
	}
	delete(f.res, id)
	return nil
}

// frozenNow is a Sunday at noon, before doors open, so every Sunday tee time
// is still bookable.
var frozenNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)

func testService(t *testing.T, store Store, bays int) *Service {
	t.Helper()
	return NewService(store, Options{
		Bays:     bays,
		MaxParty: 6,
		Window:   7,
		Lead:     30 * time.Minute,
		Now:      func() time.Time { return frozenNow },
	})
}

func TestAvailabilityMarksDarkNights(t *testing.T) {
	svc := testService(t, newFakeStore(), 3)
	avail, err := svc.Availability(context.Background(), 2, 1)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	if len(avail.Nights) != 7 {
		t.Fatalf("got %d nights, want 7", len(avail.Nights))
	}
	if avail.Nights[0].Label != "Tonight" {
		t.Errorf("first night label = %q, want %q", avail.Nights[0].Label, "Tonight")
	}
	// Sunday the 27th, so Monday and Tuesday are nights 1 and 2.
	for _, i := range []int{1, 2} {
		if !avail.Nights[i].Closed {
			t.Errorf("night %d (%s) should be dark", i, avail.Nights[i].Date)
		}
		if len(avail.Nights[i].Slots) != 0 {
			t.Errorf("night %d should have no slots, got %d", i, len(avail.Nights[i].Slots))
		}
	}
	if avail.Nights[0].Closed {
		t.Error("Sunday should be open")
	}
}

func TestAvailabilityHidesPastTeeTimes(t *testing.T) {
	store := newFakeStore()
	// 8:45PM Sunday: the 4PM through 8PM tee times are gone, and 9PM is
	// inside the 30-minute lead, so the night is fully booked out.
	svc := NewService(store, Options{
		Bays: 3, MaxParty: 6, Window: 7, Lead: 30 * time.Minute,
		Now: func() time.Time { return time.Date(2026, 9, 27, 20, 45, 0, 0, time.Local) },
	})
	avail, err := svc.Availability(context.Background(), 2, 1)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	for _, slot := range avail.Nights[0].Slots {
		if !slot.Taken {
			t.Errorf("slot %s should be past or inside the lead time", slot.Label)
		}
	}
	if !avail.Nights[0].Closed {
		t.Error("a night with no reachable tee time should read as closed")
	}
}

func TestHoldFillsBaysThenConflicts(t *testing.T) {
	store := newFakeStore()
	svc := testService(t, store, 2)
	req := Request{Date: "2026-09-27", StartHour: 18, Hours: 1, Party: 4}

	first, err := svc.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("first hold: %v", err)
	}
	second, err := svc.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("second hold: %v", err)
	}
	if first.Bay == second.Bay {
		t.Errorf("two holds landed on the same bay %d", first.Bay)
	}
	if first.Code == "" || len(first.Code) != 6 {
		t.Errorf("code = %q, want six characters", first.Code)
	}
	if _, err := svc.Hold(context.Background(), req); !errors.Is(err, ErrNoBay) {
		t.Errorf("third hold error = %v, want ErrNoBay", err)
	}

	// The slot now reads as taken on the availability strip.
	avail, err := svc.Availability(context.Background(), 4, 1)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	for _, slot := range avail.Nights[0].Slots {
		if slot.Hour == 18 && !slot.Taken {
			t.Error("6PM should read as taken once both bays are held")
		}
	}

	// Releasing one frees the stretch again.
	if err := svc.Release(context.Background(), first.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := svc.Hold(context.Background(), req); err != nil {
		t.Fatalf("hold after release: %v", err)
	}
}

func TestHoldSpansEveryHourOfTheStretch(t *testing.T) {
	store := newFakeStore()
	svc := testService(t, store, 1)

	if _, err := svc.Hold(context.Background(), Request{
		Date: "2026-09-27", StartHour: 17, Hours: 3, Party: 2,
	}); err != nil {
		t.Fatalf("three-hour hold: %v", err)
	}
	// The single bay is busy 5PM-8PM, so a one-hour slot at 7PM must be gone.
	for _, hour := range []int{17, 18, 19} {
		_, err := svc.Hold(context.Background(), Request{
			Date: "2026-09-27", StartHour: hour, Hours: 1, Party: 2,
		})
		if !errors.Is(err, ErrNoBay) {
			t.Errorf("hold at %s: error = %v, want ErrNoBay", FormatHour(hour), err)
		}
	}
	// 8PM is clear.
	if _, err := svc.Hold(context.Background(), Request{
		Date: "2026-09-27", StartHour: 20, Hours: 1, Party: 2,
	}); err != nil {
		t.Errorf("hold at 8PM: %v", err)
	}
}

func TestHoldRetriesAnotherBayAfterLosingTheRace(t *testing.T) {
	// racyStore lets the first Hold attempt lose, as a conditional write does
	// when a concurrent booking claims the bay between our read and write.
	store := newFakeStore()
	racy := &raceOnceStore{fakeStore: store}
	svc := testService(t, racy, 3)

	res, err := svc.Hold(context.Background(), Request{
		Date: "2026-09-27", StartHour: 18, Hours: 1, Party: 2,
	})
	if err != nil {
		t.Fatalf("Hold should recover from a lost race: %v", err)
	}
	if racy.attempts < 2 {
		t.Errorf("attempts = %d, want the service to retry", racy.attempts)
	}
	if res.Bay == 0 {
		t.Error("recovered hold has no bay")
	}
}

type raceOnceStore struct {
	*fakeStore
	attempts int
}

func (r *raceOnceStore) Hold(ctx context.Context, res Reservation) error {
	r.attempts++
	if r.attempts == 1 {
		return ErrSlotTaken
	}
	return r.fakeStore.Hold(ctx, res)
}

func TestHoldRejectsBadRequests(t *testing.T) {
	svc := testService(t, newFakeStore(), 3)
	tests := []struct {
		name string
		req  Request
		want error
	}{
		{"dark night", Request{Date: "2026-09-28", StartHour: 18, Hours: 1, Party: 2}, ErrClosed},
		{"before doors", Request{Date: "2026-09-27", StartHour: 10, Hours: 1, Party: 2}, ErrOutsideHours},
		{"past last call", Request{Date: "2026-09-27", StartHour: 22, Hours: 1, Party: 2}, ErrOutsideHours},
		{"party too big", Request{Date: "2026-09-27", StartHour: 18, Hours: 1, Party: 9}, ErrPartyTooBig},
		{"unparseable date", Request{Date: "tonight", StartHour: 18, Hours: 1, Party: 2}, ErrBadRequest},
		{"zero hours", Request{Date: "2026-09-27", StartHour: 18, Hours: 0, Party: 2}, ErrBadRequest},
		{"beyond the window", Request{Date: "2026-11-01", StartHour: 18, Hours: 1, Party: 2}, ErrBadRequest},
		{"in the past", Request{Date: "2026-09-20", StartHour: 18, Hours: 1, Party: 2}, ErrBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Hold(context.Background(), tt.req); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestHoldIsSafeUnderConcurrency(t *testing.T) {
	store := newFakeStore()
	const bays = 3
	svc := testService(t, store, bays)
	req := Request{Date: "2026-09-27", StartHour: 18, Hours: 2, Party: 2}

	var wg sync.WaitGroup
	results := make(chan Reservation, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := svc.Hold(context.Background(), req); err == nil {
				results <- res
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := map[int]bool{}
	for res := range results {
		if seen[res.Bay] {
			t.Errorf("bay %d was double-booked", res.Bay)
		}
		seen[res.Bay] = true
	}
	if len(seen) == 0 || len(seen) > bays {
		t.Errorf("held %d bays, want between 1 and %d", len(seen), bays)
	}
}
