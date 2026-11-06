package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/model"
	"github.com/qw145/hotelbooking/service"
	"gorm.io/gorm"
)

const maxReferenceAttempts = 8

var (
	ErrParamSize = errors.New("param length too long")
	ErrService   = errors.New("no service")
)

type Ping func(context.Context) error

type API struct {
	driver   helper.Driver
	db       *gorm.DB
	log      *slog.Logger
	service  service.Service
	ping     Ping
	refs     helper.RefGenerator
	notifier *service.Pool
}

func NewAPI(service service.Service, ping Ping, notifier *service.Pool) (*API, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: no service", ErrService)
	}
	return &API{
		driver:   helper.DriverSQLite,
		db:       helper.Db,
		log:      helper.Log,
		service:  service,
		ping:     ping,
		refs:     helper.CryptoRefGenerator{},
		notifier: notifier,
	}, nil
}

// Handler returns the fully-wired HTTP handler.
func (a *API) Handler() *http.ServeMux {
	mux := http.NewServeMux()

	// --- Hotels -----------------------------------------------------------
	mux.HandleFunc("GET /api/v1/hotels", a.handleFindHotels)
	mux.HandleFunc("GET /api/v1/hotels/{id}", a.handleGetHotel)
	mux.HandleFunc("GET /api/v1/hotels/{id}/rooms", a.handleListRooms)
	mux.HandleFunc("GET /api/v1/hotels/{id}/availability", a.handleAvailability)

	// --- Bookings ---------------------------------------------------------
	mux.HandleFunc("POST /api/v1/bookings", a.handleCreateBooking)
	mux.HandleFunc("GET /api/v1/bookings/{reference}", a.handleGetBooking)

	// --- Operational ------------------------------------------------------
	mux.HandleFunc("GET /healthz", a.handleHealth)

	// --- Test support -----------------------------------------------------

	mux.HandleFunc("POST /api/v1/test/seed", a.handleSeed)
	mux.HandleFunc("POST /api/v1/test/reset", a.handleReset)

	// Unknown paths: JSON 404 rather than net/http's plain-text default.
	mux.HandleFunc("/", a.handleNotFound)

	return mux
}

// ---------------------------------------------------------------------------
// Hotels
// ---------------------------------------------------------------------------

// GET /api/v1/hotels?name=plaza
func (a *API) handleFindHotels(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if len(name) > 200 {
		writeError(w, r, a.log, ErrParamSize)
		return
	}

	hotels, err := a.service.FindHotelsByName(r.Context(), name)
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}

	writeJSON(w, a.log, http.StatusOK, map[string]interface{}{
		"hotels": hotels,
		"count":  len(hotels),
	})
}

// handleGetHotel returns one hotel by ID.
func (a *API) handleGetHotel(w http.ResponseWriter, r *http.Request) {
	hotelId, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || hotelId < 1 {
		writeError(w, r, a.log, err)
	}

	hotel, err := a.service.GetHotel(r.Context(), uint(hotelId))
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}

	writeJSON(w, a.log, http.StatusOK, hotel)
}

// handleListRooms returns every room in a hotel.
func (a *API) handleListRooms(w http.ResponseWriter, r *http.Request) {
	hotelId, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || hotelId < 1 {
		writeError(w, r, a.log, err)
	}

	hotel, err := a.service.ListRooms(r.Context(), uint(hotelId))
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}

	writeJSON(w, a.log, http.StatusOK, hotel)
}

// GET /api/v1/hotels/{id}/availability?check_in=2026-03-01&check_out=2026-03-03&guests=2
func (a *API) handleAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	var details []string

	hotelId, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || hotelId < 1 {
		details = append(details, "invalid path parameter id must be a positive integer and greater than zero")
	}
	guests, inErr := strconv.Atoi(q.Get("guests"))
	if inErr != nil {
		details = append(details, "invalid query parameter guests must be an integer")
	}
	if inErr == nil && guests < 1 {
		details = append(details, "guests must be at least 1")
	}

	checkIn, inErr := time.Parse(time.DateOnly, q.Get("check_in"))
	if inErr != nil {
		details = append(details, "check_in invalid date format")
	}
	checkOut, outErr := time.Parse(time.DateOnly, q.Get("check_out"))
	if outErr != nil {
		details = append(details, "check_out invalid date format")
	}
	if checkIn.After(checkOut) {
		details = append(details, "check in after check out ")
	}
	if checkIn.Equal(checkOut) {
		details = append(details, "check in and check out the same")
	}
	if len(details) > 0 {
		writeJSON(w, a.log, http.StatusBadRequest, ValidationError(details))
		return
	}

	rooms, err := a.service.FindAvailableRooms(ctx, uint(hotelId), checkIn, checkOut, guests)
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}
	hotel, err := a.service.GetHotel(r.Context(), uint(hotelId))
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}

	writeJSON(w, a.log, http.StatusOK, AvailabilityResponse{
		HotelID:   uint(hotelId),
		HotelName: hotel.Name,
		CheckIn:   q.Get("check_in"),
		CheckOut:  q.Get("check_out"),
		Nights:    helper.NightsBetween(checkIn, checkOut),
		Guests:    guests,
		Available: rooms,
		Count:     len(rooms),
	})
}

// ---------------------------------------------------------------------------
// Bookings
// ---------------------------------------------------------------------------

// POST /api/v1/bookings
func (a *API) handleCreateBooking(w http.ResponseWriter, r *http.Request) {
	var req CreateBookingRequest
	ctx := r.Context()

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		// return HTTP 400 bad request
		writeError(w, r, a.log, err)
	}

	var details []string

	if req.HotelID == 0 {
		details = append(details, "hotel_id is required")
	}
	if req.RoomID == 0 {
		details = append(details, "room_id is required")
	}
	if req.Guests < 1 {
		details = append(details, "guests must be at least 1")
	}

	checkIn, inErr := time.Parse(time.DateOnly, req.CheckIn)
	if inErr != nil {
		details = append(details, "check_in invalid date format")
	}
	checkOut, outErr := time.Parse(time.DateOnly, req.CheckOut)
	if outErr != nil {
		details = append(details, "check_out invalid date format")
	}
	if checkIn.After(checkOut) {
		details = append(details, "check in after check out ")
	}
	if checkIn.Equal(checkOut) {
		details = append(details, "check in and check out the same")
	}

	if len(details) > 0 {
		writeJSON(w, a.log, http.StatusBadRequest, ValidationError(details))
		return
	}

	var (
		booking *model.Booking
		lastErr error
	)
	for attempt := 0; attempt < maxReferenceAttempts; attempt++ {
		ref, err := a.refs.NewReference()
		if err != nil {
			lastErr = err
			break
		}

		candidate := &model.Booking{
			Reference:  ref,
			HotelID:    req.HotelID,
			RoomID:     req.RoomID,
			GuestName:  strings.TrimSpace(req.GuestName),
			GuestEmail: strings.TrimSpace(req.GuestEmail),
			Guests:     req.Guests,
			CheckIn:    checkIn,
			CheckOut:   checkOut,
			Status:     model.BookingStatusConfirmed,
		}

		err = a.service.CreateBooking(ctx, candidate)

		switch {
		case err == nil:
			booking = candidate
		case errors.Is(err, service.ErrReferenceConflict):
			a.log.WarnContext(ctx, "booking reference collided, retrying",
				"attempt", attempt+1, "reference", ref)
			continue
		default:
			lastErr = err
		}
		break
	}

	if booking == nil {
		if lastErr == nil {
			lastErr = fmt.Errorf("%w: could not allocate a booking reference", service.ErrReferenceConflict)
		}
		writeError(w, r, a.log, lastErr)
		return
	}

	a.log.InfoContext(ctx, "booking created",
		"reference", booking.Reference,
		"hotel_id", booking.HotelID,
		"room_id", booking.RoomID,
		"check_in", booking.CheckIn.Format(time.DateOnly),
		"check_out", booking.CheckOut.Format(time.DateOnly),
		"guests", booking.Guests,
	)

	a.notifier.Collector <- &service.EmailSender{Log: helper.Log, Booking: booking}

	writeJSON(w, a.log, http.StatusCreated, BookingResponse{
		Booking: *booking,
		Nights:  helper.NightsBetween(booking.CheckIn, booking.CheckOut),
	})
}

// GET /api/v1/bookings/{reference}
func (a *API) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	reference := r.PathValue("reference")
	if reference == "" {
		writeJSON(w, a.log, http.StatusBadRequest, ValidationError([]string{"reference is required"}))
		return
	}
	if len(reference) > 16 {
		writeError(w, r, a.log, ErrParamSize)
		return
	}

	booking, err := a.service.GetBookingByReference(r.Context(), reference)
	if err != nil {
		writeError(w, r, a.log, err)
		return
	}

	writeJSON(w, a.log, http.StatusOK, BookingResponse{
		Booking: *booking,
		Nights:  helper.NightsBetween(booking.CheckIn, booking.CheckOut),
	})
}

// ---------------------------------------------------------------------------

// handleHealth reports whether the service can serve traffic.
func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status: "ok",
		Driver: string(a.driver),
		Time:   time.Now().UTC().Format(time.RFC3339),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if a.ping != nil {
		if err := a.ping(ctx); err != nil {
			resp.Status = "unreachable"
			a.log.ErrorContext(ctx, "health check failed", "error", err)
			writeJSON(w, a.log, http.StatusServiceUnavailable, resp)
			return
		}
	}
	writeJSON(w, a.log, http.StatusOK, resp)
}

// handleSeed populates the database with test data.
func (a *API) handleSeed(w http.ResponseWriter, r *http.Request) {
	if helper.Cfg.Env != "development" {
		return
	}
	if err := a.service.Seed(r.Context()); err != nil {
		writeError(w, r, a.log, err)
		return
	}
	writeJSON(w, a.log, http.StatusOK, MessageResponse{
		Message: "database seeded with 1 hotel, 3 room types and 6 rooms",
	})
}

// handleReset removes all data.
func (a *API) handleReset(w http.ResponseWriter, r *http.Request) {
	if helper.Cfg.Env != "development" {
		return
	}
	if err := a.service.Reset(r.Context()); err != nil {
		writeError(w, r, a.log, err)
		return
	}
	writeJSON(w, a.log, http.StatusOK, MessageResponse{
		Message: "all data removed",
	})

}

// handleNotFound returns a JSON 404 for unmatched routes.
func (a *API) handleNotFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
