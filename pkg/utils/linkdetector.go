package utils

import (
	"regexp"
	"strings"
)

var (
	youtubeRegex = regexp.MustCompile(`(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})`)
	spotifyRegex = regexp.MustCompile(`(?:https?://)?(?:open\.)?spotify\.com/track/([a-zA-Z0-9]+)`)
)

type LinkType int

const (
	LinkTypeUnknown LinkType = iota
	LinkTypeSpotify
	LinkTypeYouTube
)

func DetectLinkType(input string) LinkType {
	input = strings.TrimSpace(input)

	if youtubeRegex.MatchString(input) {
		return LinkTypeYouTube
	}

	if spotifyRegex.MatchString(input) || len(input) == 22 && regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(input) {
		return LinkTypeSpotify
	}

	return LinkTypeUnknown
}

func ExtractYouTubeVideoID(input string) (string, error) {
	input = strings.TrimSpace(input)
	matches := youtubeRegex.FindStringSubmatch(input)
	if len(matches) >= 2 {
		return matches[1], nil
	}
	return "", nil
}
