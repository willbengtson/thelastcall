// Package httpapi serves the JSON API and the built frontend.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/willbengtson/thelastcall/internal/booking"
)

// errorBody is the shape of every API error. The message is written to be
// read by a guest, not a developer.
type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("writing response body", "error", err)
	}
}

func writeError(w http.ResponseWriter, log *slog.Logger, status int, msg string) {
	writeJSON(w, log, status, errorBody{Error: msg})
}

// writeDomainError maps a booking error to a status a browser can act on.
// Anything unrecognized is logged and reported as a server fault, so we never
// leak internals to the guest.
func writeDomainError(w http.ResponseWriter, log *slog.Logger, err error) {
	switch {
	case errors.Is(err, booking.ErrNotFound):
		writeError(w, log, http.StatusNotFound, "No reservation under that name.")
	case errors.Is(err, booking.ErrNoBay):
		writeError(w, log, http.StatusConflict, "Every bay is taken for that stretch. Try another tee time.")
	case errors.Is(err, booking.ErrClosed):
		writeError(w, log, http.StatusUnprocessableEntity, "The lights are off that night.")
	case errors.Is(err, booking.ErrOutsideHours):
		writeError(w, log, http.StatusUnprocessableEntity, "That tee time is outside the night's window.")
	case errors.Is(err, booking.ErrPartyTooBig):
		writeError(w, log, http.StatusUnprocessableEntity, "That party is larger than a bay holds.")
	case errors.Is(err, booking.ErrBadRequest):
		writeError(w, log, http.StatusBadRequest, "That booking does not make sense.")
	default:
		log.Error("unhandled request failure", "error", err)
		writeError(w, log, http.StatusInternalServerError, "Something went wrong behind the bar. Try again.")
	}
}
