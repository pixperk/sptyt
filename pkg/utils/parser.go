package utils

import (
	"fmt"
	"regexp"
	"strings"
)

var spotifyTrackRegex = regexp.MustCompile(`^(?:https?://)?(?:open\.)?spotify\.com/track/([a-zA-Z0-9]+)`)

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
