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

func (h *Handler) Home(c echo.Context) error {
	return c.File("web/templates/index.html")
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
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			youtubeAppURL := h.convertToYouTubeAppURL(cachedURL)
			if youtubeAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
			}
		}
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
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		youtubeAppURL := h.convertToYouTubeAppURL(youtubeURL)
		if youtubeAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
		}
	}
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}

func (h *Handler) LyricVideoRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")
	return h.spotifyToLyricVideo(c, spotifyLink)
}

func (h *Handler) spotifyToLyricVideo(c echo.Context, spotifyLink string) error {
	trackID, err := utils.ExtractSpotifyTrackID(spotifyLink)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid spotify link or ID"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetYouTubeLyricsURL(ctx, trackID); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			youtubeAppURL := h.convertToYouTubeAppURL(cachedURL)
			if youtubeAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
			}
		}
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
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		youtubeAppURL := h.convertToYouTubeAppURL(youtubeURL)
		if youtubeAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
		}
	}
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}

func (h *Handler) GeniusRedirect(c echo.Context) error {
	spotifyLink := c.Param("spotify_link")
	return h.spotifyToGenius(c, spotifyLink)
}

func (h *Handler) spotifyToGenius(c echo.Context, spotifyLink string) error {
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
	// Get everything after /yt/ including query params
	youtubeLink := strings.TrimPrefix(c.Request().URL.Path, "/yt/")
	if c.Request().URL.RawQuery != "" {
		youtubeLink = youtubeLink + "?" + c.Request().URL.RawQuery
	}

	videoID, err := utils.ExtractYouTubeVideoID(youtubeLink)
	if err != nil || videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid youtube link"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetClient().Get(ctx, "yt2sp:"+videoID).Result(); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			spotifyAppURL := h.convertToSpotifyAppURL(cachedURL)
			if spotifyAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
			}
		}
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
			// Check if mobile and convert to app URL
			if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
				spotifyAppURL := h.convertToSpotifyAppURL(spotifyURL)
				if spotifyAppURL != "" {
					return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
				}
			}
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
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		spotifyAppURL := h.convertToSpotifyAppURL(spotifyURL)
		if spotifyAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
		}
	}
	return c.Redirect(http.StatusMovedPermanently, spotifyURL)
}

func (h *Handler) searchSpotifyByTitle(ctx context.Context, query string) (string, error) {
	return h.spotifyClient.SearchByTitle(ctx, query)
}

func (h *Handler) SmartRedirect(c echo.Context) error {
	// Get everything after the first / including query params
	link := strings.TrimPrefix(c.Request().URL.Path, "/")
	if c.Request().URL.RawQuery != "" {
		link = link + "?" + c.Request().URL.RawQuery
	}

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
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			spotifyAppURL := h.convertToSpotifyAppURL(cachedURL)
			if spotifyAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
			}
		}
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
			// Check if mobile and convert to app URL
			if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
				spotifyAppURL := h.convertToSpotifyAppURL(spotifyURL)
				if spotifyAppURL != "" {
					return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
				}
			}
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
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		spotifyAppURL := h.convertToSpotifyAppURL(spotifyURL)
		if spotifyAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, spotifyAppURL)
		}
	}
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
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			youtubeAppURL := h.convertToYouTubeAppURL(cachedURL)
			if youtubeAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
			}
		}
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
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		youtubeAppURL := h.convertToYouTubeAppURL(youtubeURL)
		if youtubeAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
		}
	}
	return c.Redirect(http.StatusMovedPermanently, youtubeURL)
}

func (h *Handler) SmartLyricVideoRedirect(c echo.Context) error {
	// Get everything after /ly/ including query params
	link := strings.TrimPrefix(c.Request().URL.Path, "/ly/")
	if c.Request().URL.RawQuery != "" {
		link = link + "?" + c.Request().URL.RawQuery
	}
	linkType := utils.DetectLinkType(link)

	switch linkType {
	case utils.LinkTypeYouTube:
		return h.youtubeToLyricVideo(c, link)
	case utils.LinkTypeSpotify:
		return h.spotifyToLyricVideo(c, link)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid link format"})
	}
}

func (h *Handler) SmartGeniusRedirect(c echo.Context) error {
	// Get everything after /gn/ including query params
	link := strings.TrimPrefix(c.Request().URL.Path, "/gn/")
	if c.Request().URL.RawQuery != "" {
		link = link + "?" + c.Request().URL.RawQuery
	}
	linkType := utils.DetectLinkType(link)

	switch linkType {
	case utils.LinkTypeYouTube:
		return h.youtubeToGenius(c, link)
	case utils.LinkTypeSpotify:
		return h.spotifyToGenius(c, link)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid link format"})
	}
}

func (h *Handler) youtubeToLyricVideo(c echo.Context, link string) error {
	videoID, err := utils.ExtractYouTubeVideoID(link)
	if err != nil || videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid youtube link"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetClient().Get(ctx, "yt2ly:"+videoID).Result(); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		// Check if mobile and convert to app URL
		if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
			youtubeAppURL := h.convertToYouTubeAppURL(cachedURL)
			if youtubeAppURL != "" {
				return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
			}
		}
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	title, description, apiISRC, err := h.youtubeClient.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch video metadata"})
	}

	var trackName, artistName string

	if apiISRC != "" {
		track, err := h.spotifyClient.GetTrackByISRC(ctx, apiISRC)
		if err == nil {
			trackName = track.Name
			artistName = strings.Join(track.Artists, " ")
		}
	}

	if trackName == "" {
		isrc := utils.ExtractISRCFromDescription(description)
		if isrc != "" {
			track, err := h.spotifyClient.GetTrackByISRC(ctx, isrc)
			if err == nil {
				trackName = track.Name
				artistName = strings.Join(track.Artists, " ")
			}
		}
	}

	if trackName == "" {
		parsed := utils.ParseYouTubeTitle(title)
		trackName = parsed.Track
		artistName = parsed.Artist
	}

	if trackName == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not parse track info"})
	}

	lyricVideoURL, err := h.youtubeClient.SearchLyricVideo(ctx, trackName, artistName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find lyric video"})
	}

	h.cache.GetClient().Set(ctx, "yt2ly:"+videoID, lyricVideoURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	// Check if mobile and convert to app URL
	if isMobile, ok := c.Get("is_mobile").(bool); ok && isMobile {
		youtubeAppURL := h.convertToYouTubeAppURL(lyricVideoURL)
		if youtubeAppURL != "" {
			return c.Redirect(http.StatusMovedPermanently, youtubeAppURL)
		}
	}
	return c.Redirect(http.StatusMovedPermanently, lyricVideoURL)
}

func (h *Handler) youtubeToGenius(c echo.Context, link string) error {
	videoID, err := utils.ExtractYouTubeVideoID(link)
	if err != nil || videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid youtube link"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if cachedURL, err := h.cache.GetClient().Get(ctx, "yt2gn:"+videoID).Result(); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		c.Response().Header().Set("X-Cache", "HIT")
		return c.Redirect(http.StatusMovedPermanently, cachedURL)
	}

	title, description, apiISRC, err := h.youtubeClient.GetVideoMetadata(ctx, videoID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch video metadata"})
	}

	var trackName, artistName string

	if apiISRC != "" {
		track, err := h.spotifyClient.GetTrackByISRC(ctx, apiISRC)
		if err == nil {
			trackName = track.Name
			artistName = strings.Join(track.Artists, " ")
		}
	}

	if trackName == "" {
		isrc := utils.ExtractISRCFromDescription(description)
		if isrc != "" {
			track, err := h.spotifyClient.GetTrackByISRC(ctx, isrc)
			if err == nil {
				trackName = track.Name
				artistName = strings.Join(track.Artists, " ")
			}
		}
	}

	if trackName == "" {
		parsed := utils.ParseYouTubeTitle(title)
		trackName = parsed.Track
		artistName = parsed.Artist
	}

	if trackName == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not parse track info"})
	}

	geniusURL, err := h.geniusClient.SearchLyrics(ctx, trackName, artistName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find lyrics on genius"})
	}

	h.cache.GetClient().Set(ctx, "yt2gn:"+videoID, geniusURL, 1*time.Hour)

	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("X-Cache", "MISS")
	return c.Redirect(http.StatusMovedPermanently, geniusURL)
}

// convertToSpotifyAppURL converts a Spotify web URL to app URL scheme
func (h *Handler) convertToSpotifyAppURL(webURL string) string {
	// Extract track ID from Spotify URL
	trackID, err := utils.ExtractSpotifyTrackID(webURL)
	if err != nil || trackID == "" {
		return ""
	}
	return "spotify:track:" + trackID
}

// convertToYouTubeAppURL converts a YouTube web URL to app URL scheme
func (h *Handler) convertToYouTubeAppURL(webURL string) string {
	// Extract video ID from YouTube URL
	videoID, err := utils.ExtractYouTubeVideoID(webURL)
	if err != nil || videoID == "" {
		return ""
	}
	return "vnd.youtube://watch?v=" + videoID
}
