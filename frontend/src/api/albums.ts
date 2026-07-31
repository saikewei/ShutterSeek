import { api } from './client'
import type { PhotoListResponse } from './photos'

export interface Album {
  id: number
  title: string
  description: string
  cover_url: string
  photo_count: number
  sort_order: number
  is_public: boolean
  created_at: string
  updated_at: string
}

export interface AlbumListResponse {
  items: Album[]
  total: number
}

export async function fetchAlbums(signal?: AbortSignal): Promise<AlbumListResponse> {
  const { data } = await api.get<AlbumListResponse>('/albums', { signal })
  return data
}

export async function fetchAlbum(id: number, signal?: AbortSignal): Promise<Album> {
  const { data } = await api.get<Album>(`/albums/${id}`, { signal })
  return data
}

export async function fetchAlbumPhotos(
  id: number,
  params?: { limit?: number; cursor?: string },
  signal?: AbortSignal
): Promise<PhotoListResponse> {
  const { data } = await api.get<PhotoListResponse>(`/albums/${id}/photos`, { params, signal })
  return data
}

export async function createAlbum(title: string, description?: string, isPublic = false): Promise<Album> {
  const { data } = await api.post<Album>('/albums', { title, description: description || '', is_public: isPublic })
  return data
}

export async function updateAlbum(
  id: number,
  fields: { title?: string; description?: string; cover_photo_id?: number | null; is_public?: boolean }
): Promise<Album> {
  const { data } = await api.put<Album>(`/albums/${id}`, fields)
  return data
}

export async function deleteAlbum(id: number): Promise<void> {
  await api.delete(`/albums/${id}`)
}

export async function removeAlbumPhoto(albumId: number, photoId: number): Promise<void> {
  await api.delete(`/albums/${albumId}/photos/${photoId}`)
}

export async function removeAlbumPhotos(albumId: number, photoIds: number[]): Promise<{ removed: number }> {
  const { data } = await api.delete<{ removed: number }>(`/albums/${albumId}/photos`, {
    data: { photo_ids: photoIds },
  })
  return data
}

export async function fetchAlbumDates(albumId: number): Promise<Array<{ date: string; count: number }>> {
  const { data } = await api.get(`/albums/${albumId}/dates`)
  return data
}

export interface BatchAddResult {
  added: number
  skipped: number
}

export async function batchAddPhotos(albumId: number, photoIds: number[]): Promise<BatchAddResult> {
  const { data } = await api.post<BatchAddResult>(`/albums/${albumId}/photos`, { photo_ids: photoIds })
  return data
}
