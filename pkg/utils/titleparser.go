package utils

import (
	"regexp"
	"strings"
)

type ParsedTrack struct {
	Artist string
	Track  string
}

var (
	hyphenPattern     = regexp.MustCompile(`^(.+?)\s*[-–—]\s*(.+)$`)
	quotesPattern     = regexp.MustCompile(`"([^"]+)"`)
	parenthesesClean  = regexp.MustCompile(`\s*\([^)]*\)`)
	bracketsClean     = regexp.MustCompile(`\s*\[[^\]]*\]`)
	officialSuffixes  = regexp.MustCompile(`(?i)\s*(official|music|video|audio|lyric|lyrics|visualizer|mv|hd|4k|hq).*$`)
	featPattern       = regexp.MustCompile(`(?i)\s*(\(|\[)?\s*(ft\.?|feat\.?|featuring)[^\)]*(\)|\])?`)
)

func ParseYouTubeTitle(title string) *ParsedTrack {
	original := title

	title = strings.ReplaceAll(title, " - Topic", "")
	title = strings.TrimSpace(title)

	if match := hyphenPattern.FindStringSubmatch(title); len(match) == 3 {
		artist := strings.TrimSpace(match[1])
		track := strings.TrimSpace(match[2])

		track = cleanTrackName(track)
		artist = cleanArtistName(artist)

		if artist != "" && track != "" {
			return &ParsedTrack{Artist: artist, Track: track}
		}
	}

	if matches := quotesPattern.FindStringSubmatch(title); len(matches) > 1 {
		track := strings.TrimSpace(matches[1])
		artist := strings.ReplaceAll(title, matches[0], "")
		artist = cleanArtistName(artist)
		track = cleanTrackName(track)

		if artist != "" && track != "" {
			return &ParsedTrack{Artist: artist, Track: track}
		}
	}

	cleaned := cleanTrackName(original)
	parts := strings.Fields(cleaned)
	if len(parts) >= 2 {
		return &ParsedTrack{Artist: parts[0], Track: strings.Join(parts[1:], " ")}
	}

	return &ParsedTrack{Artist: "", Track: cleaned}
}

func cleanTrackName(track string) string {
	track = officialSuffixes.ReplaceAllString(track, "")
	track = parenthesesClean.ReplaceAllString(track, "")
	track = bracketsClean.ReplaceAllString(track, "")
	track = featPattern.ReplaceAllString(track, "")
	track = strings.TrimSpace(track)
	return track
}

func cleanArtistName(artist string) string {
	artist = strings.ReplaceAll(artist, " - Topic", "")
	artist = strings.ReplaceAll(artist, "VEVO", "")
	artist = strings.TrimSpace(artist)
	return artist
}

func BuildSpotifyQuery(parsed *ParsedTrack) string {
	if parsed.Artist != "" && parsed.Track != "" {
		return parsed.Artist + " " + parsed.Track
	}
	if parsed.Track != "" {
		return parsed.Track
	}
	return ""
}

func ExtractISRCFromDescription(description string) string {
	isrcPattern := regexp.MustCompile(`(?i)ISRC[:\s]+([A-Z]{2}[A-Z0-9]{3}\d{7})`)
	matches := isrcPattern.FindStringSubmatch(description)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}
