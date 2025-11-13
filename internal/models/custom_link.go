package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CustomLink represents a shareable custom link with Bento-style grid elements
type CustomLink struct {
	bun.BaseModel `bun:"table:custom_links,alias:cl"`

	ID                  uuid.UUID  `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	UserID              uuid.UUID  `bun:"type:uuid,notnull" json:"user_id"`
	Slug                string     `bun:",unique,notnull" json:"slug"`                          // URL-safe identifier
	Title               string     `bun:",notnull" json:"title"`
	Description         string     `bun:"" json:"description,omitempty"`
	BackgroundColor     string     `bun:",default:'#ffffff'" json:"background_color"`           // Page background color
	Theme               string     `bun:",notnull,default:'auto'" json:"theme"`                 // light, dark, auto
	IsPasswordProtected bool       `bun:",notnull,default:false" json:"is_password_protected"`
	PasswordHash        string     `bun:"" json:"-"`                                            // Never expose in JSON
	ConversionID        *uuid.UUID `bun:"type:uuid" json:"conversion_id,omitempty"`             // Optional link to conversion
	ExpiresAt           *time.Time `bun:",nullzero" json:"expires_at,omitempty"`                // NULL = never expires
	ViewCount           int        `bun:",notnull,default:0" json:"view_count"`
	IsPublic            bool       `bun:",notnull,default:true" json:"is_public"`
	CreatedAt           time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt           time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	User     *User          `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
	Elements []LinkElement  `bun:"rel:has-many,join:id=custom_link_id" json:"elements,omitempty"`
}

// IsExpired checks if the custom link has expired
func (cl *CustomLink) IsExpired() bool {
	if cl.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*cl.ExpiresAt)
}

// LinkElement represents an individual element in a custom link
type LinkElement struct {
	bun.BaseModel `bun:"table:link_elements,alias:le"`

	ID           uuid.UUID   `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	CustomLinkID uuid.UUID   `bun:"type:uuid,notnull" json:"custom_link_id"`
	ElementType  string      `bun:",notnull" json:"element_type"`             // song, playlist, custom_text
	ElementData  ElementData `bun:"type:jsonb,notnull" json:"element_data"`
	DisplayIndex int         `bun:",notnull,default:0" json:"display_index"`  // Order in layout
	IsVisible    bool        `bun:",notnull,default:true" json:"is_visible"`
	ClickCount   int         `bun:",notnull,default:0" json:"click_count"`
	CreatedAt    time.Time   `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt    time.Time   `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	CustomLink *CustomLink `bun:"rel:belongs-to,join:custom_link_id=id" json:"-"`
}

// ElementData contains the flexible data for different element types (Bento-style)
type ElementData struct {
	// Bento Grid Styling (applies to all element types)
	GridColumn     string `json:"grid_column,omitempty"`      // CSS grid column (e.g., "span 2")
	GridRow        string `json:"grid_row,omitempty"`         // CSS grid row (e.g., "span 2")
	BackgroundColor string `json:"background_color,omitempty"` // Element background (#hex or gradient)
	BorderRadius   string `json:"border_radius,omitempty"`    // Border radius (e.g., "12px")
	BorderColor    string `json:"border_color,omitempty"`     // Border color
	TextColor      string `json:"text_color,omitempty"`       // Text color
	FontSize       string `json:"font_size,omitempty"`        // Font size (e.g., "16px", "1rem")
	Padding        string `json:"padding,omitempty"`          // Padding (e.g., "20px")

	// Common fields for song and playlist
	Title       string `json:"title,omitempty"`        // Track name or playlist name
	Artists     string `json:"artists,omitempty"`      // For songs
	CoverImage  string `json:"cover_image,omitempty"`  // Spotify image URL
	Duration    string `json:"duration,omitempty"`     // "3:45" for songs
	TrackCount  int    `json:"track_count,omitempty"`  // For playlists

	// Platform links (for song type - user can add any or all)
	SpotifyURL       string `json:"spotify_url,omitempty"`
	YouTubeURL       string `json:"youtube_url,omitempty"`
	YouTubeLyricURL  string `json:"youtube_lyric_url,omitempty"`  // YouTube lyric video
	GeniusURL        string `json:"genius_url,omitempty"`

	// Playlist-specific fields
	ConversionID       *uuid.UUID `json:"conversion_id,omitempty"`        // References playlist_conversions table
	PlaylistSpotifyURL string     `json:"playlist_spotify_url,omitempty"`
	PlaylistYouTubeURL string     `json:"playlist_youtube_url,omitempty"`

	// Custom text/html element
	CustomText  string `json:"custom_text,omitempty"`
	CustomHTML  string `json:"custom_html,omitempty"`

	// Link element (simple link with icon)
	LinkURL  string `json:"link_url,omitempty"`   // For simple link elements
	LinkIcon string `json:"link_icon,omitempty"`  // Icon name or URL

	// Image element
	ImageURL string `json:"image_url,omitempty"` // For standalone image elements
	ImageAlt string `json:"image_alt,omitempty"`
}

// LinkAnalytics tracks events on custom links
type LinkAnalytics struct {
	bun.BaseModel `bun:"table:link_analytics,alias:la"`

	ID            uuid.UUID  `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	CustomLinkID  uuid.UUID  `bun:"type:uuid,notnull" json:"custom_link_id"`
	LinkElementID *uuid.UUID `bun:"type:uuid" json:"link_element_id,omitempty"` // NULL for page views
	EventType     string     `bun:",notnull" json:"event_type"`                 // page_view, element_click, link_share
	IPAddress     string     `bun:"" json:"ip_address,omitempty"`
	UserAgent     string     `bun:"" json:"user_agent,omitempty"`
	Referrer      string     `bun:"" json:"referrer,omitempty"`
	Country       string     `bun:"" json:"country,omitempty"` // Optional geo data
	CreatedAt     time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`

	// Relations
	CustomLink  *CustomLink  `bun:"rel:belongs-to,join:custom_link_id=id" json:"-"`
	LinkElement *LinkElement `bun:"rel:belongs-to,join:link_element_id=id" json:"-"`
}
