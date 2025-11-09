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

	log.Println("Database migrations completed successfully")
}
