package service

import (
	"context"
	"testing"
	"time"

	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/model"
	"github.com/subosito/gotenv"
)

func TestSeed(t *testing.T) {
	gotenv.Load("../.env")
	helper.Cfg = helper.NewConfig()
	helper.Cfg.DSN = "../booking.db"
	ctx := context.Background()
	helper.Log = helper.NewLogger(nil)

	helper.OpenDb(ctx)

	CheckIn, _ := time.Parse(time.DateOnly, "2026-03-01")
	CheckOut, _ := time.Parse(time.DateOnly, "2026-03-03")
	booking := model.Booking{
		Reference:  "asdasd",
		HotelID:    1,
		Guests:     2,
		GuestName:  "asdasdasd",
		GuestEmail: "asdasd@asdasd.com",
		RoomID:     3,
		CheckIn:    CheckIn,
		CheckOut:   CheckOut,
		Status:     model.BookingStatusConfirmed,
	}

	s := HotelService{db: helper.Db, log: helper.Log}
	// err := s.Reset(ctx)
	// s.Seed(ctx)
	//r, _ := s.GetBookingByReference(ctx, "2")
	//rs, _ := s.FindAvailableRooms(ctx, 1, CheckIn, CheckOut, 4)
	//s.ListRooms(ctx, 1)

	err := s.CreateBooking(ctx, &booking)
	if err != nil {
		t.Fatalf("booking not created :: %v", err)
	}
}
