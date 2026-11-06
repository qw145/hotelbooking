package model

type RoomType string

const (
	RoomTypeSingle RoomType = "single"
	RoomTypeDouble RoomType = "double"
	RoomTypeDeluxe RoomType = "deluxe"
)

type Room struct {
	ID                uint     `gorm:"primaryKey" json:"id"`
	HotelID           uint     `gorm:"not null;index;uniqueIndex:idx_hotel_room_number" json:"hotel_id"`
	Number            string   `gorm:"size:20;not null;uniqueIndex:idx_hotel_room_number" json:"number"`
	RoomType          RoomType `gorm:"TYPE:string;size:20;not null" json:"room_type"`
	Capacity          int      `gorm:"not null" json:"capacity"`
	BasePricePerNight float64  `gorm:"not null;default:0" json:"base_price_per_night"`

	///Hotel *Hotel `gorm:"foreignKey:HotelID" json:"-"`
	///Bookings []Booking `gorm:"foreignKey:RoomID" json:"-"`
}
