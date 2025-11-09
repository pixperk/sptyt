package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/config"
	"github.com/pixperk/sptyt/internal/database"
	"github.com/pixperk/sptyt/internal/genius"
	"github.com/pixperk/sptyt/internal/handlers"
	custommw "github.com/pixperk/sptyt/internal/middleware"
	"github.com/pixperk/sptyt/internal/spotify"
	"github.com/pixperk/sptyt/internal/youtube"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize database configuration
	cfg := config.NewConfig()
	defer cfg.Close()

	// Run database migrations
	database.RunMigrations(cfg.DB)

	spotifyClientID := os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	youtubeAPIKey := os.Getenv("YOUTUBE_API_KEY")
	geniusAccessToken := os.Getenv("GENIUS_ACCESS_TOKEN")
	redisURL := os.Getenv("REDIS_URL")
	rateLimitStr := os.Getenv("RATE_LIMIT_PER_MINUTE")

	if spotifyClientID == "" || spotifyClientSecret == "" || youtubeAPIKey == "" || geniusAccessToken == "" {
		log.Fatal("Missing required environment variables")
	}

	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	rateLimit := 60
	if rateLimitStr != "" {
		if limit, err := strconv.Atoi(rateLimitStr); err == nil && limit > 0 {
			rateLimit = limit
		}
	}

	redisCache, err := cache.NewRedisCache(redisURL)
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer redisCache.Close()

	rateLimiter := custommw.NewRateLimiter(redisCache.GetClient(), rateLimit)

	spotifyClient := spotify.NewClient(spotifyClientID, spotifyClientSecret)
	youtubeClient := youtube.NewClient(youtubeAPIKey)
	geniusClient := genius.NewClient(geniusAccessToken)
	handler := handlers.NewHandler(spotifyClient, youtubeClient, geniusClient, redisCache)

	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(rateLimiter.Middleware())
	e.Use(custommw.MobileAppRedirect())
	e.Use(middleware.CORS())

	// Static files
	e.Static("/static", "web/static")

	// Home page
	e.GET("/", handler.Home)

	// Public API routes (free tier - no auth required)
	e.GET("/ly/:link", handler.SmartLyricVideoRedirect)
	e.GET("/gn/:link", handler.SmartGeniusRedirect)
	e.GET("/yt/:youtube_link", handler.YouTubeToSpotifyRedirect)
	e.GET("/:link", handler.SmartRedirect)

	// Webhook routes (no auth required - verified by signature)
	webhookHandler := handlers.NewWebhookHandler(cfg.DB)
	e.POST("/webhooks/dodopay", webhookHandler.DodoPayWebhook)

	// Protected API routes (require Clerk authentication)
	if cfg.ClerkSecretKey != "" {
		clerkMiddleware := auth.NewClerkMiddleware(cfg.ClerkSecretKey)
		protectedHandler := handlers.NewProtectedHandler(handler, cfg.DB)

		// Create protected API group
		api := e.Group("/api")
		api.Use(clerkMiddleware.RequireAuth())

		// User endpoints
		api.GET("/me", protectedHandler.Me)
		api.POST("/checkout", protectedHandler.CreateCheckoutSession)
		api.POST("/subscription/cancel", protectedHandler.CancelSubscription)

		log.Println("Clerk authentication enabled - /api/me route available")
	} else {
		log.Println("Clerk not configured - protected routes disabled")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	e.Logger.Fatal(e.Start(":" + port))
}
