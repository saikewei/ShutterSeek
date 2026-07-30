import { api } from './client'

export interface Photo {
  id: number
  thumbnail_url: string
  file_name: string
  file_path: string
  camera_make: string
  camera_model: string
  lens_model: string
  focal_length: string
  aperture: string
  iso: number
  taken_at: string
  width: number
  height: number
  album_ids?: number[]
}

export interface PhotoListResponse {
  items: Photo[]
  next_cursor: string
  total: number
}

export interface PhotoListParams {
  limit?: number
  cursor?: string
  album_id?: string
  with_albums?: boolean
}

export async function fetchPhotos(params?: PhotoListParams, signal?: AbortSignal): Promise<PhotoListResponse> {
  const { data } = await api.get<PhotoListResponse>('/photos', {
    params: params ? { ...params, with_albums: params.with_albums ? 'true' : undefined, album_id: params.album_id || undefined } : undefined,
    signal
  })
  return data
}

export interface DateCount {
  date: string
  count: number
}

export async function fetchPhotoDates(signal?: AbortSignal): Promise<DateCount[]> {
  const { data } = await api.get<DateCount[]>('/photos/dates', { signal })
  return data
}
