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
	redisAddr := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDBStr := os.Getenv("REDIS_DB")

	if spotifyClientID == "" || spotifyClientSecret == "" || youtubeAPIKey == "" || geniusAccessToken == "" {
		log.Fatal("Missing required environment variables")
	}

	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisDB := 0
	if redisDBStr != "" {
		if db, err := strconv.Atoi(redisDBStr); err == nil {
			redisDB = db
		}
	}

	redisCache, err := cache.NewRedisCache(redisAddr, redisPassword, redisDB)
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer redisCache.Close()

	spotifyClient := spotify.NewClient(spotifyClientID, spotifyClientSecret)
	youtubeClient := youtube.NewClient(youtubeAPIKey)
	geniusClient := genius.NewClient(geniusAccessToken)
	handler := handlers.NewHandler(spotifyClient, youtubeClient, geniusClient, redisCache)

	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/ly/:spotify_link", handler.GeniusRedirect)
	e.GET("/:spotify_link", handler.SpotifyRedirect)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	e.Logger.Fatal(e.Start(":" + port))
}
