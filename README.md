# sptyt - Spotify ↔ YouTube Smart Redirect Service

A high-performance Go service that intelligently redirects between Spotify, YouTube, and Genius using the Echo framework with Redis caching and rate limiting.

## Features

- Bidirectional Spotify ↔ YouTube redirects
- Official music videos, lyric videos, and Genius lyrics support
- ISRC-based matching for accuracy
- Smart link type detection
- Redis caching for fast redirects
- Per-IP rate limiting
- HTTP connection pooling
- Browser cache optimization

## Quick Start

### Prerequisites

- Go 1.24.0+
- Redis (or Upstash Redis)
- Spotify API credentials
- YouTube Data API key
- Genius API access token

### Setup

1. Copy `.env.example` to `.env` and fill in your API credentials:

```bash
cp .env.example .env
```

2. Install dependencies:

```bash
go mod tidy
```

3. Run the service:

```bash
go run main.go
```

The service will start on `http://localhost:8080` (or the PORT specified in `.env`).

## API Endpoints

### 1. Smart Redirect: `/:link`

Auto-detects link type and redirects to official music video.

**Supported inputs:**
- Spotify track URLs: `https://open.spotify.com/track/{id}`
- Spotify track IDs: `1RDLbX9L2G4zDqxXCaMJo5`
- YouTube URLs: `https://www.youtube.com/watch?v={id}`
- YouTube short URLs: `https://youtu.be/{id}`
- YouTube shorts: `https://www.youtube.com/shorts/{id}`
- YouTube embed: `https://www.youtube.com/embed/{id}`

**Examples:**

```bash
# Spotify → YouTube official MV
curl -L http://localhost:8080/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5

# YouTube → Spotify
curl -L http://localhost:8080/https://www.youtube.com/watch?v=dQw4w9WgXcQ

# Spotify ID only
curl -L http://localhost:8080/1RDLbX9L2G4zDqxXCaMJo5
```

### 2. Lyric Video: `/ly/:link`

Redirects to YouTube lyric video. Works with both Spotify and YouTube inputs.

**Examples:**

```bash
# Spotify → YouTube lyric video
curl -L http://localhost:8080/ly/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5

# YouTube → YouTube lyric video (finds lyric version)
curl -L http://localhost:8080/ly/https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

### 3. Genius Lyrics: `/gn/:link`

Redirects to Genius lyrics page. Works with both Spotify and YouTube inputs.

**Examples:**

```bash
# Spotify → Genius
curl -L http://localhost:8080/gn/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5

# YouTube → Genius
curl -L http://localhost:8080/gn/https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

### 4. YouTube to Spotify: `/yt/:youtube_link`

Explicit YouTube → Spotify conversion.

**Example:**

```bash
curl -L http://localhost:8080/yt/https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

## Accuracy Disclaimers

### YouTube → Spotify Matching

**NOT 100% ACCURATE.** The service uses multiple strategies with varying accuracy:

1. **ISRC from YouTube API** (Most Accurate) - Only available for official music uploads
2. **ISRC from Description** (High Accuracy) - Depends on uploader including ISRC
3. **Title Parsing** (Moderate Accuracy) - May fail with non-standard title formats

**Known limitations:**
- Covers, remixes, and live versions may not match correctly
- Fan uploads with incorrect metadata will produce wrong results
- Non-music videos return unpredictable matches

### Lyric Videos

**MAY NOT EXIST** for all songs. The service searches YouTube for lyric videos, but:

- New releases may not have lyric videos yet
- Older or less popular songs may never get lyric videos
- Search may return official videos if no lyric version exists

### Genius Lyrics

**MAY NOT BE AVAILABLE** for all tracks:

- Genius relies on community submissions
- Some artists/labels may not have lyrics on Genius
- Song titles must match closely for successful search

## Response Headers

The service provides diagnostic headers:

```
X-Cache: HIT|MISS                    # Whether result was cached
X-Match-Method: api-isrc|desc-isrc|title  # YouTube→Spotify match strategy
X-RateLimit-Limit: 60                # Requests allowed per minute
X-RateLimit-Remaining: 59            # Requests remaining
X-RateLimit-Reset: 1609459200        # Unix timestamp when limit resets
Cache-Control: public, max-age=3600  # Browser cache for 1 hour
```

## Caching Strategy

Two-layer caching for optimal performance:

1. **Track Metadata**: 24 hours TTL
   - Spotify track info
   - YouTube video metadata

2. **Final URLs**: 1 hour TTL
   - YouTube video URLs
   - Genius lyrics URLs
   - YouTube → Spotify mappings

## Rate Limiting

Default: 60 requests per minute per IP address.

Configure via `RATE_LIMIT_PER_MINUTE` environment variable.

Exceeding the limit returns `429 Too Many Requests`.

## Environment Variables

```bash
PORT=8080                                    # Service port
SPOTIFY_CLIENT_ID=your_spotify_client_id    # Spotify API credentials
SPOTIFY_CLIENT_SECRET=your_spotify_secret
YOUTUBE_API_KEY=your_youtube_api_key        # YouTube Data API v3 key
GENIUS_ACCESS_TOKEN=your_genius_token       # Genius API token
REDIS_URL=redis://localhost:6379            # Redis connection (supports rediss:// for TLS)
RATE_LIMIT_PER_MINUTE=60                    # Rate limit per IP
```

## Error Responses

All errors return JSON:

```json
{
  "error": "error message description"
}
```

Common errors:
- `400 Bad Request`: Invalid link format
- `404 Not Found`: Track/video not found
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: API failure or timeout

## Browser Usage

Simply paste the URL in your browser's address bar:

```
http://localhost:8080/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5
```

The browser will follow the redirect automatically.

## Performance

- **Redis caching**: Sub-10ms response for cached results
- **HTTP connection pooling**: 100 max idle connections
- **Spotify token refresh**: Background refresh every 55 minutes
- **API timeouts**: 10 seconds max per request
- **Browser caching**: 1 hour Cache-Control headers

## API Credentials

### Spotify

1. Go to https://developer.spotify.com/dashboard
2. Create an app
3. Copy Client ID and Client Secret

### YouTube

1. Go to https://console.cloud.google.com
2. Enable YouTube Data API v3
3. Create API key

### Genius

1. Go to https://genius.com/api-clients
2. Create API client
3. Generate access token

## License

MIT
