package utils

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"strings"
)

// GenerateSlug creates a URL-safe slug from a title
func GenerateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace spaces and special characters with hyphens
	reg := regexp.MustCompile("[^a-z0-9]+")
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length to 50 characters
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// GenerateRandomSlug creates a random URL-safe slug
func GenerateRandomSlug(length int) string {
	if length <= 0 {
		length = 8
	}

	// Generate random bytes
	b := make([]byte, length)
	rand.Read(b)

	// Encode to base64 URL-safe
	slug := base64.URLEncoding.EncodeToString(b)

	// Remove padding and ensure correct length
	slug = strings.TrimRight(slug, "=")
	if len(slug) > length {
		slug = slug[:length]
	}

	return strings.ToLower(slug)
}

// AppendRandomSuffix adds a random suffix to handle slug collisions
func AppendRandomSuffix(slug string) string {
	suffix := GenerateRandomSlug(4)
	return slug + "-" + suffix
}
