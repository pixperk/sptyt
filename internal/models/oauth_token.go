package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/pixperk/sptyt/internal/crypto"
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

	// Google account information
	AccountEmail   string `bun:"" json:"account_email,omitempty"`
	AccountName    string `bun:"" json:"account_name,omitempty"`
	AccountPicture string `bun:"" json:"account_picture,omitempty"`

	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}

// IsExpired checks if the access token has expired
func (t *UserOAuthToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// DecryptTokens decrypts the access and refresh tokens in-place.
// Safe to call on tokens that were stored before encryption was enabled.
func (t *UserOAuthToken) DecryptTokens() error {
	access, err := crypto.Decrypt(t.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := crypto.Decrypt(t.RefreshToken)
	if err != nil {
		return err
	}
	t.AccessToken = access
	t.RefreshToken = refresh
	return nil
}
