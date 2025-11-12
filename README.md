# SPTYT - Spotify to YouTube Playlist Converter

A production-ready SaaS application that converts Spotify playlists and albums to YouTube playlists with real-time progress tracking, subscription management, and intelligent track matching.

## Overview

SPTYT provides a seamless way to transfer your music collections between Spotify and YouTube. The platform uses multiple matching strategies to ensure the highest quality conversions, including ISRC matching, official music video detection, and lyric video fallbacks.

## Core Features

### Playlist Conversion
- **Multi-format Support**: Convert Spotify playlists, albums, and EPs to YouTube
- **Intelligent Track Matching**:
  - ISRC-based exact matching
  - Official music video detection
  - Lyric video fallback
  - Title-based search with artist matching
- **Real-time Progress**: WebSocket-based live updates during conversion
- **Background Processing**: Asynchronous job queue using Asynq (Redis-backed)
- **Worker Pool Architecture**: Concurrent processing with 5 workers for optimal performance
- **Cover Art Preservation**: Automatically captures and stores playlist/album artwork

### Smart Link Redirects
- **Universal Smart Links**: `sptyt.xyz/:link` - Auto-detects Spotify or YouTube links
- **Lyric Video Mode**: `sptyt.xyz/ly/:link` - Forces lyric video matches
- **Genius Integration**: `sptyt.xyz/gn/:link` - Redirects to Genius lyrics pages
- **Reverse Lookup**: `sptyt.xyz/yt/:link` - Find Spotify tracks from YouTube videos
- **Mobile App Deep Links**: Automatically converts to native app URLs on mobile devices
- **Redis Caching**: 24-hour cache for track metadata, 1-hour cache for YouTube URLs

### Subscription Management
- **Free Tier**: 1 playlist per month, 10 songs maximum
- **Premium Tier**: 20 playlists per month, 100 songs maximum
- **DodoPay Integration**: Secure payment processing with card and UPI support
- **Automatic Billing**: Handles subscription lifecycle events (renewals, cancellations, failures)
- **Webhook Security**: Signature verification using Standard Webhooks specification

### Analytics & Monitoring
- **User Analytics Dashboard**:
  - Total conversions and success rates
  - Monthly usage tracking with automatic resets
  - Track-level statistics (matched vs failed)
  - Match method distribution
- **Conversion History**:
  - Paginated detailed view with cover images
  - Separated successful and failed tracks
  - Match method tracking (ISRC, official MV, lyric video)
  - Full track metadata storage

### Authentication & Authorization
- **Clerk Integration**: Secure user authentication and session management
- **YouTube OAuth 2.0**:
  - Complete OAuth flow with state validation
  - Automatic token refresh (5-minute expiry detection)
  - Google account information retrieval (email, name, profile picture)
  - Disconnect and reconnect capabilities
- **Scope Management**: YouTube + OpenID + Profile + Email permissions

## Technical Architecture

### Backend Stack
- **Language**: Go 1.21+
- **Web Framework**: Echo v4
- **Database**: PostgreSQL with Bun ORM
- **Cache**: Redis for caching and job queue
- **Task Queue**: Asynq for background job processing
- **WebSockets**: Real-time progress updates via WebSocket Hub

### External Integrations
- **Spotify API**: Client credentials flow for playlist and track metadata
- **YouTube Data API v3**: Playlist creation and video search
- **Genius API**: Lyrics search and metadata
- **Google OAuth 2.0**: YouTube account authorization
- **DodoPay**: Payment processing and subscription management
- **Clerk**: Authentication and user management

### Infrastructure Features
- **Database Migrations**: Safe ALTER TABLE patterns with IF NOT EXISTS
- **Indexed Queries**: Unique indexes on user_id for fast lookups
- **Rate Limiting**: Per-IP and per-user rate limiting middleware
- **Mobile Detection**: Automatic mobile app redirect middleware
- **CORS**: Configured for cross-origin requests
- **Error Recovery**: Panic recovery middleware with logging

## Performance Optimizations

### Caching Strategy
- **Track Metadata**: 24-hour TTL in Redis
- **YouTube URLs**: 1-hour TTL in Redis
- **OAuth State Tokens**: 10-minute TTL in Redis
- **No Analytics Caching**: Direct indexed database queries (faster than cache invalidation)

### Database Optimization
- Unique index on `user_analytics.user_id` for O(1) lookups
- Composite indexes on `playlist_conversions` for efficient filtering
- Monthly counter stored in single row per user (no aggregation queries)

### Race Condition Prevention
- Immediate monthly counter increment on conversion start
- Prevents concurrent request abuse
- Counter incremented before background job processing

## API Documentation

### Public Endpoints
```
GET  /:link                    - Smart redirect (auto-detects Spotify/YouTube)
GET  /ly/:link                 - Force lyric video redirect
GET  /gn/:link                 - Redirect to Genius lyrics
GET  /yt/:youtube_link         - YouTube to Spotify reverse lookup
```

### Protected Endpoints (Authentication Required)
```
# User Management
GET  /api/me                                      - Current user info
POST /api/checkout                                - Create payment checkout session
POST /api/subscription/cancel                     - Cancel subscription
GET  /api/payment/return                          - Payment return handler

# YouTube OAuth
GET  /api/auth/youtube/authorize                  - Start YouTube OAuth flow
GET  /api/auth/youtube/status                     - YouTube connection status
GET  /api/auth/youtube/reconnect                  - Reconnect YouTube account
DELETE /api/auth/youtube/disconnect               - Disconnect YouTube account

# Playlist Conversion
POST /api/playlists/convert                       - Convert playlist/album
GET  /api/playlists/conversions                   - List user conversions (basic)
GET  /api/playlists/conversions/detailed          - Detailed conversions with pagination
GET  /api/playlists/conversions/:id               - Specific conversion status
GET  /api/playlists/limits                        - User subscription limits

# Analytics
GET  /api/analytics/monthly                       - Monthly usage statistics

# WebSocket
GET  /api/ws/playlist-progress                    - Real-time conversion progress
```

### Webhook Endpoints
```
POST /webhooks/dodopay         - DodoPay payment webhook
```

## Environment Configuration

```env
# Server
PORT=8080

# Database
DATABASE_URL=postgresql://user:password@localhost:5432/sptyt

# Redis
REDIS_HOST=localhost:6379
REDIS_PASSWORD=

# Spotify API
SPOTIFY_CLIENT_ID=your_spotify_client_id
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret

# YouTube API
YOUTUBE_API_KEY=your_youtube_api_key

# YouTube OAuth
YOUTUBE_OAUTH_CLIENT_ID=your_google_oauth_client_id
YOUTUBE_OAUTH_CLIENT_SECRET=your_google_oauth_client_secret
YOUTUBE_OAUTH_REDIRECT_URI=https://yourdomain.com/api/auth/youtube/callback

# Genius API
GENIUS_ACCESS_TOKEN=your_genius_access_token

# Clerk Authentication
CLERK_SECRET_KEY=your_clerk_secret_key

# DodoPay Payment
DODOPAY_API_KEY=your_dodopay_api_key
DODOPAY_API_HOST=https://test.dodopayments.com
DODOPAY_PRODUCT_ID=your_dodopay_product_id
DODOPAY_WEBHOOK_SECRET=your_dodopay_webhook_secret
DODOPAY_RETURN_URL=https://yourdomain.com/payment/return

# Frontend
FRONTEND_URL=https://yourdomain.com
```

## Installation & Setup

### Prerequisites
- Go 1.21 or higher
- PostgreSQL 14+
- Redis 6+

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/yourusername/sptyt.git
cd sptyt
```

2. Install dependencies:
```bash
go mod download
```

3. Set up environment variables:
```bash
cp .env.example .env
# Edit .env with your API keys and configuration
```

4. Run database migrations:
```bash
go run main.go
# Migrations run automatically on startup
```

5. Start the server:
```bash
go run main.go
```

The server will start on `http://localhost:8080`

## Database Schema

### Users Table
- User authentication and profile information
- Subscription tier and status tracking
- Timestamps for account lifecycle

### OAuth Tokens Table
- YouTube OAuth access and refresh tokens
- Google account information (email, name, picture)
- Token expiry tracking

### Playlist Conversions Table
- Conversion job metadata and status
- Spotify and YouTube playlist IDs and URLs
- Cover image URLs
- Track count and success/failure counts
- JSONB field for detailed track conversion logs

### User Analytics Table
- Total and successful conversion counts
- Monthly usage tracking with automatic resets
- Track-level statistics (processed, matched, failed)
- Current month/year for automatic reset logic

## Key Technical Decisions

### Why Asynq Instead of Cron Jobs?
- Reliable job queue with retries
- Horizontal scalability
- Job deduplication
- Real-time job status tracking
- Redis-backed persistence

### Why No Redis Caching for Monthly Limits?
- Database query with unique index is already fast (< 1ms)
- Caching adds complexity and stale data issues
- Eliminated race conditions from cache invalidation
- Simpler codebase with direct database queries

### Why Immediate Counter Increment?
- Prevents users from bypassing limits with concurrent requests
- Counter increments before async job processing
- Race-safe without distributed locks

### Why WebSockets for Progress Updates?
- Real-time user experience
- Lower latency than polling
- Reduced server load compared to frequent HTTP requests
- Native support in modern browsers

## Coming Soon

### Custom Link Generation
- User-generated shareable links for playlists
- Configurable content options:
  - Include/exclude official music videos
  - Include/exclude lyric videos
  - Include/exclude Genius lyrics links
  - Include/exclude Spotify links
- Beautiful public landing pages for shared playlists
- View count tracking and analytics
- Optional password protection for private shares
- Configurable link expiration (free tier: 7 days, premium: permanent)

### Enhanced Track Matching
- Album art-based matching using image recognition
- Audio fingerprinting for exact track matching
- User-defined fallback preferences
- Manual track remapping interface
- Spotify ISRC database expansion

### Playlist Management
- Edit converted playlists
- Add/remove individual tracks
- Bulk playlist operations
- Playlist merging and splitting
- Duplicate detection and removal

### Social Features
- Public profile pages
- Share conversion statistics
- Follow other users
- Collaborative playlists
- Playlist comments and ratings

### Advanced Analytics
- Conversion quality scoring
- Most popular matched tracks
- Failed track patterns analysis
- Match method effectiveness tracking
- API usage statistics dashboard

### Integration Expansions
- Apple Music support
- SoundCloud integration
- Deezer compatibility
- Tidal integration
- Bandcamp support

### Mobile Applications
- Native iOS app
- Native Android app
- Progressive Web App (PWA)
- Offline playlist queueing
- Push notifications for completed conversions

### API Platform
- Public API for developers
- Rate-limited API keys
- Webhook support for third-party integrations
- SDKs for popular languages (Python, JavaScript, Ruby)
- API documentation portal

### Enterprise Features
- Team accounts with shared conversion quotas
- Bulk conversion API
- White-label options
- Custom domain support
- Advanced permission management
- SSO integration (SAML, OIDC)

## Contributing

Contributions are welcome. Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License. See the LICENSE file for details.

## Support

For support, email support@sptyt.xyz or open an issue on GitHub.

## Acknowledgments

- Spotify Web API for music metadata
- YouTube Data API for playlist management
- Genius API for lyrics integration
- DodoPay for payment processing
- Clerk for authentication services
