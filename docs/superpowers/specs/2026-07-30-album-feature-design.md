# Album Feature Design

**Date:** 2026-07-30
**Status:** Approved

## Overview

Add an album system to ShutterSeek. Albums organize photos into named collections. A dedicated sidebar navigation lets users switch between the existing "All Photos" view and an "Albums" section. The album feature is built in two phases — Phase 1 is view-only (this spec), Phase 2 adds CRUD management.

## Database

Two new tables alongside the existing `photos` and `photo_embeddings`:

### albums

| Column | Type | Notes |
|---|---|---|
| id | BIGSERIAL PK | |
| title | VARCHAR(200) NOT NULL UNIQUE | |
| description | TEXT DEFAULT '' | |
| cover_photo_id | BIGINT FK→photos ON DELETE SET NULL | NULL = auto-pick first photo in album |
| sort_order | INTEGER DEFAULT 0 | Manual ordering between albums |
| created_at | TIMESTAMPTZ DEFAULT now() | |
| updated_at | TIMESTAMPTZ DEFAULT now() | |

### album_photos

| Column | Type | Notes |
|---|---|---|
| album_id | BIGINT FK→albums ON DELETE CASCADE | |
| photo_id | BIGINT FK→photos ON DELETE CASCADE | |
| sort_order | INTEGER DEFAULT 0 | Order within album |
| added_at | TIMESTAMPTZ DEFAULT now() | |

- PK: `(album_id, photo_id)` — a photo appears at most once per album
- Many-to-many: a photo can belong to multiple albums
- Flat structure, no nesting

### Initial population

Default albums are created from the top-level directory name in each photo's `file_path`:

```sql
INSERT INTO albums (title, sort_order, created_at, updated_at)
SELECT DISTINCT SPLIT_PART(file_path, '/', 1), 0, now(), now() FROM photos;

INSERT INTO album_photos (album_id, photo_id, sort_order)
SELECT a.id, p.id,
       ROW_NUMBER() OVER (PARTITION BY a.id ORDER BY p.taken_at DESC NULLS LAST, p.id DESC)
FROM photos p JOIN albums a ON a.title = SPLIT_PART(p.file_path, '/', 1);

UPDATE albums a SET cover_photo_id = (
  SELECT ap.photo_id FROM album_photos ap
  WHERE ap.album_id = a.id ORDER BY ap.sort_order LIMIT 1
);
```

## Backend

### Architecture

```
internal/
  handler/
    handler.go        ← existing photo list + original (untouched)
    album.go          ← new: album handlers
  service/
    original.go       ← existing
    album.go          ← new: album business logic
  model/
    albums.gen.go     ← new: GORM model for albums
    album_photos.gen.go ← new: GORM model for album_photos
```

Handler depends on Service, not on raw DB queries. Router passes the existing `Handler` struct (which gains an `AlbumSvc` field).

### API — Phase 1 (view-only)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/albums` | List all albums (cover URL, photo count) |
| GET | `/api/v1/albums/:id` | Single album detail |
| GET | `/api/v1/albums/:id/photos` | Paginated photos within album |

**GET /api/v1/albums response:**
```json
{
  "items": [{
    "id": 1,
    "title": "2024新加坡",
    "description": "",
    "cover_url": "/api/thumbnails/12345.webp",
    "photo_count": 1175,
    "sort_order": 10,
    "created_at": "2026-07-30T00:00:00Z",
    "updated_at": "2026-07-30T00:00:00Z"
  }],
  "total": 56
}
```

`cover_url` logic: if `cover_photo_id` is set, use that photo's thumbnail; otherwise use the first photo's thumbnail (by sort_order).

**GET /api/v1/albums/:id/photos** reuses the same cursor-pagination pattern as `ListPhotos`:

- Query: `SELECT * FROM photos WHERE id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)`
- Cursor format: `taken_at,id`
- Redis cache for first page only (same TTL strategy)

### API — Phase 2 (reserved)

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/albums` | Create album |
| PUT | `/api/v1/albums/:id` | Update title/description/cover |
| DELETE | `/api/v1/albums/:id` | Delete album |
| DELETE | `/api/v1/albums/:id/photos/:photo_id` | Remove photo from album |

## Frontend

### Routes

| Path | Component | Description |
|---|---|---|
| `/` | Home | All photos (unchanged aside from layout) |
| `/albums` | AlbumList | Album card grid |
| `/albums/:id` | AlbumDetail | Photos within an album |

### Layout — Sidebar

A persistent sidebar replaces the current header-only layout:

```
┌─────────┬─────────────────────────────┐
│ 全部照片 │                             │
│         │     <router-view />         │
│ 相册    │                             │
└─────────┴─────────────────────────────┘
```

- Sidebar: fixed width (~200px), dark background, two nav items
- Active item highlighted
- Existing header ("ShutterSeek / N photos") removed or moved into sidebar

### PhotoGrid (reusable component)

Extracted from the current Home.vue. Encapsulates:

- CSS grid layout (responsive columns)
- Infinite scroll (IntersectionObserver + AbortController + dynamic batch size)
- Lightbox with zoom/pan/EXIF sidebar
- Keyboard left/right navigation (within currently loaded photos)

```ts
defineProps<{
  fetchFn: (
    params: { limit: number; cursor?: string },
    signal?: AbortSignal
  ) => Promise<PhotoListResponse>
}>()
```

**Home.vue** becomes:
```html
<PhotoGrid :fetch-fn="fetchPhotos" />
```

**AlbumDetail.vue** becomes:
```html
<PhotoGrid :fetch-fn="(params, signal) => fetchAlbumPhotos(albumId, params, signal)" />
```

### AlbumList

Card grid layout:
- Each card: cover thumbnail (aspect-square, object-cover), album title, photo count
- Click → navigate to `/albums/:id`
- Responsive columns matching the photo grid

## Implementation Order

1. Database: run SQL to create `albums` and `album_photos` tables, populate initial data
2. Backend: model → service → handler → router (register new routes)
3. Frontend: PhotoGrid extraction → AlbumList → AlbumDetail → sidebar layout → router update
4. Verify end-to-end
