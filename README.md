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

## Key Technical Decisions

### Why Asynq Instead of Cron Jobs?
- Reliable job queue with retries and horizontal scalability
- Job deduplication and real-time status tracking
- Redis-backed persistence

### Why No Redis Caching for Monthly Limits?
- Database query with unique index is already fast (< 1ms)
- Eliminated race conditions from cache invalidation
- Simpler codebase with direct database queries

### Why Immediate Counter Increment?
- Prevents users from bypassing limits with concurrent requests
- Counter increments before async job processing
- Race-safe without distributed locks

## New: Custom Bento-Style Links ✨

### Bento-Style Link Builder
Create beautiful, customizable shareable links with a modular grid layout inspired by Bento.me:

**Features:**
- **Flexible Grid Layout**: Each element has custom size (1x1, 2x1, 2x2, etc.)
- **Rich Element Types**:
  - Song cards with Spotify, YouTube, YouTube Lyrics, and Genius links
  - Playlist cards from your converted playlists
  - Custom text/HTML blocks
  - Social media links with icons
  - Standalone images
- **Per-Element Styling**: Custom colors, gradients, borders, padding, and text styles
- **Page Customization**: Set background colors and theme (light/dark/auto)
- **Premium Features**:
  - Password protection with bcrypt encryption
  - Custom slugs (e.g., `sptyt.xyz/l/my-music`)
  - Unlimited links and elements
- **Analytics**: Track page views and element clicks
- **Free Tier**: 3 links, 10 elements per link, links expire after 7 days

