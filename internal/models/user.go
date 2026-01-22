package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID              uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	ClerkID         string    `bun:",unique,notnull" json:"clerk_id"`
	Email           string    `bun:",unique,notnull" json:"email"`
	FirstName       string    `bun:"" json:"first_name"`
	LastName        string    `bun:"" json:"last_name"`
	ProfileImageURL string    `bun:"" json:"profile_image_url"`

	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
