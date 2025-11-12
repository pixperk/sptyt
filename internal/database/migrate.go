package database

import (
	"context"
	"log"

	"github.com/pixperk/sptyt/internal/models"
	"github.com/uptrace/bun"
)

func RunMigrations(db *bun.DB) {
	ctx := context.Background()

	// Enable UUID extension for PostgreSQL
	_, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto")
	if err != nil {
		log.Fatalf("Failed to enable pgcrypto extension: %v", err)
	}

	// Create users table if not exists
	_, err = db.NewCreateTable().
		Model((*models.User)(nil)).
		IfNotExists().
		Exec(ctx)

	if err != nil {
		log.Fatalf("Failed to create users table: %v", err)
	}

	// Create indexes for performance optimization
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_users_clerk_id ON users(clerk_id);
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
		CREATE INDEX IF NOT EXISTS idx_users_subscription_tier ON users(subscription_tier);
		CREATE INDEX IF NOT EXISTS idx_users_subscription_status ON users(subscription_status);
	`)
	if err != nil {
		log.Fatalf("Failed to create indexes: %v", err)
	}

	// Drop username column if it exists (cleanup migration)
	_, err = db.Exec("ALTER TABLE users DROP COLUMN IF EXISTS username")
	if err != nil {
		log.Printf("Warning: Failed to drop username column: %v", err)
	} else {
		log.Println("Dropped username column from users table")
	}

	// Create oauth_tokens table
	_, err = db.NewCreateTable().
		Model((*models.UserOAuthToken)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to create oauth_tokens table: %v", err)
	}

	// Create playlist_conversions table
	_, err = db.NewCreateTable().
		Model((*models.PlaylistConversion)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to create playlist_conversions table: %v", err)
	}

	// Create indexes for oauth_tokens
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_oauth_tokens_user_id ON oauth_tokens(user_id);
		CREATE INDEX IF NOT EXISTS idx_oauth_tokens_provider ON oauth_tokens(provider);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_tokens_user_provider ON oauth_tokens(user_id, provider);
	`)
	if err != nil {
		log.Fatalf("Failed to create oauth_tokens indexes: %v", err)
	}

	// Create indexes for playlist_conversions
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_playlist_conversions_user_id ON playlist_conversions(user_id);
		CREATE INDEX IF NOT EXISTS idx_playlist_conversions_status ON playlist_conversions(status);
		CREATE INDEX IF NOT EXISTS idx_playlist_conversions_created_at ON playlist_conversions(created_at DESC);
	`)
	if err != nil {
		log.Fatalf("Failed to create playlist_conversions indexes: %v", err)
	}

	// Create user_analytics table
	_, err = db.NewCreateTable().
		Model((*models.UserAnalytics)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to create user_analytics table: %v", err)
	}

	// Create indexes for user_analytics
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_user_analytics_user_id ON user_analytics(user_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_analytics_user_unique ON user_analytics(user_id);
	`)
	if err != nil {
		log.Fatalf("Failed to create user_analytics indexes: %v", err)
	}

	// Add spotify_cover_image column (if it doesn't exist)
	_, err = db.Exec(`
		ALTER TABLE playlist_conversions
		ADD COLUMN IF NOT EXISTS spotify_cover_image TEXT;
	`)
	if err != nil {
		log.Printf("Warning: Failed to add spotify_cover_image column: %v", err)
	} else {
		log.Println("Added spotify_cover_image column to playlist_conversions table")
	}

	// Drop youtube_cover_image column if it exists (cleanup)
	_, err = db.Exec("ALTER TABLE playlist_conversions DROP COLUMN IF EXISTS youtube_cover_image")
	if err != nil {
		log.Printf("Warning: Failed to drop youtube_cover_image column: %v", err)
	} else {
		log.Println("Dropped youtube_cover_image column from playlist_conversions table")
	}

	// Add new columns to user_analytics table (if they don't exist)
	_, err = db.Exec(`
		ALTER TABLE user_analytics
		ADD COLUMN IF NOT EXISTS monthly_conversions INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS current_month INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS current_year INTEGER DEFAULT 0;
	`)
	if err != nil {
		log.Printf("Warning: Failed to add monthly tracking columns: %v", err)
	} else {
		log.Println("Added monthly tracking columns to user_analytics table")
	}

	// Add Google account info columns to oauth_tokens table (if they don't exist)
	_, err = db.Exec(`
		ALTER TABLE oauth_tokens
		ADD COLUMN IF NOT EXISTS account_email TEXT,
		ADD COLUMN IF NOT EXISTS account_name TEXT,
		ADD COLUMN IF NOT EXISTS account_picture TEXT;
	`)
	if err != nil {
		log.Printf("Warning: Failed to add account info columns: %v", err)
	} else {
		log.Println("Added account info columns to oauth_tokens table")
	}

	// Add counts_against_quota column to playlist_conversions table
	_, err = db.Exec(`
		ALTER TABLE playlist_conversions
		ADD COLUMN IF NOT EXISTS counts_against_quota BOOLEAN DEFAULT true;
	`)
	if err != nil {
		log.Printf("Warning: Failed to add counts_against_quota column: %v", err)
	} else {
		log.Println("Added counts_against_quota column to playlist_conversions table")
	}

	log.Println("Database migrations completed successfully")
}
