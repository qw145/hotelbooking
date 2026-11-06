package model

import "time"

type Hotel struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:200;not null;index" json:"name"`
	Address   string    `gorm:"size:300" json:"address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Rooms []Room `gorm:"foreignKey:HotelID" json:"rooms,omitempty"`
}
