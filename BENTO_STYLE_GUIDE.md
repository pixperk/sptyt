# Custom Links - Bento Style Implementation Guide

This guide explains how to implement **Bento.me-style** custom links with a flexible grid layout system where each element has its own styling and positioning.

## 🎨 What is Bento Style?

Bento style is inspired by [Bento.me](https://bento.me) - a modular, grid-based layout where:
- Elements have **different sizes** (1x1, 2x1, 1x2, 2x2 grid cells)
- Each element has **custom styling** (colors, borders, padding)
- Layout is **flexible and visually rich**
- No predefined page layouts - each element positions itself

---

## 📊 Architecture Overview

### Backend (API Only)
- **No proxy** - Frontend handles all rendering
- **API returns JSON** with element data and styling
- **Element styling stored in JSONB** (flexible schema)

### Frontend
- **Fetches link data** via `GET /api/links/:slug`
- **Renders Bento grid** using CSS Grid
- **Applies per-element styling** from element_data

---

## 🗂️ Data Structure

### CustomLink
```json
{
  "id": "uuid",
  "slug": "my-bento-link",
  "title": "My Music Collection",
  "description": "Check out my favorite tracks",
  "background_color": "#f5f5f5",
  "theme": "light",
  "elements": [...]
}
```

**Fields:**
- `background_color` - Page background (default: `#ffffff`)
- `theme` - `light`, `dark`, or `auto`
- No `layout_type` - each element controls its own grid position

### Element Types

#### 1. **Song Element**
```json
{
  "element_type": "song",
  "element_data": {
    // Bento Styling
    "grid_column": "span 2",
    "grid_row": "span 1",
    "background_color": "#1DB954",
    "border_radius": "16px",
    "text_color": "#ffffff",
    "padding": "24px",

    // Song Data
    "title": "Blinding Lights",
    "artists": "The Weeknd",
    "cover_image": "https://i.scdn.co/image/...",
    "duration": "3:20",
    "spotify_url": "https://open.spotify.com/track/...",
    "youtube_url": "https://youtube.com/watch?v=...",
    "youtube_lyric_url": "https://youtube.com/watch?v=...",
    "genius_url": "https://genius.com/..."
  }
}
```

#### 2. **Playlist Element**
```json
{
  "element_type": "playlist",
  "element_data": {
    // Bento Styling
    "grid_column": "span 2",
    "grid_row": "span 2",
    "background_color": "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
    "border_radius": "20px",
    "text_color": "#ffffff",
    "padding": "32px",

    // Playlist Data
    "title": "Summer Vibes 2024",
    "cover_image": "https://i.scdn.co/image/...",
    "track_count": 30,
    "playlist_spotify_url": "https://open.spotify.com/playlist/...",
    "playlist_youtube_url": "https://youtube.com/playlist?list=...",
    "conversion_id": "uuid"
  }
}
```

#### 3. **Custom Text Element**
```json
{
  "element_type": "text",
  "element_data": {
    // Bento Styling
    "grid_column": "span 1",
    "grid_row": "span 1",
    "background_color": "#FFE66D",
    "border_radius": "12px",
    "text_color": "#2C2C2C",
    "font_size": "18px",
    "padding": "20px",

    // Text Content
    "custom_text": "👋 Hey! Check out my music collection",
    "custom_html": "<h2>Welcome</h2><p>My curated playlist</p>"
  }
}
```

#### 4. **Link Element** (Simple Button)
```json
{
  "element_type": "link",
  "element_data": {
    // Bento Styling
    "grid_column": "span 1",
    "grid_row": "span 1",
    "background_color": "#000000",
    "border_radius": "50%",
    "text_color": "#ffffff",
    "padding": "16px",

    // Link Data
    "title": "Instagram",
    "link_url": "https://instagram.com/username",
    "link_icon": "instagram"  // or icon URL
  }
}
```

#### 5. **Image Element**
```json
{
  "element_type": "image",
  "element_data": {
    // Bento Styling
    "grid_column": "span 1",
    "grid_row": "span 1",
    "border_radius": "12px",
    "padding": "0",

    // Image Data
    "image_url": "https://example.com/photo.jpg",
    "image_alt": "Concert photo",
    "link_url": "https://example.com"  // Optional clickable
  }
}
```

---

## 🎨 Grid Sizing Options

### Grid Column (Width)
- `"span 1"` - 1 column wide (small)
- `"span 2"` - 2 columns wide (medium)
- `"span 3"` - 3 columns wide (large)
- `"span 4"` - 4 columns wide (full width on desktop)

### Grid Row (Height)
- `"span 1"` - 1 row tall (short)
- `"span 2"` - 2 rows tall (medium)
- `"span 3"` - 3 rows tall (tall)

### Recommended Combinations
- **Small card**: `"span 1"` column, `"span 1"` row (1x1)
- **Wide card**: `"span 2"` column, `"span 1"` row (2x1)
- **Tall card**: `"span 1"` column, `"span 2"` row (1x2)
- **Large card**: `"span 2"` column, `"span 2"` row (2x2)
- **Hero card**: `"span 4"` column, `"span 2"` row (4x2)

---

## 🎯 API Endpoints

### Get Link Data (Public)
```
GET /api/links/:slug
```

Returns complete link with all elements and their styling:
```json
{
  "id": "uuid",
  "slug": "my-bento-link",
  "title": "My Music Collection",
  "background_color": "#f5f5f5",
  "theme": "light",
  "elements": [
    {
      "id": "uuid",
      "element_type": "song",
      "element_data": { /* styling + data */ },
      "display_index": 0
    },
    {
      "id": "uuid",
      "element_type": "playlist",
      "element_data": { /* styling + data */ },
      "display_index": 1
    }
  ]
}
```

### Get Song Element Data
```
GET /api/links/element-data/song?spotify_url=<url>
```

Returns song data that you can customize with Bento styling:
```json
{
  "element_type": "song",
  "element_data": {
    "title": "Song Title",
    "artists": "Artist Name",
    "cover_image": "https://i.scdn.co/image/...",
    "duration": "3:20",
    "spotify_url": "...",
    "youtube_url": "...",
    "youtube_lyric_url": "...",
    "genius_url": "..."
  }
}
```

**Then add Bento styling before saving:**
```typescript
const songData = await api.getSongElementData(spotifyUrl);
songData.element_data.grid_column = "span 2";
songData.element_data.grid_row = "span 1";
songData.element_data.background_color = "#1DB954";
songData.element_data.border_radius = "16px";
songData.element_data.text_color = "#ffffff";
songData.element_data.padding = "24px";

await api.addElement(linkId, songData);
```

### Add Element with Styling
```
POST /api/links/:id/elements
```

```json
{
  "element_type": "song",
  "element_data": {
    // Bento styling first
    "grid_column": "span 2",
    "grid_row": "span 1",
    "background_color": "#1DB954",
    "border_radius": "16px",
    "text_color": "#ffffff",
    "padding": "24px",

    // Then data
    "title": "Song Title",
    "artists": "Artist Name",
    "cover_image": "https://...",
    "spotify_url": "https://..."
  }
}
```

---

## 💻 Frontend Implementation

### 1. Create Bento Grid Container

```tsx
// components/BentoGrid.tsx
interface BentoGridProps {
  link: CustomLink;
}

export function BentoGrid({ link }: BentoGridProps) {
  return (
    <div
      className="min-h-screen p-6"
      style={{ backgroundColor: link.background_color }}
    >
      <div className="max-w-6xl mx-auto">
        {/* Header */}
        <header className="mb-8 text-center">
          <h1 className="text-4xl font-bold mb-2">{link.title}</h1>
          <p className="text-gray-600">{link.description}</p>
        </header>

        {/* Bento Grid */}
        <div className="bento-grid">
          {link.elements
            .sort((a, b) => a.display_index - b.display_index)
            .map((element) => (
              <BentoElement key={element.id} element={element} linkId={link.id} />
            ))}
        </div>
      </div>
    </div>
  );
}
```

### 2. CSS Grid Layout

```css
/* styles/bento.css */
.bento-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
  grid-auto-rows: 200px;
}

@media (min-width: 768px) {
  .bento-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (max-width: 767px) {
  .bento-grid {
    grid-template-columns: repeat(2, 1fr);
    grid-auto-rows: 150px;
  }
}
```

### 3. Bento Element Component

```tsx
// components/BentoElement.tsx
import { LinkElement } from '@/types';

interface Props {
  element: LinkElement;
  linkId: string;
}

export function BentoElement({ element, linkId }: Props) {
  const data = element.element_data;

  // Apply element styling
  const style = {
    gridColumn: data.grid_column || 'span 1',
    gridRow: data.grid_row || 'span 1',
    background: data.background_color || '#ffffff',
    borderRadius: data.border_radius || '12px',
    border: data.border_color ? `2px solid ${data.border_color}` : 'none',
    color: data.text_color || '#000000',
    fontSize: data.font_size || '16px',
    padding: data.padding || '20px',
  };

  return (
    <div className="bento-element" style={style}>
      {element.element_type === 'song' && <SongCard element={element} linkId={linkId} />}
      {element.element_type === 'playlist' && <PlaylistCard element={element} linkId={linkId} />}
      {element.element_type === 'text' && <TextCard element={element} />}
      {element.element_type === 'link' && <LinkCard element={element} linkId={linkId} />}
      {element.element_type === 'image' && <ImageCard element={element} />}
    </div>
  );
}
```

### 4. Song Card Example

```tsx
// components/elements/SongCard.tsx
export function SongCard({ element, linkId }: Props) {
  const data = element.element_data;

  return (
    <div className="flex flex-col h-full">
      <img
        src={data.cover_image}
        alt={data.title}
        className="w-full aspect-square object-cover rounded-lg mb-3"
      />
      <h3 className="font-bold text-lg mb-1 truncate">{data.title}</h3>
      <p className="text-sm opacity-80 truncate">{data.artists}</p>
      <p className="text-xs opacity-60 mt-1">{data.duration}</p>

      <div className="flex gap-2 mt-auto pt-3">
        {data.spotify_url && (
          <a
            href={`/api/track/${linkId}/${element.id}`}
            className="flex-1 bg-black bg-opacity-20 rounded-lg py-2 text-center text-sm font-medium hover:bg-opacity-30 transition"
          >
            Spotify
          </a>
        )}
        {data.youtube_url && (
          <a
            href={data.youtube_url}
            target="_blank"
            className="flex-1 bg-black bg-opacity-20 rounded-lg py-2 text-center text-sm font-medium hover:bg-opacity-30 transition"
          >
            YouTube
          </a>
        )}
      </div>
    </div>
  );
}
```

### 5. Element Editor (Backend UI)

```tsx
// components/editor/ElementStyleEditor.tsx
export function ElementStyleEditor({ elementData, onChange }) {
  return (
    <div className="space-y-4">
      <h3 className="font-bold">Bento Styling</h3>

      {/* Grid Size */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label>Width</label>
          <select
            value={elementData.grid_column || 'span 1'}
            onChange={(e) => onChange({ ...elementData, grid_column: e.target.value })}
          >
            <option value="span 1">Small (1 col)</option>
            <option value="span 2">Medium (2 cols)</option>
            <option value="span 3">Large (3 cols)</option>
            <option value="span 4">Full (4 cols)</option>
          </select>
        </div>

        <div>
          <label>Height</label>
          <select
            value={elementData.grid_row || 'span 1'}
            onChange={(e) => onChange({ ...elementData, grid_row: e.target.value })}
          >
            <option value="span 1">Short (1 row)</option>
            <option value="span 2">Medium (2 rows)</option>
            <option value="span 3">Tall (3 rows)</option>
          </select>
        </div>
      </div>

      {/* Colors */}
      <div>
        <label>Background Color</label>
        <input
          type="color"
          value={elementData.background_color || '#ffffff'}
          onChange={(e) => onChange({ ...elementData, background_color: e.target.value })}
        />
      </div>

      <div>
        <label>Text Color</label>
        <input
          type="color"
          value={elementData.text_color || '#000000'}
          onChange={(e) => onChange({ ...elementData, text_color: e.target.value })}
        />
      </div>

      {/* Border Radius */}
      <div>
        <label>Border Radius</label>
        <input
          type="range"
          min="0"
          max="40"
          value={parseInt(elementData.border_radius || '12')}
          onChange={(e) => onChange({ ...elementData, border_radius: `${e.target.value}px` })}
        />
      </div>
    </div>
  );
}
```

---

## 🎨 Preset Styles

### Spotify Green Theme
```json
{
  "background_color": "#1DB954",
  "text_color": "#ffffff",
  "border_radius": "16px",
  "padding": "24px"
}
```

### Dark Gradient
```json
{
  "background_color": "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
  "text_color": "#ffffff",
  "border_radius": "20px",
  "padding": "32px"
}
```

### Minimal White
```json
{
  "background_color": "#ffffff",
  "text_color": "#000000",
  "border_radius": "12px",
  "border_color": "#e5e5e5",
  "padding": "20px"
}
```

### Neon Pink
```json
{
  "background_color": "#FF006E",
  "text_color": "#ffffff",
  "border_radius": "24px",
  "padding": "28px"
}
```

---

## 📱 Responsive Design

```css
/* Mobile: 2 columns, smaller cards */
@media (max-width: 767px) {
  .bento-grid {
    grid-template-columns: repeat(2, 1fr);
    grid-auto-rows: 150px;
    gap: 12px;
  }

  /* Force all elements to span 1 on mobile */
  .bento-element[style*="span 3"],
  .bento-element[style*="span 4"] {
    grid-column: span 2 !important;
  }
}

/* Tablet: 3 columns */
@media (min-width: 768px) and (max-width: 1023px) {
  .bento-grid {
    grid-template-columns: repeat(3, 1fr);
    grid-auto-rows: 180px;
  }
}

/* Desktop: 4 columns */
@media (min-width: 1024px) {
  .bento-grid {
    grid-template-columns: repeat(4, 1fr);
    grid-auto-rows: 200px;
    gap: 16px;
  }
}
```

---

## 🚀 Key Changes from Previous Version

### ❌ Removed
- **Frontend proxy** (`/l/:slug` route)
- **Global `layout_type`** (horizontal, grid, compact, etc.)
- **Fixed layouts** - now fully flexible

### ✅ Added
- **Per-element grid styling** (column/row span)
- **Per-element colors** (background, text, border)
- **Per-element sizing** (padding, border-radius, font-size)
- **New element types** (link, image, text)
- **Page background color**
- **Gradient support**

### 🔄 Changed
- Frontend **fetches via API only** (no proxy)
- **CSS Grid** replaces fixed layout types
- Elements are **self-contained** with their own styles

---

## ✅ Implementation Checklist

- [ ] Update API client to handle new element_data fields
- [ ] Create BentoGrid component with CSS Grid
- [ ] Create BentoElement component that applies styles
- [ ] Build element editors (SongCard, PlaylistCard, etc.)
- [ ] Add style editor UI for customizing elements
- [ ] Implement drag & drop for reordering
- [ ] Add preset style templates
- [ ] Test responsive layouts (mobile, tablet, desktop)
- [ ] Add gradient picker for backgrounds
- [ ] Implement theme toggle (light/dark)

---

## 🎯 Example: Creating a Bento Link

```typescript
// 1. Create link with background color
const { link } = await api.createLink({
  title: "My Music Collection",
  description: "Curated by me",
  background_color: "#f5f5f5",
  theme: "light"
});

// 2. Add hero song (large, 2x2)
const heroSong = await api.getSongElementData('https://open.spotify.com/track/...');
await api.addElement(link.id, {
  element_type: 'song',
  element_data: {
    ...heroSong.element_data,
    grid_column: "span 2",
    grid_row: "span 2",
    background_color: "#1DB954",
    border_radius: "20px",
    text_color: "#ffffff",
    padding: "32px"
  }
});

// 3. Add playlist (medium, 2x1)
const playlist = await api.getConversionSongs(conversionId);
await api.addElement(link.id, {
  element_type: 'playlist',
  element_data: {
    title: playlist.playlist_name,
    cover_image: playlist.cover_image,
    track_count: playlist.track_count,
    playlist_spotify_url: playlist.spotify_playlist_url,
    playlist_youtube_url: playlist.youtube_playlist_url,
    conversion_id: playlist.conversion_id,
    grid_column: "span 2",
    grid_row: "span 1",
    background_color: "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
    border_radius: "16px",
    text_color: "#ffffff",
    padding: "24px"
  }
});

// 4. Add Instagram link (small, 1x1)
await api.addElement(link.id, {
  element_type: 'link',
  element_data: {
    title: "Instagram",
    link_url: "https://instagram.com/username",
    link_icon: "instagram",
    grid_column: "span 1",
    grid_row: "span 1",
    background_color: "#E1306C",
    border_radius: "50%",
    text_color: "#ffffff",
    padding: "20px"
  }
});

// 5. Share the link!
// https://sptyt.xyz/l/my-music-collection
```

---

For questions or implementation help, refer to the backend code or API documentation.
