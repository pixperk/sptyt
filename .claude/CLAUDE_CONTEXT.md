# SPTYT - Spotify to YouTube Lyric Video Redirector

## Project Overview
A simple Go web service using Echo framework that takes Spotify song links and redirects users to the corresponding YouTube lyric video.

## Architecture
- **Framework**: Echo v4
- **Language**: Go 1.23.6
- **Port**: 8080 (configurable via PORT env var)

## Endpoint
- `GET /:spotify_link` - Accepts Spotify track link or code, fetches track info from Spotify API, searches YouTube for lyric video, redirects to YouTube

## Flow
1. User hits `http://localhost:8080/{spotify_link_or_code}`
2. Parse Spotify link/code to extract track ID
3. Use Spotify API to get track name and artist
4. Use YouTube Data API to search for "{track_name} {artist} lyrics"
5. Redirect to first YouTube result

## Environment Variables
- `PORT` - Server port (default: 8080)
- `SPOTIFY_CLIENT_ID` - Spotify API client ID
- `SPOTIFY_CLIENT_SECRET` - Spotify API client secret
- `YOUTUBE_API_KEY` - YouTube Data API v3 key

## File Structure
```
.
├── main.go              # Main application and handler
├── go.mod               # Go module dependencies
├── .env.example         # Example environment variables
├── .gitignore          # Git ignore rules
└── CLAUDE_CONTEXT.md   # This file
```

## Next Steps
1. Create Spotify API client to authenticate and fetch track details
2. Create YouTube API client to search for lyric videos
3. Implement link parsing logic for Spotify URLs
4. Implement redirect logic in handler
5. Add error handling and validation

## API Setup Required
- **Spotify**: Create app at https://developer.spotify.com/dashboard
- **YouTube**: Enable YouTube Data API v3 at https://console.cloud.google.com/
