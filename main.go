package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pixperk/sptyt/internal/cache"
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

	// Static files
	e.Static("/static", "web/static")

	// Home page
	e.GET("/", handler.Home)

	// API routes
	e.GET("/ly/:link", handler.SmartLyricVideoRedirect)
	e.GET("/gn/:link", handler.SmartGeniusRedirect)
	e.GET("/yt/:youtube_link", handler.YouTubeToSpotifyRedirect)
	e.GET("/:link", handler.SmartRedirect)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	e.Logger.Fatal(e.Start(":" + port))
}
