package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

type Config struct {
	DB              *bun.DB
	Server          ServerConfig
	ClerkSecretKey  string
}

type ServerConfig struct {
	Port string
	Host string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func NewConfig() *Config {
	dbConfig := getDatabaseConfig()
	db := initDatabase(dbConfig)

	clerkSecretKey := getEnv("CLERK_SECRET_KEY", "")
	if clerkSecretKey == "" {
		log.Println("Warning: CLERK_SECRET_KEY not set - authentication disabled")
	}

	return &Config{
		DB: db,
		Server: ServerConfig{
			Port: getEnv("PORT", "8080"),
			Host: getEnv("SERVER_HOST", "localhost"),
		},
		ClerkSecretKey: clerkSecretKey,
	}
}

func getDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		DBName:   getEnv("DB_NAME", "sptyt"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
}

func initDatabase(config DatabaseConfig) *bun.DB {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.User,
		config.Password,
		config.Host,
		config.Port,
		config.DBName,
		config.SSLMode,
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	// Configure connection pool for production scalability
	// MaxOpenConns: Maximum number of open connections to the database
	// - Set to 100 to handle high concurrent load while preventing database overload
	// - PostgreSQL default max_connections is 100, so we stay within safe limits
	sqldb.SetMaxOpenConns(100)

	// MaxIdleConns: Maximum number of idle connections in the pool
	// - Set to 25 to keep connections ready for burst traffic
	// - Reduces latency by avoiding connection establishment overhead
	sqldb.SetMaxIdleConns(25)

	// ConnMaxLifetime: Maximum time a connection can be reused
	// - Set to 5 minutes to prevent stale connections
	// - Helps with load balancer connection draining and database maintenance
	sqldb.SetConnMaxLifetime(5 * time.Minute)

	// ConnMaxIdleTime: Maximum time a connection can be idle before closing
	// - Set to 2 minutes to free up resources during low traffic
	// - Balances resource usage with connection reuse
	sqldb.SetConnMaxIdleTime(2 * time.Minute)

	db := bun.NewDB(sqldb, pgdialect.New())

	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Printf("Database connection pool configured: MaxOpen=100, MaxIdle=25, ConnMaxLifetime=5m")
	log.Println("Database connection established successfully")

	return db
}

func (c *Config) Close() error {
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
