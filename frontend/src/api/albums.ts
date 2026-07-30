import { api } from './client'
import type { PhotoListResponse } from './photos'

export interface Album {
  id: number
  title: string
  description: string
  cover_url: string
  photo_count: number
  sort_order: number
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
