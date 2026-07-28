import { api } from './client'

export interface Photo {
  id: number
  thumbnail_url: string
  camera_make: string
  camera_model: string
  lens_model: string
  focal_length: string
  aperture: string
  iso: number
  taken_at: string
  width: number
  height: number
}

export interface PhotoListResponse {
  items: Photo[]
  next_cursor: string
  total: number
}

export interface PhotoListParams {
  limit?: number
  cursor?: string
}

export async function fetchPhotos(params?: PhotoListParams): Promise<PhotoListResponse> {
  const { data } = await api.get<PhotoListResponse>('/photos', { params })
  return data
}
