package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/willbengtson/thelastcall/internal/booking"
)

// maxBodyBytes caps a request body. A reservation is a few hundred bytes.
const maxBodyBytes = 8 << 10

// handleAvailability renders the seven-night strip for a party and duration.
//
//	GET /api/v1/availability?party=2&hours=1
func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request) {
	party, err := queryInt(r, "party", 2)
	if err != nil {
		writeError(w, s.log, http.StatusBadRequest, "party must be a number.")
		return
	}
	hours, err := queryInt(r, "hours", 1)
	if err != nil {
		writeError(w, s.log, http.StatusBadRequest, "hours must be a number.")
		return
	}

	avail, err := s.bookings.Availability(r.Context(), party, hours)
	if err != nil {
		writeDomainError(w, s.log, err)
		return
	}
	// Availability turns over as bays fill; never let a proxy hold it.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, s.log, http.StatusOK, avail)
}

// handleCreateBooking holds a bay.
//
//	POST /api/v1/bookings
func (s *Server) handleCreateBooking(w http.ResponseWriter, r *http.Request) {
	var req booking.Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, s.log, http.StatusBadRequest, "Tell us when you want the bay.")
			return
		}
		writeError(w, s.log, http.StatusBadRequest, "We could not read that booking.")
		return
	}

	res, err := s.bookings.Hold(r.Context(), req)
	if err != nil {
		writeDomainError(w, s.log, err)
		return
	}
	s.log.Info("bay held",
		"reservation", res.ID, "date", res.Date, "hour", res.StartHour,
		"hours", res.Hours, "bay", res.Bay, "party", res.Party,
	)
	w.Header().Set("Location", "/api/v1/bookings/"+res.ID)
	writeJSON(w, s.log, http.StatusCreated, res)
}

// handleGetBooking returns one reservation.
//
//	GET /api/v1/bookings/{id}
func (s *Server) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	res, err := s.bookings.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, s.log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, s.log, http.StatusOK, res)
}

// handleDeleteBooking releases a reservation.
//
//	DELETE /api/v1/bookings/{id}
func (s *Server) handleDeleteBooking(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.bookings.Release(r.Context(), id); err != nil {
		writeDomainError(w, s.log, err)
		return
	}
	s.log.Info("bay released", "reservation", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleHouse publishes the house facts the frontend renders as copy: hours,
// bay count, party limit.
//
//	GET /api/v1/house
func (s *Server) handleHouse(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.log, http.StatusOK, s.house)
}

// handleHealth is the load balancer's probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.log, http.StatusOK, map[string]string{"status": "open"})
}

func queryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}
