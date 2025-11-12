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
	"github.com/pixperk/sptyt/internal/services"
	"github.com/pixperk/sptyt/internal/spotify"
	"github.com/pixperk/sptyt/internal/tasks"
	ws "github.com/pixperk/sptyt/internal/websocket"
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
	asynqRedisAddr := os.Getenv("ASYNQ_REDIS_ADDR")
	asynqRedisPassword := os.Getenv("ASYNQ_REDIS_PASSWORD")
	rateLimitStr := os.Getenv("RATE_LIMIT_PER_MINUTE")

	if spotifyClientID == "" || spotifyClientSecret == "" || youtubeAPIKey == "" || geniusAccessToken == "" {
		log.Fatal("Missing required environment variables")
	}

	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	if asynqRedisAddr == "" {
		asynqRedisAddr = "localhost:6379"
	}

	rateLimit := 200 // Increased from 60 to 200 requests per minute
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

	// Initialize WebSocket hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// Initialize Asynq task client
	taskClient := tasks.NewClient(asynqRedisAddr, asynqRedisPassword)
	defer taskClient.Close()

	// Initialize playlist conversion service with WebSocket hub and task client
	converterService := services.NewPlaylistConverterService(cfg.DB, spotifyClient, youtubeClient, wsHub, taskClient)

	// Start Asynq task server in background
	taskServer := tasks.NewServer(asynqRedisAddr, asynqRedisPassword, converterService, 10) // 10 concurrent workers
	go func() {
		if err := taskServer.Start(); err != nil {
			log.Fatalf("Asynq server failed: %v", err)
		}
	}()
	defer taskServer.Shutdown()

	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://sptyt.xyz", "http://localhost:3000"},
		AllowCredentials: true,
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowMethods:     []string{echo.GET, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
	}))
	e.Use(rateLimiter.Middleware())
	e.Use(custommw.MobileAppRedirect())

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

	// Custom link public routes (no auth required)
	customLinkService := services.NewCustomLinkService(cfg.DB)
	customLinkHandler := handlers.NewCustomLinkHandler(customLinkService, cfg.DB)

	// Public custom link routes
	e.GET("/l/:slug", customLinkHandler.ProxyToFrontend)                                      // Proxy to frontend for rendering
	e.GET("/api/links/:slug", customLinkHandler.GetLinkBySlugPublic)                          // API to get link data
	e.POST("/api/links/:slug/verify", customLinkHandler.VerifyLinkPassword)                   // Verify password for protected link
	e.GET("/api/track/:link_id/:element_id", customLinkHandler.TrackElementClick)             // Track element click and redirect

	// Protected API routes (require Clerk authentication)
	if cfg.ClerkSecretKey != "" {
		clerkMiddleware := auth.NewClerkMiddleware(cfg.ClerkSecretKey)
		protectedHandler := handlers.NewProtectedHandler(handler, cfg.DB)
		youtubeOAuthHandler := handlers.NewYouTubeOAuthHandler(cfg.DB, redisCache)
		playlistHandler := handlers.NewPlaylistHandler(cfg.DB, converterService, taskClient)
		playlistLimiter := custommw.NewPlaylistLimiter(cfg.DB, redisCache)
		wsHandler := handlers.NewWebSocketHandler(wsHub)
		analyticsHandler := handlers.NewAnalyticsHandler(cfg.DB)

		// YouTube OAuth callback (NO AUTH REQUIRED - uses state token)
		e.GET("/api/auth/youtube/callback", youtubeOAuthHandler.Callback)

		// WebSocket endpoint for real-time progress (handles auth via token query param)
		e.GET("/api/ws/playlist-progress", wsHandler.HandleConnection)

		// Create protected API group
		api := e.Group("/api")
		api.Use(clerkMiddleware.RequireAuth())

		// User endpoints
		api.GET("/me", protectedHandler.Me)
		api.POST("/checkout", protectedHandler.CreateCheckoutSession)
		api.POST("/subscription/cancel", protectedHandler.CancelSubscription)
		api.GET("/payment/return", protectedHandler.PaymentReturn)

		// YouTube OAuth endpoints (protected - except callback above)
		api.GET("/auth/youtube/authorize", youtubeOAuthHandler.Authorize)
		api.GET("/auth/youtube/reconnect", youtubeOAuthHandler.ReconnectYouTube)
		api.GET("/auth/youtube/status", youtubeOAuthHandler.GetYouTubeAuthStatus)
		api.DELETE("/auth/youtube/disconnect", youtubeOAuthHandler.DisconnectYouTube)

		// Playlist conversion endpoints (with rate limiting)
		api.GET("/playlists/limits", playlistLimiter.GetUserLimitsInfo)
		api.POST("/playlists/convert", playlistHandler.ConvertPlaylist, playlistLimiter.CheckPlaylistConversionLimits())
		api.GET("/playlists/conversions", playlistHandler.GetUserConversions)
		api.GET("/playlists/conversions/detailed", playlistHandler.GetDetailedUserConversions)
		api.GET("/playlists/conversions/:id", playlistHandler.GetConversionStatus)

		// Analytics endpoints
		api.GET("/analytics", analyticsHandler.GetUserAnalytics)
		api.GET("/analytics/monthly", analyticsHandler.GetMonthlyStats)
		api.GET("/dashboard", analyticsHandler.GetUserDashboard)

		// Custom link management endpoints (protected - require authentication)
		api.POST("/links", customLinkHandler.CreateCustomLink)                               // Create custom link
		api.GET("/links", customLinkHandler.GetUserLinks)                                    // Get user's custom links
		api.GET("/links/:id", customLinkHandler.GetCustomLink)                               // Get specific link by ID
		api.PUT("/links/:id", customLinkHandler.UpdateCustomLink)                            // Update custom link
		api.DELETE("/links/:id", customLinkHandler.DeleteCustomLink)                         // Delete custom link
		api.POST("/links/:id/elements", customLinkHandler.AddElement)                        // Add element to link
		api.PUT("/links/:id/elements/reorder", customLinkHandler.ReorderElements)            // Reorder elements
		api.DELETE("/links/:id/elements/:element_id", customLinkHandler.DeleteElement)       // Delete element
		api.GET("/links/:id/analytics", customLinkHandler.GetLinkAnalytics)                  // Get link analytics

		log.Println("Clerk authentication enabled - /api/me route available")
		log.Println("WebSocket server running - /api/ws/playlist-progress available")
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
