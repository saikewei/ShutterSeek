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
  head_count?: number
}

export interface PhotoListParams {
  limit?: number
  cursor?: string
  newer_t?: string
  newer_id?: number
  album_id?: string
  with_albums?: boolean
  month?: string
  date?: string
}

export async function fetchPhotos(params?: PhotoListParams, signal?: AbortSignal): Promise<PhotoListResponse> {
  const { data } = await api.get<PhotoListResponse>('/photos', {
    params: params ? {
      limit: params.limit,
      cursor: params.cursor || undefined,
      newer_t: params.newer_t || undefined,
      newer_id: params.newer_id ?? undefined,
      album_id: params.album_id || undefined,
      with_albums: params.with_albums ? 'true' : undefined,
      month: params.month || undefined,
      date: params.date || undefined,
    } : undefined,
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

export async function fetchPhotoRange(params: {
  from_id: number
  to_id: number
  album_id?: string
  month?: string
}): Promise<{ photo_ids: number[]; count: number }> {
  const { data } = await api.get<{ photo_ids: number[]; count: number }>('/photos/range', {
    params: {
      from_id: params.from_id,
      to_id: params.to_id,
      album_id: params.album_id || undefined,
      month: params.month || undefined,
    },
  })
  return data
}

export interface UploadResult {
  id: number
  file_path: string
  taken_at: string
  width: number
  height: number
  thumbnail_url: string
  duplicate: boolean
  existing_id?: number
}

export async function uploadPhoto(file: File, vector: number[]): Promise<UploadResult> {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('vector', JSON.stringify(vector))
  const { data } = await api.post<UploadResult>('/photos/upload', fd, { timeout: 120000 })
  return data
}
