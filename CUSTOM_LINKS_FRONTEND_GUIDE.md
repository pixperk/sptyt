# Custom Links - Frontend Integration Guide

This guide covers everything needed to integrate the custom links feature on the frontend.

## Table of Contents
- [API Endpoints](#api-endpoints)
- [Data Types](#data-types)
- [Feature Flow](#feature-flow)
- [Implementation Steps](#implementation-steps)
- [UI Components](#ui-components)
- [Code Examples](#code-examples)

## API Endpoints

### Public Endpoints (No Authentication)

#### 1. Get Link by Slug
```
GET /api/links/:slug
```
Returns link data with all elements for public viewing. Automatically tracks page view.

**Response:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "slug": "my-playlist",
  "title": "My Summer Playlist",
  "description": "Best songs for summer 2024",
  "layout_type": "little_broad",
  "theme": "auto",
  "is_password_protected": false,
  "conversion_id": "uuid",
  "expires_at": null,
  "view_count": 42,
  "is_public": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z",
  "elements": [
    {
      "id": "uuid",
      "custom_link_id": "uuid",
      "element_type": "spotify_track",
      "element_data": {
        "track_name": "Song Title",
        "artists": "Artist Name",
        "cover_image": "https://...",
        "duration": "3:45",
        "spotify_url": "https://open.spotify.com/track/...",
        "youtube_url": "https://youtube.com/watch?v=...",
        "genius_url": "https://genius.com/..."
      },
      "display_index": 0,
      "is_visible": true,
      "click_count": 15,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

**Error Responses:**
- `404` - Link not found
- `410 Gone` - Link has expired (free tier after 7 days)

#### 2. Verify Password (Password-Protected Links)
```
POST /api/links/:slug/verify
```
Verifies the password for a password-protected link.

**Request Body:**
```json
{
  "password": "secret123"
}
```

**Response (Success):**
```json
{
  "valid": true
}
```

**Error Responses:**
- `401 Unauthorized` - Incorrect password
- `404 Not Found` - Link not found

**Usage Flow:**
1. Try to fetch link data with `GET /api/links/:slug`
2. If `is_password_protected: true`, show password prompt
3. User enters password → POST to `/api/links/:slug/verify`
4. If valid, store verification in session/localStorage
5. Re-fetch link data or allow access

#### 3. Track Element Click
```
GET /api/track/:link_id/:element_id
```
Tracks a click and redirects to the target URL (Spotify/YouTube/Genius).

**Behavior:**
- Increments element click count
- Records analytics (IP, user agent, referrer)
- Redirects to appropriate URL based on element type
- Returns `302 Found` redirect

**Usage:**
```jsx
// Instead of direct links, use this endpoint
<a href={`/api/track/${linkId}/${element.id}`}>
  Listen on Spotify
</a>
```

### Protected Endpoints (Require Authentication)

All protected endpoints require `Authorization: Bearer <clerk_token>` header.

#### 3. Create Custom Link
```
POST /api/links
```

**Request Body:**
```json
{
  "title": "My Playlist",
  "description": "Optional description",
  "custom_slug": "my-custom-slug",  // Premium only, optional
  "layout_type": "little_broad",     // horizontal, full_broad, little_broad, grid, compact
  "theme": "auto",                   // light, dark, auto
  "password": "secret123",           // Premium only, optional
  "conversion_id": "uuid",           // Optional - link to a playlist conversion
  "is_public": true
}
```

**Response:**
```json
{
  "link": { /* CustomLink object */ },
  "public_url": "https://sptyt.xyz/l/my-custom-slug"
}
```

**Error Responses:**
- `403 Forbidden` - Free tier limit reached (3 links max)
- `403 Forbidden` - Custom slug requires premium
- `403 Forbidden` - Password protection requires premium
- `400 Bad Request` - Slug already taken

#### 4. Get User's Links
```
GET /api/links?limit=20&offset=0
```

**Response:**
```json
{
  "links": [ /* Array of CustomLink objects */ ],
  "total": 5,
  "limit": 20,
  "offset": 0
}
```

#### 5. Get Specific Link (Owner)
```
GET /api/links/:id
```

Returns link with all elements (including hidden ones) for owner.

#### 6. Update Link
```
PUT /api/links/:id
```

**Request Body (all fields optional):**
```json
{
  "title": "Updated Title",
  "description": "Updated description",
  "layout_type": "grid",
  "theme": "dark",
  "is_public": false
}
```

#### 7. Delete Link
```
DELETE /api/links/:id
```

Deletes the link and all its elements permanently.

#### 8. Add Element
```
POST /api/links/:id/elements
```

**Request Body:**
```json
{
  "element_type": "spotify_track",  // spotify_track, youtube_video, genius_lyrics, custom_text
  "element_data": {
    "track_name": "Song Title",
    "artists": "Artist Name",
    "cover_image": "https://...",
    "duration": "3:45",
    "spotify_url": "https://...",
    "youtube_url": "https://...",
    "genius_url": "https://..."
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "custom_link_id": "uuid",
  "element_type": "spotify_track",
  "element_data": { /* ... */ },
  "display_index": 0,
  "is_visible": true,
  "click_count": 0,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

**Error Responses:**
- `403 Forbidden` - Free tier limit reached (10 elements max per link)

#### 9. Reorder Elements (Drag & Drop)
```
PUT /api/links/:id/elements/reorder
```

**Request Body:**
```json
[
  { "element_id": "uuid-1", "index": 0 },
  { "element_id": "uuid-2", "index": 1 },
  { "element_id": "uuid-3", "index": 2 }
]
```

#### 10. Delete Element
```
DELETE /api/links/:id/elements/:element_id
```

#### 11. Get Link Analytics
```
GET /api/links/:id/analytics
```

**Response:**
```json
{
  "link_id": "uuid",
  "total_views": 150,
  "total_clicks": 89,
  "recent_views": 45,  // Last 30 days
  "view_count": 150,
  "element_stats": [
    {
      "element_id": "uuid",
      "click_count": 30
    }
  ]
}
```

## Data Types

### CustomLink
```typescript
interface CustomLink {
  id: string;
  user_id: string;
  slug: string;
  title: string;
  description?: string;
  layout_type: 'horizontal' | 'full_broad' | 'little_broad' | 'grid' | 'compact';
  theme: 'light' | 'dark' | 'auto';
  is_password_protected: boolean;
  conversion_id?: string;
  expires_at?: string | null;  // ISO 8601 date string
  view_count: number;
  is_public: boolean;
  created_at: string;
  updated_at: string;
  elements?: LinkElement[];
}
```

### LinkElement
```typescript
interface LinkElement {
  id: string;
  custom_link_id: string;
  element_type: 'spotify_track' | 'youtube_video' | 'genius_lyrics' | 'custom_text';
  element_data: ElementData;
  display_index: number;
  is_visible: boolean;
  click_count: number;
  created_at: string;
  updated_at: string;
}
```

### ElementData
```typescript
interface ElementData {
  // Track information
  track_name?: string;
  artists?: string;
  cover_image?: string;
  duration?: string;  // "3:45"

  // Links
  spotify_url?: string;
  youtube_url?: string;
  genius_url?: string;

  // Custom elements
  custom_text?: string;
  custom_html?: string;
  custom_color?: string;  // Hex color
}
```

### Analytics
```typescript
interface LinkAnalytics {
  link_id: string;
  total_views: number;
  total_clicks: number;
  recent_views: number;  // Last 30 days
  view_count: number;
  element_stats: ElementStats[];
}

interface ElementStats {
  element_id: string;
  click_count: number;
}
```

## Feature Flow

### User Journey: Creating a Custom Link

1. **User converts a playlist** → Gets conversion ID
2. **User clicks "Create Custom Link"** → Opens link creation modal
3. **User fills form:**
   - Title (required)
   - Description (optional)
   - Custom slug (premium only)
   - Layout type selection
   - Theme selection
   - Password (premium only)
   - Public/Private toggle
4. **Submit** → POST `/api/links`
5. **Redirect to link editor** → Add/reorder elements
6. **Add elements from conversion** → Populate with track data
7. **Preview link** → View public page
8. **Share link** → Copy `https://sptyt.xyz/l/slug`

### User Journey: Viewing a Custom Link (Public)

1. **User visits `/l/my-playlist`** → Backend proxies to frontend
2. **Frontend fetches data** → GET `/api/links/my-playlist`
3. **Check if expired** → Show expiry message if needed
4. **Render layout** → Display elements based on `layout_type`
5. **User clicks element** → Redirects through `/api/track/:link_id/:element_id`
6. **Analytics tracked** → View count increments, click recorded

### User Journey: Managing Links (Dashboard)

1. **User visits dashboard** → GET `/api/links?limit=20&offset=0`
2. **Display link cards** → Show title, view count, created date
3. **User clicks "Edit"** → Navigate to editor
4. **User clicks "Analytics"** → GET `/api/links/:id/analytics`
5. **User clicks "Delete"** → Confirm → DELETE `/api/links/:id`

## Implementation Steps

### Step 1: Create API Client

```typescript
// lib/api/customLinks.ts
import { CustomLink, LinkElement, LinkAnalytics } from '@/types/customLinks';

const API_BASE = process.env.NEXT_PUBLIC_API_URL;

export class CustomLinksAPI {
  constructor(private getToken: () => Promise<string>) {}

  // Public endpoints
  async getLinkBySlug(slug: string): Promise<CustomLink> {
    const res = await fetch(`${API_BASE}/api/links/${slug}`);
    if (!res.ok) {
      if (res.status === 410) throw new Error('LINK_EXPIRED');
      throw new Error('Link not found');
    }
    return res.json();
  }

  // Protected endpoints
  async createLink(data: CreateLinkRequest): Promise<{ link: CustomLink; public_url: string }> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    });

    if (!res.ok) {
      const error = await res.json();
      if (res.status === 403 && error.upgrade_required) {
        throw new Error('FREE_TIER_LIMIT');
      }
      throw new Error(error.error || 'Failed to create link');
    }

    return res.json();
  }

  async getUserLinks(limit = 20, offset = 0): Promise<{ links: CustomLink[]; total: number }> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links?limit=${limit}&offset=${offset}`, {
      headers: { 'Authorization': `Bearer ${token}` },
    });
    return res.json();
  }

  async updateLink(id: string, data: UpdateLinkRequest): Promise<void> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${id}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new Error('Failed to update link');
  }

  async deleteLink(id: string): Promise<void> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${id}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` },
    });
    if (!res.ok) throw new Error('Failed to delete link');
  }

  async addElement(linkId: string, data: AddElementRequest): Promise<LinkElement> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${linkId}/elements`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new Error('Failed to add element');
    return res.json();
  }

  async reorderElements(linkId: string, order: { element_id: string; index: number }[]): Promise<void> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${linkId}/elements/reorder`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify(order),
    });
    if (!res.ok) throw new Error('Failed to reorder elements');
  }

  async deleteElement(linkId: string, elementId: string): Promise<void> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${linkId}/elements/${elementId}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` },
    });
    if (!res.ok) throw new Error('Failed to delete element');
  }

  async getAnalytics(linkId: string): Promise<LinkAnalytics> {
    const token = await this.getToken();
    const res = await fetch(`${API_BASE}/api/links/${linkId}/analytics`, {
      headers: { 'Authorization': `Bearer ${token}` },
    });
    if (!res.ok) throw new Error('Failed to get analytics');
    return res.json();
  }

  async verifyPassword(slug: string, password: string): Promise<boolean> {
    const res = await fetch(`${API_BASE}/api/links/${slug}/verify`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    });
    if (res.ok) return true;
    if (res.status === 401) return false;
    throw new Error('Failed to verify password');
  }
}
```

### Step 2: Public Link Page

```tsx
// app/l/[slug]/page.tsx
'use client';

import { useState, useEffect } from 'react';
import { CustomLinksAPI } from '@/lib/api/customLinks';
import { LinkRenderer } from '@/components/customLinks/LinkRenderer';
import { PasswordPrompt } from '@/components/customLinks/PasswordPrompt';
import { CustomLink } from '@/types/customLinks';

export default function PublicLinkPage({ params }: { params: { slug: string } }) {
  const [link, setLink] = useState<CustomLink | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [needsPassword, setNeedsPassword] = useState(false);
  const [isVerified, setIsVerified] = useState(false);

  useEffect(() => {
    loadLink();
  }, [params.slug]);

  async function loadLink() {
    try {
      const api = new CustomLinksAPI(async () => '');
      const data = await api.getLinkBySlug(params.slug);

      // Check if password protected
      if (data.is_password_protected && !isVerified) {
        setNeedsPassword(true);
        setLink(data); // Store link metadata for password check
      } else {
        setLink(data);
      }
    } catch (err) {
      if (err.message === 'LINK_EXPIRED') {
        setError('EXPIRED');
      } else {
        setError('NOT_FOUND');
      }
    } finally {
      setLoading(false);
    }
  }

  async function handlePasswordSubmit(password: string): Promise<boolean> {
    try {
      const api = new CustomLinksAPI(async () => '');
      const isValid = await api.verifyPassword(params.slug, password);

      if (isValid) {
        setIsVerified(true);
        setNeedsPassword(false);
        // Reload link data now that we're verified
        await loadLink();
        return true;
      }
      return false;
    } catch (err) {
      return false;
    }
  }

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center">Loading...</div>;
  }

  if (error === 'EXPIRED') {
    return <ExpiredLinkMessage />;
  }

  if (error === 'NOT_FOUND') {
    return <div className="min-h-screen flex items-center justify-center">Link not found</div>;
  }

  if (needsPassword && link) {
    return <PasswordPrompt onSubmit={handlePasswordSubmit} linkTitle={link.title} />;
  }

  if (!link) {
    return <div className="min-h-screen flex items-center justify-center">Link not found</div>;
  }

  return <LinkRenderer link={link} />;
}

function ExpiredLinkMessage() {
  return (
    <div className="min-h-screen flex items-center justify-center">
      <div className="text-center">
        <h1 className="text-4xl font-bold mb-4">Link Expired</h1>
        <p className="text-gray-600">
          This free tier link has expired after 7 days.
        </p>
        <p className="text-sm mt-2">
          Premium links never expire. <a href="/pricing" className="text-blue-600">Upgrade now</a>
        </p>
      </div>
    </div>
  );
}
```

### Step 3: Link Renderer Component

```tsx
// components/customLinks/LinkRenderer.tsx
'use client';

import { CustomLink } from '@/types/customLinks';
import { TrackCard } from './TrackCard';

interface Props {
  link: CustomLink;
}

export function LinkRenderer({ link }: Props) {
  const layoutClass = getLayoutClass(link.layout_type);

  return (
    <div className={`min-h-screen ${getThemeClass(link.theme)}`}>
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <header className="text-center mb-8">
          <h1 className="text-4xl font-bold mb-2">{link.title}</h1>
          {link.description && (
            <p className="text-gray-600 text-lg">{link.description}</p>
          )}
          <p className="text-sm text-gray-500 mt-2">
            {link.view_count} views
          </p>
        </header>

        {/* Elements Grid */}
        <div className={layoutClass}>
          {link.elements?.map((element) => (
            <TrackCard
              key={element.id}
              element={element}
              linkId={link.id}
            />
          ))}
        </div>

        {/* Footer */}
        <footer className="text-center mt-12 text-sm text-gray-500">
          <p>
            Created with <a href="https://sptyt.xyz" className="text-blue-600">sptyt.xyz</a>
          </p>
        </footer>
      </div>
    </div>
  );
}

function getLayoutClass(layoutType: string): string {
  switch (layoutType) {
    case 'horizontal':
      return 'flex overflow-x-auto gap-4 pb-4';
    case 'full_broad':
      return 'grid grid-cols-1 gap-6';
    case 'little_broad':
      return 'grid grid-cols-1 md:grid-cols-2 gap-4';
    case 'grid':
      return 'grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4';
    case 'compact':
      return 'space-y-2 max-w-2xl mx-auto';
    default:
      return 'grid grid-cols-1 md:grid-cols-2 gap-4';
  }
}

function getThemeClass(theme: string): string {
  if (theme === 'dark') return 'bg-gray-900 text-white';
  if (theme === 'light') return 'bg-white text-gray-900';
  return 'bg-white text-gray-900 dark:bg-gray-900 dark:text-white';
}
```

### Step 4: Track Card Component

```tsx
// components/customLinks/TrackCard.tsx
import { LinkElement } from '@/types/customLinks';
import Image from 'next/image';

interface Props {
  element: LinkElement;
  linkId: string;
}

export function TrackCard({ element, linkId }: Props) {
  const { element_data } = element;

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md overflow-hidden">
      {/* Cover Image */}
      {element_data.cover_image && (
        <div className="relative h-48 w-full">
          <Image
            src={element_data.cover_image}
            alt={element_data.track_name || 'Track cover'}
            fill
            className="object-cover"
          />
        </div>
      )}

      {/* Track Info */}
      <div className="p-4">
        <h3 className="font-bold text-lg mb-1">{element_data.track_name}</h3>
        <p className="text-gray-600 text-sm mb-2">{element_data.artists}</p>
        {element_data.duration && (
          <p className="text-gray-500 text-xs">{element_data.duration}</p>
        )}

        {/* Action Buttons */}
        <div className="flex gap-2 mt-4">
          {element_data.spotify_url && (
            <a
              href={`/api/track/${linkId}/${element.id}`}
              className="flex-1 bg-green-500 hover:bg-green-600 text-white py-2 px-4 rounded text-center text-sm font-medium"
              target="_blank"
            >
              Spotify
            </a>
          )}
          {element_data.youtube_url && (
            <a
              href={`/api/track/${linkId}/${element.id}`}
              className="flex-1 bg-red-500 hover:bg-red-600 text-white py-2 px-4 rounded text-center text-sm font-medium"
              target="_blank"
            >
              YouTube
            </a>
          )}
          {element_data.genius_url && (
            <a
              href={`/api/track/${linkId}/${element.id}`}
              className="flex-1 bg-yellow-500 hover:bg-yellow-600 text-white py-2 px-4 rounded text-center text-sm font-medium"
              target="_blank"
            >
              Lyrics
            </a>
          )}
        </div>
      </div>
    </div>
  );
}
```

### Step 5: Dashboard - Link Management

```tsx
// components/dashboard/CustomLinksManager.tsx
'use client';

import { useState, useEffect } from 'react';
import { useAuth } from '@clerk/nextjs';
import { CustomLinksAPI } from '@/lib/api/customLinks';
import { CustomLink } from '@/types/customLinks';

export function CustomLinksManager() {
  const { getToken } = useAuth();
  const [links, setLinks] = useState<CustomLink[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadLinks();
  }, []);

  async function loadLinks() {
    const api = new CustomLinksAPI(getToken);
    const data = await api.getUserLinks();
    setLinks(data.links);
    setLoading(false);
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this link? This cannot be undone.')) return;

    const api = new CustomLinksAPI(getToken);
    await api.deleteLink(id);
    setLinks(links.filter(l => l.id !== id));
  }

  if (loading) return <div>Loading...</div>;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">My Custom Links</h2>
        <button
          onClick={() => router.push('/links/create')}
          className="bg-blue-600 text-white px-4 py-2 rounded"
        >
          Create New Link
        </button>
      </div>

      <div className="grid gap-4">
        {links.map((link) => (
          <div key={link.id} className="border rounded-lg p-4">
            <div className="flex justify-between items-start">
              <div>
                <h3 className="font-bold text-lg">{link.title}</h3>
                <p className="text-sm text-gray-600">{link.description}</p>
                <div className="flex gap-4 mt-2 text-sm text-gray-500">
                  <span>{link.view_count} views</span>
                  <span>{link.elements?.length || 0} elements</span>
                  <span>
                    {link.expires_at
                      ? `Expires ${new Date(link.expires_at).toLocaleDateString()}`
                      : 'Never expires'
                    }
                  </span>
                </div>
              </div>

              <div className="flex gap-2">
                <a
                  href={`/l/${link.slug}`}
                  target="_blank"
                  className="text-blue-600 hover:underline text-sm"
                >
                  View
                </a>
                <button
                  onClick={() => router.push(`/links/${link.id}/edit`)}
                  className="text-blue-600 hover:underline text-sm"
                >
                  Edit
                </button>
                <button
                  onClick={() => router.push(`/links/${link.id}/analytics`)}
                  className="text-blue-600 hover:underline text-sm"
                >
                  Analytics
                </button>
                <button
                  onClick={() => handleDelete(link.id)}
                  className="text-red-600 hover:underline text-sm"
                >
                  Delete
                </button>
              </div>
            </div>

            <div className="mt-3">
              <input
                type="text"
                readOnly
                value={`https://sptyt.xyz/l/${link.slug}`}
                className="w-full px-3 py-2 border rounded text-sm"
                onClick={(e) => e.currentTarget.select()}
              />
            </div>
          </div>
        ))}
      </div>

      {links.length === 0 && (
        <div className="text-center py-12 text-gray-500">
          <p>No custom links yet. Create your first one!</p>
        </div>
      )}
    </div>
  );
}
```

### Step 6: Create Link Form

```tsx
// components/customLinks/CreateLinkForm.tsx
'use client';

import { useState } from 'react';
import { useAuth } from '@clerk/nextjs';
import { useRouter } from 'next/navigation';
import { CustomLinksAPI } from '@/lib/api/customLinks';

export function CreateLinkForm({ isPremium }: { isPremium: boolean }) {
  const { getToken } = useAuth();
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const [formData, setFormData] = useState({
    title: '',
    description: '',
    custom_slug: '',
    layout_type: 'little_broad',
    theme: 'auto',
    password: '',
    is_public: true,
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      const api = new CustomLinksAPI(getToken);
      const { link } = await api.createLink(formData);
      router.push(`/links/${link.id}/edit`);
    } catch (err) {
      if (err.message === 'FREE_TIER_LIMIT') {
        setError('Free tier limit reached. You can create up to 3 links. Upgrade to premium for unlimited links!');
      } else {
        setError(err.message);
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 max-w-2xl">
      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
          {error}
        </div>
      )}

      <div>
        <label className="block text-sm font-medium mb-1">Title *</label>
        <input
          type="text"
          required
          value={formData.title}
          onChange={(e) => setFormData({ ...formData, title: e.target.value })}
          className="w-full px-3 py-2 border rounded"
          placeholder="My Amazing Playlist"
        />
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">Description</label>
        <textarea
          value={formData.description}
          onChange={(e) => setFormData({ ...formData, description: e.target.value })}
          className="w-full px-3 py-2 border rounded"
          rows={3}
          placeholder="Tell people about this playlist..."
        />
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">
          Custom Slug {!isPremium && '(Premium Only)'}
        </label>
        <input
          type="text"
          disabled={!isPremium}
          value={formData.custom_slug}
          onChange={(e) => setFormData({ ...formData, custom_slug: e.target.value })}
          className="w-full px-3 py-2 border rounded disabled:bg-gray-100"
          placeholder="my-custom-slug"
        />
        <p className="text-xs text-gray-500 mt-1">
          Leave blank for random slug. {!isPremium && 'Upgrade to premium to use custom slugs.'}
        </p>
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">Layout</label>
        <select
          value={formData.layout_type}
          onChange={(e) => setFormData({ ...formData, layout_type: e.target.value })}
          className="w-full px-3 py-2 border rounded"
        >
          <option value="little_broad">Little Broad (2 columns)</option>
          <option value="full_broad">Full Broad (1 column)</option>
          <option value="horizontal">Horizontal Scroll</option>
          <option value="grid">Grid (4 columns)</option>
          <option value="compact">Compact List</option>
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">Theme</label>
        <select
          value={formData.theme}
          onChange={(e) => setFormData({ ...formData, theme: e.target.value })}
          className="w-full px-3 py-2 border rounded"
        >
          <option value="auto">Auto (follows system)</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">
          Password Protection {!isPremium && '(Premium Only)'}
        </label>
        <input
          type="password"
          disabled={!isPremium}
          value={formData.password}
          onChange={(e) => setFormData({ ...formData, password: e.target.value })}
          className="w-full px-3 py-2 border rounded disabled:bg-gray-100"
          placeholder="Optional password"
        />
      </div>

      <div className="flex items-center">
        <input
          type="checkbox"
          checked={formData.is_public}
          onChange={(e) => setFormData({ ...formData, is_public: e.target.checked })}
          className="mr-2"
        />
        <label className="text-sm">Make this link public</label>
      </div>

      <button
        type="submit"
        disabled={loading}
        className="w-full bg-blue-600 text-white py-2 px-4 rounded font-medium disabled:bg-gray-400"
      >
        {loading ? 'Creating...' : 'Create Link'}
      </button>
    </form>
  );
}
```

### Step 7: Analytics Dashboard

```tsx
// components/customLinks/AnalyticsDashboard.tsx
'use client';

import { useState, useEffect } from 'react';
import { useAuth } from '@clerk/nextjs';
import { CustomLinksAPI } from '@/lib/api/customLinks';
import { LinkAnalytics } from '@/types/customLinks';

export function AnalyticsDashboard({ linkId }: { linkId: string }) {
  const { getToken } = useAuth();
  const [analytics, setAnalytics] = useState<LinkAnalytics | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadAnalytics();
  }, [linkId]);

  async function loadAnalytics() {
    const api = new CustomLinksAPI(getToken);
    const data = await api.getAnalytics(linkId);
    setAnalytics(data);
    setLoading(false);
  }

  if (loading) return <div>Loading analytics...</div>;
  if (!analytics) return <div>No analytics available</div>;

  return (
    <div className="space-y-6">
      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <StatCard
          title="Total Views"
          value={analytics.total_views}
          subtitle="All time"
        />
        <StatCard
          title="Recent Views"
          value={analytics.recent_views}
          subtitle="Last 30 days"
        />
        <StatCard
          title="Total Clicks"
          value={analytics.total_clicks}
          subtitle="All time"
        />
        <StatCard
          title="Click Rate"
          value={`${((analytics.total_clicks / analytics.total_views) * 100).toFixed(1)}%`}
          subtitle="Clicks per view"
        />
      </div>

      {/* Element Performance */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
        <h3 className="text-lg font-bold mb-4">Element Performance</h3>
        <div className="space-y-2">
          {analytics.element_stats.map((stat, index) => (
            <div key={stat.element_id} className="flex items-center justify-between py-2 border-b">
              <div className="flex items-center gap-3">
                <span className="text-sm font-medium text-gray-500">#{index + 1}</span>
                <span className="text-sm">Element {stat.element_id.slice(0, 8)}...</span>
              </div>
              <div className="flex items-center gap-4">
                <span className="text-sm font-bold">{stat.click_count} clicks</span>
                <div className="w-32 bg-gray-200 rounded-full h-2">
                  <div
                    className="bg-blue-600 h-2 rounded-full"
                    style={{
                      width: `${(stat.click_count / analytics.total_clicks) * 100}%`
                    }}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function StatCard({ title, value, subtitle }: { title: string; value: string | number; subtitle: string }) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
      <p className="text-sm text-gray-600 mb-1">{title}</p>
      <p className="text-3xl font-bold mb-1">{value}</p>
      <p className="text-xs text-gray-500">{subtitle}</p>
    </div>
  );
}
```

### Step 8: Password Prompt Component

```tsx
// components/customLinks/PasswordPrompt.tsx
'use client';

import { useState } from 'react';

interface Props {
  onSubmit: (password: string) => Promise<boolean>;
  linkTitle: string;
}

export function PasswordPrompt({ onSubmit, linkTitle }: Props) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError('');

    const isValid = await onSubmit(password);

    if (!isValid) {
      setError('Incorrect password. Please try again.');
      setPassword('');
    }

    setLoading(false);
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="bg-white p-8 rounded-lg shadow-lg max-w-md w-full">
        <div className="text-center mb-6">
          <svg
            className="mx-auto h-12 w-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
            />
          </svg>
          <h2 className="mt-4 text-2xl font-bold">Password Protected</h2>
          <p className="text-gray-600 mt-2">{linkTitle}</p>
          <p className="text-sm text-gray-500 mt-1">
            This link is password protected. Enter the password to view.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter password"
              className="w-full px-4 py-3 border rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              required
              autoFocus
            />
            {error && (
              <p className="text-red-600 text-sm mt-2">{error}</p>
            )}
          </div>

          <button
            type="submit"
            disabled={loading || !password}
            className="w-full bg-blue-600 text-white py-3 px-4 rounded-lg font-medium hover:bg-blue-700 disabled:bg-gray-400 disabled:cursor-not-allowed"
          >
            {loading ? 'Verifying...' : 'Unlock'}
          </button>
        </form>

        <p className="text-xs text-center text-gray-500 mt-6">
          Don't have the password? Contact the link owner.
        </p>
      </div>
    </div>
  );
}
```

## UI Components

### Recommended Component Structure

```
components/
├── customLinks/
│   ├── LinkRenderer.tsx          # Public link display
│   ├── TrackCard.tsx             # Individual track/element card
│   ├── CreateLinkForm.tsx        # Link creation form
│   ├── LinkEditor.tsx            # Edit existing link
│   ├── ElementEditor.tsx         # Add/edit elements
│   ├── DragDropList.tsx          # Reorderable element list
│   ├── AnalyticsDashboard.tsx    # Analytics visualization
│   ├── PasswordPrompt.tsx        # Password entry for protected links
│   └── ShareModal.tsx            # Share link modal
├── dashboard/
│   └── CustomLinksManager.tsx    # Dashboard list view
```

## Important Notes

### Free Tier Limitations

Always check limits and show upgrade prompts:

```tsx
if (userLinks.length >= 3 && !isPremium) {
  return <UpgradePrompt feature="create more links" />;
}
```

### Link Expiration

Free tier links expire after 7 days. Show warnings:

```tsx
function ExpiryWarning({ expiresAt }: { expiresAt: string }) {
  const daysLeft = Math.ceil(
    (new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24)
  );

  if (daysLeft <= 2) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 p-3 rounded">
        ⚠️ This link expires in {daysLeft} days. Upgrade to premium for permanent links!
      </div>
    );
  }
  return null;
}
```

### SEO Optimization

Add metadata for shared links:

```tsx
// app/l/[slug]/page.tsx
export async function generateMetadata({ params }) {
  const link = await getLinkBySlug(params.slug);

  return {
    title: link.title,
    description: link.description,
    openGraph: {
      title: link.title,
      description: link.description,
      images: link.elements[0]?.element_data.cover_image
        ? [link.elements[0].element_data.cover_image]
        : [],
    },
  };
}
```

### Mobile Responsiveness

Ensure all layouts work on mobile:

```css
/* Horizontal scroll on mobile */
.horizontal-layout {
  display: flex;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
}

.horizontal-layout > * {
  scroll-snap-align: start;
  flex: 0 0 280px;
}
```

## Testing Checklist

- [ ] Create link as free user (test 3-link limit)
- [ ] Create link as premium user (test custom slug)
- [ ] Create password-protected link (premium only)
- [ ] View public link (test analytics tracking)
- [ ] View password-protected link (test password prompt)
- [ ] Submit wrong password (test error handling)
- [ ] Submit correct password (test unlock)
- [ ] Click elements (test redirect tracking)
- [ ] Edit link (test update)
- [ ] Reorder elements (test drag & drop)
- [ ] Delete elements
- [ ] Delete link
- [ ] View analytics
- [ ] Test all 5 layout types
- [ ] Test all 3 themes (light, dark, auto)
- [ ] Test expired link (mock expiry date)
- [ ] Test mobile responsiveness
- [ ] Test sharing (copy link, social media)
- [ ] Test password protection indicator on dashboard

## Support & Troubleshooting

**Issue: 403 errors when creating links**
- Check if user has reached free tier limit (3 links)
- Verify Clerk authentication token is being sent

**Issue: Analytics not updating**
- Analytics are tracked asynchronously via goroutines
- May take 1-2 seconds to reflect in database

**Issue: Elements not reordering**
- Ensure you're sending correct `display_index` values
- Check that element IDs match the link's elements

**Issue: Proxy not working for `/l/:slug`**
- Verify `FRONTEND_URL` is set correctly in backend
- Check CORS configuration allows your domain

## Next Steps

1. Implement the API client
2. Build the public link page (`/l/:slug`)
3. Create the dashboard link management UI
4. Add link creation form
5. Implement element editor with drag & drop
6. Build analytics dashboard
7. Add share functionality (copy link, social media)
8. Implement premium upgrade prompts
9. Add mobile optimizations
10. Test thoroughly!

---

For questions or issues, refer to the backend code or create an issue in the repository.
