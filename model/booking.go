package model

import "time"

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
)

type Booking struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// Reference is the customer-facing booking number. It is UNIQUE and is
	// generated from crypto/rand rather than a sequence, so references are not
	// guessable and do not leak booking volume.
	Reference string `gorm:"size:16;not null;uniqueIndex" json:"reference"`

	HotelID uint `gorm:"not null;index" json:"hotel_id"`
	RoomID  uint `gorm:"not null;index" json:"room_id"`

	GuestName  string `gorm:"size:200;not null" json:"guest_name"`
	GuestEmail string `gorm:"size:200;not null" json:"guest_email"`
	Guests     int    `gorm:"not null" json:"guests"`

	// CheckIn is inclusive; CheckOut is exclusive. A stay of
	// [2026-03-01, 2026-03-03) occupies the nights of Mar 1 and Mar 2.
	CheckIn  time.Time `gorm:"not null;index" json:"check_in"`
	CheckOut time.Time `gorm:"not null;index" json:"check_out"`

	Status    BookingStatus `gorm:"size:20;not null;default:confirmed;index" json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`

	Hotel *Hotel `gorm:"foreignKey:HotelID" json:"hotel,omitempty"`
	Room  *Room  `gorm:"foreignKey:RoomID" json:"room,omitempty"`
}
