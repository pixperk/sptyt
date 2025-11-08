# Frontend Integration Guide - Clerk Authentication with React

This guide explains how to integrate Clerk authentication with your React frontend for the sptyt application.

## Prerequisites

1. **Clerk Account Setup**
   - Sign up at [clerk.com](https://clerk.com)
   - Create a new application
   - Get your **Publishable Key** from the API Keys section

2. **Configure Clerk Dashboard**
   - Enable **Google OAuth** in Authentication → Social Connections
   - Enable **Email OTP** in Authentication → Email & Phone → Email verification strategy
   - Add allowed domains for your app (localhost:5173 for development)

## React + Vite Setup

### 1. Create React App

```bash
npm create vite@latest sptyt-app -- --template react-ts
cd sptyt-app
npm install
```

### 2. Install Clerk SDK

```bash
npm install @clerk/clerk-react
```

### 3. Environment Variables

Create `.env.local`:

```env
VITE_CLERK_PUBLISHABLE_KEY=pk_test_your_publishable_key_here
VITE_API_BASE_URL=http://localhost:8080
```

### 4. Wrap App with ClerkProvider

Update `src/main.tsx`:

```tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import { ClerkProvider } from '@clerk/clerk-react'
import App from './App.tsx'
import './index.css'

const PUBLISHABLE_KEY = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

if (!PUBLISHABLE_KEY) {
  throw new Error('Missing Publishable Key')
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ClerkProvider publishableKey={PUBLISHABLE_KEY}>
      <App />
    </ClerkProvider>
  </React.StrictMode>,
)
```

### 5. Create Authentication Components

#### `src/components/SignInButton.tsx`

```tsx
import { useClerk } from '@clerk/clerk-react'

export default function SignInButton() {
  const { openSignIn } = useClerk()

  return (
    <button onClick={() => openSignIn()}>
      Sign In
    </button>
  )
}
```

#### `src/components/SignUpButton.tsx`

```tsx
import { useClerk } from '@clerk/clerk-react'

export default function SignUpButton() {
  const { openSignUp } = useClerk()

  return (
    <button onClick={() => openSignUp()}>
      Sign Up
    </button>
  )
}
```

#### `src/components/UserButton.tsx`

```tsx
import { UserButton as ClerkUserButton } from '@clerk/clerk-react'

export default function UserButton() {
  return <ClerkUserButton afterSignOutUrl="/" />
}
```

### 6. Making API Calls to Backend

#### `src/hooks/useAuth.ts`

```tsx
import { useAuth as useClerkAuth } from '@clerk/clerk-react'

export function useAuth() {
  const { getToken, isLoaded, isSignedIn } = useClerkAuth()

  const fetchWithAuth = async (url: string, options: RequestInit = {}) => {
    const token = await getToken()

    const response = await fetch(url, {
      ...options,
      headers: {
        ...options.headers,
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    })

    if (!response.ok) {
      throw new Error(`API error: ${response.statusText}`)
    }

    return response.json()
  }

  return {
    fetchWithAuth,
    isLoaded,
    isSignedIn,
  }
}
```

#### `src/hooks/useUser.ts`

```tsx
import { useEffect, useState } from 'react'
import { useAuth } from './useAuth'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL

interface User {
  id: string
  clerk_id: string
  email: string
  username?: string
  first_name: string
  last_name: string
  profile_image_url: string
  subscription_tier: string
  subscription_status: string
}

interface UserData {
  user: User
  subscription: {
    tier: string
    status: string
    expires_at: string | null
    is_premium: boolean
  }
  usage: {
    conversions_this_month: number
    can_convert: boolean
  }
}

export function useUser() {
  const { fetchWithAuth, isLoaded, isSignedIn } = useAuth()
  const [userData, setUserData] = useState<UserData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!isLoaded || !isSignedIn) {
      setLoading(false)
      return
    }

    const fetchUserData = async () => {
      try {
        const data = await fetchWithAuth(`${API_BASE_URL}/api/me`)
        setUserData(data)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch user data')
      } finally {
        setLoading(false)
      }
    }

    fetchUserData()
  }, [isLoaded, isSignedIn, fetchWithAuth])

  return { userData, loading, error }
}
```

### 7. Example App Component

#### `src/App.tsx`

```tsx
import { SignedIn, SignedOut, useUser as useClerkUser } from '@clerk/clerk-react'
import SignInButton from './components/SignInButton'
import SignUpButton from './components/SignUpButton'
import UserButton from './components/UserButton'
import { useUser } from './hooks/useUser'

function Dashboard() {
  const { user: clerkUser } = useClerkUser()
  const { userData, loading, error } = useUser()

  if (loading) {
    return <div>Loading...</div>
  }

  if (error) {
    return <div>Error: {error}</div>
  }

  return (
    <div>
      <h1>Welcome, {clerkUser?.firstName}!</h1>
      {userData && (
        <div>
          <h2>Account Info</h2>
          <p>Email: {userData.user.email}</p>
          <p>Subscription: {userData.subscription.tier}</p>
          <p>Status: {userData.subscription.status}</p>
          <p>
            Conversions this month: {userData.usage.conversions_this_month}
          </p>
          {userData.subscription.is_premium ? (
            <span>✨ Premium User</span>
          ) : (
            <span>🆓 Free User</span>
          )}
        </div>
      )}
      <UserButton />
    </div>
  )
}

function App() {
  return (
    <div className="App">
      <SignedOut>
        <h1>Welcome to sptyt</h1>
        <p>Convert music links between Spotify and YouTube</p>
        <SignInButton />
        <SignUpButton />
      </SignedOut>

      <SignedIn>
        <Dashboard />
      </SignedIn>
    </div>
  )
}

export default App
```

## Authentication Flow

### Sign In / Sign Up

Clerk provides built-in UI components for authentication. When users click sign in/up:

1. Clerk modal opens with options:
   - **Google Sign-In** (OAuth)
   - **Email + OTP** (passwordless)
   - Email + Password (if enabled)

2. After authentication:
   - Clerk manages the session
   - JWT token is automatically stored
   - User is redirected back to your app

### Making Authenticated Requests

```tsx
const { getToken } = useAuth()

// Get the JWT token
const token = await getToken()

// Make authenticated request to your Go backend
const response = await fetch('http://localhost:8080/api/me', {
  headers: {
    'Authorization': `Bearer ${token}`,
  },
})
```

## Backend API Endpoints

### `/api/me` (GET) - Protected

Returns authenticated user's profile and subscription info.

**Request:**
```bash
curl -H "Authorization: Bearer <clerk_jwt_token>" \
  http://localhost:8080/api/me
```

**Response:**
```json
{
  "user": {
    "id": "uuid",
    "clerk_id": "user_xxx",
    "email": "user@example.com",
    "username": "username",
    "first_name": "John",
    "last_name": "Doe",
    "profile_image_url": "https://...",
    "subscription_tier": "free",
    "subscription_status": "inactive",
    "created_at": "2024-11-08T..."
  },
  "subscription": {
    "tier": "free",
    "status": "inactive",
    "expires_at": null,
    "is_premium": false
  },
  "usage": {
    "conversions_this_month": 10,
    "can_convert": true
  }
}
```

### Public Routes (No Auth Required)

- `GET /` - Landing page
- `GET /:link` - Smart redirect (Spotify to YouTube)
- `GET /ly/:link` - Redirect to lyric video
- `GET /gn/:link` - Redirect to Genius lyrics
- `GET /yt/:youtube_link` - YouTube to Spotify redirect

## Clerk Dashboard Configuration

### Enable Google OAuth

1. Go to **Authentication** → **Social Connections**
2. Enable **Google**
3. Use Clerk's development OAuth credentials (auto-configured)
4. For production, add your own Google OAuth credentials

### Enable Email OTP

1. Go to **Authentication** → **Email, phone, username**
2. Under **Contact information**, enable **Email address**
3. Set **Verification methods** to **Email verification code**
4. Save changes

Now users can sign in with:
- Google account (OAuth)
- Email + OTP (passwordless)

## Development

### Run Backend

```bash
# In sptyt directory
go run main.go
```

Backend runs on `http://localhost:8080`

### Run Frontend

```bash
# In sptyt-app directory
npm run dev
```

Frontend runs on `http://localhost:5173`

## Production Deployment

### Backend

1. Set environment variables in production:
   - `CLERK_SECRET_KEY` (from Clerk Dashboard)
   - Database credentials
   - Other API keys

### Frontend

1. Update `.env.production`:
   ```env
   VITE_CLERK_PUBLISHABLE_KEY=pk_live_your_live_key
   VITE_API_BASE_URL=https://api.yourdomain.com
   ```

2. Update Clerk Dashboard:
   - Add production URLs to allowed domains
   - Set up production Google OAuth credentials

## Hybrid Landing Page Setup

To keep your current static landing page and add React app for authenticated features:

### Structure

```
/                    → Static landing page (index.html)
/app/*               → React SPA for authenticated users
/api/me              → Backend API endpoint
```

### Implementation

1. Keep `web/templates/index.html` as is
2. Deploy React app to `/app` route using static file server or CDN
3. Add link from landing page to React app:

```html
<!-- In your current index.html -->
<a href="/app">Open Dashboard</a>
```

## Next Steps

1. **DodoPay Integration**: Add webhook handlers for subscription events
2. **Premium Features**: Implement playlist conversion, batch operations, etc.
3. **User Onboarding**: Create user profile on first sign-in
4. **Analytics**: Track conversions and usage
5. **Custom Links**: Premium feature for branded short URLs

## Troubleshooting

### Token Issues

If you get "Invalid token" errors:
- Check that `CLERK_SECRET_KEY` is set in backend
- Verify Clerk Publishable Key in frontend
- Ensure token is sent in `Authorization: Bearer <token>` header

### CORS Issues

Backend has CORS enabled. If you still face issues:
```go
// In main.go, configure CORS
e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins: []string{"http://localhost:5173"},
    AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAuthorization},
}))
```

### Database Connection

Make sure PostgreSQL is running and credentials in `.env` are correct:
```bash
# Test connection
psql -h localhost -U postgres -d sptyt
```

## Resources

- [Clerk React Documentation](https://clerk.com/docs/quickstarts/react)
- [Clerk Backend Integration](https://clerk.com/docs/backend-requests/overview)
- [Vite Environment Variables](https://vitejs.dev/guide/env-and-mode.html)
