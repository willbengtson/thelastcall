package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/willbengtson/thelastcall/internal/booking"
	"github.com/willbengtson/thelastcall/internal/store"
)

// noon on Sunday 2026-09-27, before doors open.
var frozenNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	svc := booking.NewService(store.NewMemory(), booking.Options{
		Bays: 2, MaxParty: 6, Window: 7, Lead: 30 * time.Minute,
		Now: func() time.Time { return frozenNow },
	})
	return NewServer(Options{
		Log:      slog.New(slog.DiscardHandler),
		Bookings: svc,
		House:    HouseFromSchedule("The Last Call", "America/Los_Angeles", booking.DefaultSchedule, 2, 6),
	}).Handler()
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAvailabilityEndpoint(t *testing.T) {
	rec := do(t, testHandler(t), http.MethodGet, "/api/v1/availability?party=4&hours=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var avail booking.Availability
	if err := json.Unmarshal(rec.Body.Bytes(), &avail); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if avail.Party != 4 || avail.Hours != 2 || avail.BayCount != 2 {
		t.Errorf("echoed request = %+v", avail)
	}
	if len(avail.Nights) != 7 {
		t.Errorf("got %d nights, want 7", len(avail.Nights))
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestAvailabilityRejectsJunkQuery(t *testing.T) {
	rec := do(t, testHandler(t), http.MethodGet, "/api/v1/availability?party=lots", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestBookingRoundTrip(t *testing.T) {
	h := testHandler(t)

	rec := do(t, h, http.MethodPost, "/api/v1/bookings",
		`{"date":"2026-09-27","startHour":18,"hours":1,"party":4,"guestName":"Bengtson"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var res booking.Reservation
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if res.ID == "" || res.Code == "" || res.Bay == 0 {
		t.Fatalf("incomplete reservation: %+v", res)
	}
	if want := "/api/v1/bookings/" + res.ID; rec.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), want)
	}

	got := do(t, h, http.MethodGet, "/api/v1/bookings/"+res.ID, "")
	if got.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", got.Code, got.Body)
	}

	gone := do(t, h, http.MethodDelete, "/api/v1/bookings/"+res.ID, "")
	if gone.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, body = %s", gone.Code, gone.Body)
	}
	if after := do(t, h, http.MethodGet, "/api/v1/bookings/"+res.ID, ""); after.Code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", after.Code)
	}
}

func TestBookingErrorStatuses(t *testing.T) {
	h := testHandler(t)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"dark night", `{"date":"2026-09-28","startHour":18,"hours":1,"party":2}`, http.StatusUnprocessableEntity},
		{"past last call", `{"date":"2026-09-27","startHour":22,"hours":1,"party":2}`, http.StatusUnprocessableEntity},
		{"party too big", `{"date":"2026-09-27","startHour":18,"hours":1,"party":12}`, http.StatusUnprocessableEntity},
		{"bad date", `{"date":"whenever","startHour":18,"hours":1,"party":2}`, http.StatusBadRequest},
		{"malformed json", `{`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
		{"unknown field", `{"date":"2026-09-27","startHour":18,"hours":1,"party":2,"bay":3}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := do(t, h, http.MethodPost, "/api/v1/bookings", tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestBookingConflictWhenBaysFill(t *testing.T) {
	h := testHandler(t)
	body := `{"date":"2026-09-27","startHour":19,"hours":1,"party":2}`
	for i := 0; i < 2; i++ {
		if rec := do(t, h, http.MethodPost, "/api/v1/bookings", body); rec.Code != http.StatusCreated {
			t.Fatalf("hold %d: status = %d, body = %s", i+1, rec.Code, rec.Body)
		}
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/bookings", body); rec.Code != http.StatusConflict {
		t.Errorf("third hold status = %d, want 409 (body %s)", rec.Code, rec.Body)
	}
}

func TestHouseEndpointPublishesHours(t *testing.T) {
	rec := do(t, testHandler(t), http.MethodGet, "/api/v1/house", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var house House
	if err := json.Unmarshal(rec.Body.Bytes(), &house); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(house.Nights) != 7 {
		t.Fatalf("got %d weekdays, want 7", len(house.Nights))
	}
	if !house.Nights[1].Closed || !house.Nights[2].Closed {
		t.Error("Monday and Tuesday should be published as dark")
	}
	if house.Nights[5].Open != "5PM" || house.Nights[5].LastCall != "2AM" {
		t.Errorf("Friday = %+v, want 5PM to 2AM", house.Nights[5])
	}
}

func TestUnknownAPIPathIsJSON(t *testing.T) {
	rec := do(t, testHandler(t), http.MethodGet, "/api/v1/cellar", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}
