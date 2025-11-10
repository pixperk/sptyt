package utils

import (
	"fmt"
	"regexp"
	"strings"
)

var spotifyTrackRegex = regexp.MustCompile(`^(?:https?://)?(?:open\.)?spotify\.com/track/([a-zA-Z0-9]+)`)
var spotifyPlaylistRegex = regexp.MustCompile(`^(?:https?://)?(?:open\.)?spotify\.com/playlist/([a-zA-Z0-9]+)`)
var spotifyAlbumRegex = regexp.MustCompile(`^(?:https?://)?(?:open\.)?spotify\.com/album/([a-zA-Z0-9]+)`)

func ExtractSpotifyTrackID(input string) (string, error) {
	input = strings.TrimSpace(input)

	matches := spotifyTrackRegex.FindStringSubmatch(input)
	if len(matches) >= 2 {
		return matches[1], nil
	}

	if len(input) == 22 && regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(input) {
		return input, nil
	}

	return "", fmt.Errorf("invalid spotify track link or ID")
}

func ExtractSpotifyPlaylistID(input string) (string, string, error) {
	input = strings.TrimSpace(input)

	// Remove query parameters if present
	if idx := strings.Index(input, "?"); idx != -1 {
		input = input[:idx]
	}

	// Check for playlist
	if matches := spotifyPlaylistRegex.FindStringSubmatch(input); len(matches) >= 2 {
		return matches[1], "playlist", nil
	}

	// Check for album
	if matches := spotifyAlbumRegex.FindStringSubmatch(input); len(matches) >= 2 {
		return matches[1], "album", nil
	}

	// If it looks like an ID (22 alphanumeric chars), assume it's a playlist
	if len(input) == 22 && regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(input) {
		return input, "playlist", nil
	}

	return "", "", fmt.Errorf("invalid spotify playlist/album link or ID")
}
