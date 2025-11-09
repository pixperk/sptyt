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

	// Subscription fields
	SubscriptionTier   string     `bun:",notnull,default:'free'" json:"subscription_tier"`       // free, premium
	SubscriptionStatus string     `bun:",notnull,default:'inactive'" json:"subscription_status"` // active, inactive, cancelled
	SubscriptionID     string     `bun:"" json:"subscription_id,omitempty"`                      // DodoPay subscription ID
	SubscriptionEndsAt *time.Time `bun:",nullzero" json:"subscription_ends_at,omitempty"`

	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// IsPremium checks if user has active premium subscription
func (u *User) IsPremium() bool {
	if u.SubscriptionTier != "premium" || u.SubscriptionStatus != "active" {
		return false
	}

	if u.SubscriptionEndsAt != nil && u.SubscriptionEndsAt.Before(time.Now()) {
		return false
	}

	return true
}
