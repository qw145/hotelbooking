package handlers

import (
	"github.com/qw145/hotelbooking/model"
)

type MessageResponse struct {
	Message string `json:"message"`
}

type CreateBookingRequest struct {
	HotelID    uint   `json:"hotel_id"`
	RoomID     uint   `json:"room_id"`
	GuestName  string `json:"guest_name"`
	GuestEmail string `json:"guest_email"`
	Guests     int    `json:"guests"`
	CheckIn    string `json:"check_in"`
	CheckOut   string `json:"check_out"`
}

type BookingResponse struct {
	model.Booking
	Nights int `json:"nights"`
}

type AvailabilityResponse struct {
	HotelID   uint         `json:"hotel_id"`
	HotelName string       `gorm:"size:200;not null;index" json:"hotel_name"`
	CheckIn   string       `json:"check_in"`
	CheckOut  string       `json:"check_out"`
	Nights    int          `json:"nights"`
	Guests    int          `json:"guests"`
	Available []model.Room `json:"available_rooms"`
	Count     int          `json:"count"`
}
type HealthResponse struct {
	Status string `json:"status"`
	Driver string `json:"driver"`
	Time   string `json:"time"`
}
