// Package store holds the persistence backends for reservations.
package store

import (
	"context"
	"sync"

	"github.com/willbengtson/thelastcall/internal/booking"
)

// Memory is an in-process store. It is the dev and test backend; it forgets
// everything when the process exits.
type Memory struct {
	mu       sync.Mutex
	res      map[string]booking.Reservation
	occupied map[string]map[int]map[int]string // date -> hour -> bay -> reservation id
}

// NewMemory builds an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		res:      map[string]booking.Reservation{},
		occupied: map[string]map[int]map[int]string{},
	}
}

var _ booking.Store = (*Memory)(nil)

// Occupied reports the busy bay-hours for a night.
func (m *Memory) Occupied(_ context.Context, date string) (map[int]map[int]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := map[int]map[int]bool{}
	for hour, bays := range m.occupied[date] {
		out[hour] = map[int]bool{}
		for bay := range bays {
			out[hour][bay] = true
		}
	}
	return out, nil
}

// Hold claims every bay-hour for a reservation, or none of them.
func (m *Memory) Hold(_ context.Context, r booking.Reservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		if _, taken := m.occupied[r.Date][h][r.Bay]; taken {
			return booking.ErrSlotTaken
		}
	}
	if m.occupied[r.Date] == nil {
		m.occupied[r.Date] = map[int]map[int]string{}
	}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		if m.occupied[r.Date][h] == nil {
			m.occupied[r.Date][h] = map[int]string{}
		}
		m.occupied[r.Date][h][r.Bay] = r.ID
	}
	m.res[r.ID] = r
	return nil
}

// Get loads a reservation by id.
func (m *Memory) Get(_ context.Context, id string) (booking.Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.res[id]
	if !ok {
		return booking.Reservation{}, booking.ErrNotFound
	}
	return r, nil
}

// Release frees a reservation's bay-hours.
func (m *Memory) Release(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.res[id]
	if !ok {
		return booking.ErrNotFound
	}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		delete(m.occupied[r.Date][h], r.Bay)
		if len(m.occupied[r.Date][h]) == 0 {
			delete(m.occupied[r.Date], h)
		}
	}
	if len(m.occupied[r.Date]) == 0 {
		delete(m.occupied, r.Date)
	}
	delete(m.res, id)
	return nil
}
