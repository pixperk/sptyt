package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/genius"
	"github.com/pixperk/sptyt/internal/spotify"
	"github.com/pixperk/sptyt/internal/youtube"
	"github.com/pixperk/sptyt/pkg/utils"
)

type Handler struct {
	spotifyClient *spotify.Client
	youtubeClient *youtube.Client
	geniusClient  *genius.Client
}

func NewHandler(spotifyClient *spotify.Client, youtubeClient *youtube.Client, geniusClient *genius.Client) *Handler {
	return &Handler{
		spotifyClient: spotifyClient,
		youtubeClient: youtubeClient,
		geniusClient:  geniusClient,
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

	track, err := h.spotifyClient.GetTrack(ctx, trackID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch track info"})
	}

	artist := strings.Join(track.Artists, " ")

	youtubeURL, err := h.youtubeClient.SearchLyricVideo(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find youtube video"})
	}

	return c.Redirect(http.StatusFound, youtubeURL)
}

func (h *Handler) GeniusRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")

	trackID, err := utils.ExtractSpotifyTrackID(spotifyLink)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	track, err := h.spotifyClient.GetTrack(ctx, trackID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch track info"})
	}

	artist := strings.Join(track.Artists, " ")

	geniusURL, err := h.geniusClient.SearchLyrics(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find lyrics on genius"})
	}

	return c.Redirect(http.StatusFound, geniusURL)
}
