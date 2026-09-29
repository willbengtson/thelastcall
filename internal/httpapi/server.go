package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/willbengtson/thelastcall/internal/booking"
)

// House is the set of standing facts about the room. The frontend reads it so
// the hours on the page and the hours the booking service enforces can never
// drift apart.
type House struct {
	Name     string      `json:"name"`
	Timezone string      `json:"timezone"`
	Bays     int         `json:"bays"`
	MaxParty int         `json:"maxParty"`
	Nights   []HouseHour `json:"nights"`
}

// HouseHour is one weekday's published service window.
type HouseHour struct {
	Weekday  string `json:"weekday"`
	Closed   bool   `json:"closed"`
	Open     string `json:"open,omitempty"`
	LastCall string `json:"lastCall,omitempty"`
}

// Server wires the API handlers and the frontend into one http.Handler.
type Server struct {
	log      *slog.Logger
	bookings *booking.Service
	house    House
	ui       fs.FS
	origins  []string
}

// Options configures the server.
type Options struct {
	Log      *slog.Logger
	Bookings *booking.Service
	House    House
	// UI is the built frontend. Nil serves the API only, which is what the
	// Vite dev server wants.
	UI fs.FS
	// AllowedOrigins are the browser origins permitted to call the API
	// cross-origin.
	AllowedOrigins []string
}

// NewServer builds the server.
func NewServer(opts Options) *Server {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		log:      log,
		bookings: opts.Bookings,
		house:    opts.House,
		ui:       opts.UI,
		origins:  opts.AllowedOrigins,
	}
}

// Handler returns the routed, wrapped handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/house", s.handleHouse)
	mux.HandleFunc("GET /api/v1/availability", s.handleAvailability)
	mux.HandleFunc("POST /api/v1/bookings", s.handleCreateBooking)
	mux.HandleFunc("GET /api/v1/bookings/{id}", s.handleGetBooking)
	mux.HandleFunc("DELETE /api/v1/bookings/{id}", s.handleDeleteBooking)

	// Anything under /api that did not match is a 404 in JSON, not HTML.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, s.log, http.StatusNotFound, "No such door.")
	})

	if s.ui != nil {
		mux.Handle("/", s.uiHandler())
	}

	return recoverPanics(s.log, accessLog(s.log, cors(s.origins, mux)))
}

// uiHandler serves the built frontend, falling back to index.html so the
// client router owns the URL space.
func (s *Server) uiHandler() http.Handler {
	files := http.FileServerFS(s.ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path := r.URL.Path; path != "/" {
			if _, err := fs.Stat(s.ui, cleanFSPath(path)); err != nil {
				// A client route, not a missing asset: hand back the shell.
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		// Hashed assets are immutable; the shell must never be cached.
		if isHashedAsset(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// HouseFromSchedule renders a Schedule as the published hours, so the copy on
// the site is generated from the same source the booking rules use.
func HouseFromSchedule(name, tz string, sched booking.Schedule, bays, maxParty int) House {
	h := House{Name: name, Timezone: tz, Bays: bays, MaxParty: maxParty}
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		entry := HouseHour{Weekday: wd.String()}
		if win, open := sched[wd]; open {
			entry.Open = booking.FormatHour(win.Open)
			entry.LastCall = booking.FormatHour(win.LastCall)
		} else {
			entry.Closed = true
		}
		h.Nights = append(h.Nights, entry)
	}
	return h
}
