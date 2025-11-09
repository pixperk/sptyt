# sptyt - Convert Music Links Instantly

Switch between Spotify and YouTube in seconds. Just add your music link to our URL and get redirected to the platform you want.

## How to Use

### Convert Spotify to YouTube

Add your Spotify link after `sptyt.xyz/`:

`sptyt.xyz/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5`

Or just use the track ID:

`sptyt.xyz/1RDLbX9L2G4zDqxXCaMJo5`

### Convert YouTube to Spotify


Works the same way with YouTube links:

`sptyt.xyz/https://www.youtube.com/watch?v=dQw4w9WgXcQ`

Supports all YouTube URL formats:
- Full URLs: `youtube.com/watch?v=...`
- Short URLs: `youtu.be/...`
- Shorts: `youtube.com/shorts/...`

### Get Lyric Videos

Add `/ly/` before your link:

- From Spotify: `sptyt.xyz/ly/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5`
- From YouTube: `sptyt.xyz/ly/https://www.youtube.com/watch?v=dQw4w9WgXcQ`

### Get Genius Lyrics

Add `/gn/` before your link:

- From Spotify: `sptyt.xyz/gn/https://open.spotify.com/track/1RDLbX9L2G4zDqxXCaMJo5`
- From YouTube: `sptyt.xyz/gn/https://www.youtube.com/watch?v=dQw4w9WgXcQ`

## Important Notes

### Matching Accuracy

**YouTube → Spotify conversions are not 100% accurate.**

Works best with:
- Official music videos and uploads
- Videos with proper metadata

May not work correctly with:
- Covers, remixes, live versions
- Fan uploads with incorrect information
- Non-music videos

### Lyric Videos

Not all songs have lyric videos. If a lyric video doesn't exist, you might get redirected to the official music video instead.

### Genius Lyrics

Lyrics availability depends on community submissions. Some songs may not be available on Genius.

## Rate Limits

60 requests per minute per IP address. If you exceed this, wait a minute and try again.

---

## For Developers

### Technical Details

**Stack:**
- Go with Echo framework
- Redis caching
- Spotify, YouTube, and Genius APIs

**Matching Strategy:**
1. ISRC from YouTube API (most accurate)
2. ISRC from video description
3. Title parsing (fallback)

**Caching:**
- Track metadata: 24 hours
- Final URLs: 1 hour
- Browser cache: 1 hour

**Performance:**
- Cached results: <10ms
- HTTP connection pooling
- Background token refresh

### Setup

**Prerequisites:**
- Go 1.24.0+
- Redis
- API credentials (Spotify, YouTube, Genius)

**Installation:**

1. Clone and install dependencies:
```bash
go mod tidy
```

2. Copy `.env.example` to `.env` and add your credentials:
```bash
PORT=8080
SPOTIFY_CLIENT_ID=your_spotify_client_id
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret
YOUTUBE_API_KEY=your_youtube_api_key
GENIUS_ACCESS_TOKEN=your_genius_access_token
REDIS_URL=redis://localhost:6379
RATE_LIMIT_PER_MINUTE=60
```

3. Run:
```bash
go run main.go
```

### Get API Credentials

**Spotify:**
1. Visit https://developer.spotify.com/dashboard
2. Create an app
3. Copy Client ID and Secret

**YouTube:**
1. Visit https://console.cloud.google.com
2. Enable YouTube Data API v3
3. Create API key

**Genius:**
1. Visit https://genius.com/api-clients
2. Create API client
3. Generate access token

### API Endpoints

- `/:link` - Smart redirect to music video
- `/ly/:link` - Redirect to lyric video
- `/gn/:link` - Redirect to Genius lyrics
- `/yt/:youtube_link` - YouTube to Spotify

### Response Headers

```
X-Cache: HIT|MISS
X-Match-Method: api-isrc|desc-isrc|title
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 59
X-RateLimit-Reset: 1609459200
Cache-Control: public, max-age=3600
```

### Error Responses

JSON format:
```json
{
  "error": "error message"
}
```

Status codes:
- `400` - Invalid link format
- `404` - Track/video not found
- `429` - Rate limit exceeded
- `500` - API failure

## License

MIT
