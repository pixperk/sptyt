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

	if cachedURL, err := h.cache.GetYouTubeMVURL(ctx, trackID); err == nil {
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

	youtubeURL, err := h.youtubeClient.SearchOfficialMV(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find youtube video"})
	}

	h.cache.SetYouTubeMVURL(ctx, trackID, youtubeURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}

func (h *Handler) LyricVideoRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")

	trackID, err := utils.ExtractSpotifyTrackID(spotifyLink)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetYouTubeLyricsURL(ctx, trackID); err == nil {
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find youtube lyric video"})
	}

	h.cache.SetYouTubeLyricsURL(ctx, trackID, youtubeURL, 1*time.Hour)

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

func (h *Handler) YouTubeToSpotifyRedirect(c echo.Context) error {
	youtubeLink := c.Param("youtube_link")

	videoID, err := utils.ExtractYouTubeVideoID(youtubeLink)
	if err != nil || videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid youtube link"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetClient().Get(ctx, "yt2sp:"+videoID).Result(); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	title, description, apiISRC, err := h.youtubeClient.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch video metadata"})
	}

	var spotifyURL string
	var isrc string

	if apiISRC != "" {
		isrc = apiISRC
	} else {
		isrc = utils.ExtractISRCFromDescription(description)
	}

	if isrc != "" {
		spotifyURL, err = h.spotifyClient.SearchByISRC(ctx, isrc)
		if err == nil {
			h.cache.GetClient().Set(ctx, "yt2sp:"+videoID, spotifyURL, 1*time.Hour)
			c.Response().Header().Set("Cache-Control", "public, max-age=3600")
			c.Response().Header().Set("X-Cache", "MISS")
			c.Response().Header().Set("X-Match-Method", "ISRC")
			return c.Redirect(http.StatusMovedPermanently, spotifyURL)
		}
	}

	parsed := utils.ParseYouTubeTitle(title)
	query := utils.BuildSpotifyQuery(parsed)

	if query == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not parse track info from youtube title"})
	}

	spotifyURL, err = h.searchSpotifyByTitle(ctx, query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find track on spotify", "query": query})
	}

	h.cache.GetClient().Set(ctx, "yt2sp:"+videoID, spotifyURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	c.Response().Header().Set("X-Match-Method", "title-parse")
	return c.Redirect(http.StatusMovedPermanently, spotifyURL)
}

func (h *Handler) searchSpotifyByTitle(ctx context.Context, query string) (string, error) {
	return h.spotifyClient.SearchByTitle(ctx, query)
}

func (h *Handler) SmartRedirect(c echo.Context) error {
	link := c.Param("link")

	linkType := utils.DetectLinkType(link)

	switch linkType {
	case utils.LinkTypeYouTube:
		return h.handleYouTubeLink(c, link)
	case utils.LinkTypeSpotify:
		return h.handleSpotifyLink(c, link)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid link format"})
	}
}

func (h *Handler) handleYouTubeLink(c echo.Context, link string) error {
	videoID, err := utils.ExtractYouTubeVideoID(link)
	if err != nil || videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid youtube link"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetClient().Get(ctx, "yt2sp:"+videoID).Result(); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	title, description, apiISRC, err := h.youtubeClient.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch video metadata"})
	}

	var spotifyURL string
	var isrc string

	if apiISRC != "" {
		isrc = apiISRC
	} else {
		isrc = utils.ExtractISRCFromDescription(description)
	}

	if isrc != "" {
		spotifyURL, err = h.spotifyClient.SearchByISRC(ctx, isrc)
		if err == nil {
			h.cache.GetClient().Set(ctx, "yt2sp:"+videoID, spotifyURL, 1*time.Hour)
			c.Response().Header().Set("Cache-Control", "public, max-age=3600")
			c.Response().Header().Set("X-Cache", "MISS")
			c.Response().Header().Set("X-Match-Method", "ISRC")
			return c.Redirect(http.StatusMovedPermanently, spotifyURL)
		}
	}

	parsed := utils.ParseYouTubeTitle(title)
	query := utils.BuildSpotifyQuery(parsed)

	if query == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not parse track info from youtube title"})
	}

	spotifyURL, err = h.searchSpotifyByTitle(ctx, query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find track on spotify", "query": query})
	}

	h.cache.GetClient().Set(ctx, "yt2sp:"+videoID, spotifyURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	c.Response().Header().Set("X-Match-Method", "title-parse")
	return c.Redirect(http.StatusMovedPermanently, spotifyURL)
}

func (h *Handler) handleSpotifyLink(c echo.Context, link string) error {
	trackID, err := utils.ExtractSpotifyTrackID(link)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetYouTubeMVURL(ctx, trackID); err == nil {
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

	youtubeURL, err := h.youtubeClient.SearchOfficialMV(ctx, track.Name, artist)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find youtube video"})
	}

	h.cache.SetYouTubeMVURL(ctx, trackID, youtubeURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}
