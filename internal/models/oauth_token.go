package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type UserOAuthToken struct {
	bun.BaseModel `bun:"table:oauth_tokens,alias:oat"`

	ID           uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID `bun:"type:uuid,notnull" json:"user_id"`
	Provider     string    `bun:",notnull" json:"provider"` // "youtube"
	AccessToken  string    `bun:",notnull" json:"-"`        // Not exposed in JSON
	RefreshToken string    `bun:",notnull" json:"-"`        // Not exposed in JSON
	ExpiresAt    time.Time `bun:",notnull" json:"expires_at"`
	Scope        string    `bun:"" json:"scope"`

	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}

// IsExpired checks if the access token has expired
func (t *UserOAuthToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}
