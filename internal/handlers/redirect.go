package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/genius"
	"github.com/pixperk/sptyt/internal/spotify"
	"github.com/pixperk/sptyt/internal/youtube"
	"github.com/pixperk/sptyt/pkg/utils"
)

type Handler struct {
	spotifyClient *spotify.Client
	youtubeClient *youtube.Client
	geniusClient  *genius.Client
	cache         *cache.RedisCache
}

func NewHandler(spotifyClient *spotify.Client, youtubeClient *youtube.Client, geniusClient *genius.Client, redisCache *cache.RedisCache) *Handler {
	return &Handler{
		spotifyClient: spotifyClient,
		youtubeClient: youtubeClient,
		geniusClient:  geniusClient,
		cache:         redisCache,
	}
}

func (h *Handler) SpotifyRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")

	trackID, err := utils.ExtractSpotifyTrackID(spotifyLink)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetYouTubeURL(ctx, trackID); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	var track *spotify.Track
	if cachedTrack, err := h.cache.GetTrack(ctx, trackID); err == nil {
		track = &spotify.Track{
			Name:    cachedTrack.Name,
			Artists: cachedTrack.Artists,
		}
	} else {
		track, err = h.spotifyClient.GetTrack(ctx, trackID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch track info"})
		}
		h.cache.SetTrack(ctx, trackID, &cache.CachedTrack{
			Name:    track.Name,
			Artists: track.Artists,
		}, 24*time.Hour)
	}

	artist := strings.Join(track.Artists, " ")

	youtubeURL, err := h.youtubeClient.SearchLyricVideo(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find youtube video"})
	}

	h.cache.SetYouTubeURL(ctx, trackID, youtubeURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}

func (h *Handler) GeniusRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")

	trackID, err := utils.ExtractSpotifyTrackID(spotifyLink)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetGeniusURL(ctx, trackID); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	var track *spotify.Track
	if cachedTrack, err := h.cache.GetTrack(ctx, trackID); err == nil {
		track = &spotify.Track{
			Name:    cachedTrack.Name,
			Artists: cachedTrack.Artists,
		}
	} else {
		track, err = h.spotifyClient.GetTrack(ctx, trackID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch track info"})
		}
		h.cache.SetTrack(ctx, trackID, &cache.CachedTrack{
			Name:    track.Name,
			Artists: track.Artists,
		}, 24*time.Hour)
	}

	artist := strings.Join(track.Artists, " ")

	geniusURL, err := h.geniusClient.SearchLyrics(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find lyrics on genius"})
	}

	h.cache.SetGeniusURL(ctx, trackID, geniusURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	return c.Redirect(http.StatusMovedPermanently, geniusURL)
}
