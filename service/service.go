// Package service implements the application's use cases, sitting between the
// HTTP transport and the repository.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/model"
	domain "github.com/qw145/hotelbooking/model"
	"gorm.io/gorm"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation failed")
	ErrRoomUnavailable   = errors.New("room is not available for the requested dates")
	ErrNoAvailability    = errors.New("no room available for the requested dates and party size")
	ErrCapacityExceeded  = errors.New("party size exceeds room capacity")
	ErrReferenceConflict = errors.New("booking reference already exists")
)

type Service interface {
	FindHotelsByName(ctx context.Context, name string) ([]model.Hotel, error)
	GetHotel(ctx context.Context, hotelID uint) (*domain.Hotel, error)
	ListRooms(ctx context.Context, hotelID uint) (*model.Hotel, error)
	FindAvailableRooms(ctx context.Context, hotelID uint, CheckIn, CheckOut time.Time, guestsNo int) ([]model.Room, error)
	CreateBooking(ctx context.Context, booking *domain.Booking) error
	GetBookingByReference(ctx context.Context, reference string) (*domain.Booking, error)
	Seed(ctx context.Context) error
	Reset(ctx context.Context) error
}

// HotelService holds the application use cases.
type HotelService struct {
	driver helper.Driver
	db     *gorm.DB
	log    *slog.Logger
}

func NewService() (*HotelService, error) {
	switch helper.Cfg.Driver {
	case helper.DriverSQLite:
	default:
		return nil, fmt.Errorf("%w: unsupported DB driver %q", helper.ErrUnsupportedDriver, helper.Cfg.Driver)
	}
	return &HotelService{
		driver: helper.DriverSQLite,
		db:     helper.Db,
		log:    helper.Log,
	}, nil
}

func (s *HotelService) FindHotelsByName(
	ctx context.Context, name string,
) ([]model.Hotel, error) {
	var hotels []model.Hotel

	err := s.db.WithContext(ctx).
		Where("LOWER(name) LIKE ?", "%"+strings.ToLower(strings.TrimSpace(name))+"%").
		Order("name ASC").
		Find(&hotels).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: hotel %s", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("get booking: %w", err)
	}

	return hotels, nil
}

func (s *HotelService) GetHotel(
	ctx context.Context, hotelID uint,
) (*domain.Hotel, error) {
	var hotel *model.Hotel

	err := s.db.WithContext(ctx).
		Where("id = ?", hotelID).
		First(&hotel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: hotel %d", ErrNotFound, hotelID)
	}
	if err != nil {
		return nil, fmt.Errorf("get hotel: %w", err)
	}
	return hotel, nil
}

func (s *HotelService) ListRooms(
	ctx context.Context, hotelID uint,
) (*model.Hotel, error) {
	var hotel *model.Hotel

	err := s.db.WithContext(ctx).
		Preload("Rooms", func(db *gorm.DB) *gorm.DB {
			return db.Order("rooms.number ASC")
		}).
		Where("id = ?", hotelID).
		Find(&hotel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: hotel %d", ErrNotFound, hotelID)
	}
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	return hotel, nil
}

func (s *HotelService) FindAvailableRooms(
	ctx context.Context, hotelID uint, CheckIn, CheckOut time.Time, guestsNo int,
) ([]model.Room, error) {
	if guestsNo < 1 {
		return nil, fmt.Errorf("%w: guests must be at least 1", ErrValidation)
	}
	var rooms []domain.Room
	sub := s.db.WithContext(ctx).
		Model(&domain.Booking{}).
		Select("1").
		Where("bookings.room_id = rooms.id").
		Where("bookings.status = ?", model.BookingStatusConfirmed).
		Where("bookings.check_in < ? AND bookings.check_out > ?", CheckOut, CheckIn)

	err := s.db.WithContext(ctx).
		Where("hotel_id = ?", hotelID).
		Where("capacity >= ?", guestsNo).
		Where("NOT EXISTS (?)", sub).
		Order("capacity ASC, number ASC").
		Find(&rooms).Error
	if err != nil {
		return nil, fmt.Errorf("find available rooms: %w", err)
	}

	return rooms, nil
}

func (s *HotelService) CreateBooking(ctx context.Context, booking *domain.Booking) error {
	err := s.db.WithContext(ctx).Transaction(func(intTx *gorm.DB) error {
		var room model.Room
		if err := intTx.
			First(&room, booking.RoomID).
			Error; err != nil {
			return err
		}
		if booking.Guests > room.Capacity {
			return fmt.Errorf("%w: room %s holds %d, requested %d",
				ErrCapacityExceeded, room.Number, room.Capacity, booking.Guests)
		}

		var conflicts int64
		if err := intTx.
			Model(&domain.Booking{}).
			Where("room_id = ?", booking.RoomID).
			Where("status = ?", model.BookingStatusConfirmed).
			Where("check_in < ? AND check_out > ?", booking.CheckOut, booking.CheckIn).
			Count(&conflicts).Error; err != nil {
			return err
		}
		if conflicts > 0 {
			return fmt.Errorf("%w: room %s is already booked for those dates",
				ErrRoomUnavailable, room.Number)
		}

		if err := intTx.Create(booking).Error; err != nil {
			if isDuplicateKeyError(err) {
				return fmt.Errorf("%w: %s", ErrReferenceConflict, booking.Reference)
			}
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.log.InfoContext(ctx, fmt.Sprintf("booking  created %v", booking))
	return nil
}

func (s *HotelService) GetBookingByReference(
	ctx context.Context, reference string,
) (*domain.Booking, error) {
	var booking model.Booking

	err := s.db.WithContext(ctx).
		Joins("Hotel").
		Joins("Room").
		Where("reference = ?", strings.ToUpper(strings.TrimSpace(reference))).
		First(&booking).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: booking %s", ErrNotFound, reference)
	}
	if err != nil {
		return nil, fmt.Errorf("get booking: %w", err)
	}
	return &booking, nil
}

func (s *HotelService) Seed(ctx context.Context) error {
	hotels := []domain.Hotel{
		{ID: 1,
			Name:    "Grand Plaza Hotel",
			Address: "1 Riverside Walk, London"},
		{ID: 2,
			Name:    "Another hotel Hotel",
			Address: "2 Riverside Walk, London"},
	}

	// Six rooms
	rooms := []domain.Room{
		{ID: 1, HotelID: hotels[0].ID, Number: "101", RoomType: model.RoomTypeSingle, Capacity: 1},
		{ID: 2, HotelID: hotels[0].ID, Number: "102", RoomType: model.RoomTypeSingle, Capacity: 1},
		{ID: 3, HotelID: hotels[0].ID, Number: "103", RoomType: model.RoomTypeDouble, Capacity: 2},
		{ID: 4, HotelID: hotels[0].ID, Number: "104", RoomType: model.RoomTypeDouble, Capacity: 2},
		{ID: 5, HotelID: hotels[0].ID, Number: "105", RoomType: model.RoomTypeDeluxe, Capacity: 3},
		{ID: 6, HotelID: hotels[0].ID, Number: "106", RoomType: model.RoomTypeDouble, Capacity: 2},
		{ID: 7, HotelID: hotels[1].ID, Number: "101", RoomType: model.RoomTypeDouble, Capacity: 2},
	}

	err := s.db.WithContext(ctx).Transaction(func(intTx *gorm.DB) error {
		res := intTx.CreateInBatches(&hotels, 20)
		if res.Error != nil {
			return res.Error
		}
		res = intTx.CreateInBatches(&rooms, 20)
		if res.Error != nil {
			return res.Error
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.log.InfoContext(ctx, "database seeded")
	return nil
}

func (s *HotelService) Reset(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec(`
		delete from  bookings;
		delete from  rooms;
		delete from  hotels;
	`).Error
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique index")
}
