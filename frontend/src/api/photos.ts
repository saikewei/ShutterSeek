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

/** 一次批量请求里的一项。preview 是客户端生成的缩略图源（长边 ≤1080 的 JPEG），
 *  服务端拿它直接转 webp，省掉整张原图的解码；缺失时服务端自己兜底。 */
export interface BatchUploadItem {
  file: File
  vector: Float32Array | number[]
  preview?: Blob | null
}

export interface BatchUploadItemResult {
  index: number
  filename: string
  status: 'created' | 'duplicate' | 'error'
  id?: number
  file_path?: string
  taken_at?: string
  width?: number
  height?: number
  thumbnail_url?: string
  thumbnail?: boolean
  duplicate: boolean
  existing_id?: number
  error?: string
}

export interface BatchUploadResponse {
  results: BatchUploadItemResult[]
  summary: { created: number; duplicate: number; failed: number; elapsed_ms: number }
}

export interface UploadBatchOptions {
  signal?: AbortSignal
  /** 单文件批次才有意义：一次请求就是一张照片，进度可直接当该文件的进度 */
  onUploadProgress?: (loaded: number, total: number) => void
  timeoutMs?: number
}

/** 批量上传：multipart 字段带下标（file_N / vector_N / preview_N），
 *  服务端按下标归位，因此与发送顺序无关。 */
export async function uploadBatch(
  items: BatchUploadItem[],
  opts: UploadBatchOptions = {}
): Promise<BatchUploadResponse> {
  const fd = new FormData()
  items.forEach((it, i) => {
    fd.append(`file_${i}`, it.file, it.file.name)
    fd.append(`vector_${i}`, JSON.stringify(Array.from(it.vector)))
    if (it.preview) fd.append(`preview_${i}`, it.preview, 'preview.jpg')
  })
  const { data } = await api.post<BatchUploadResponse>('/photos/upload/batch', fd, {
    timeout: opts.timeoutMs ?? 300000,
    signal: opts.signal,
    onUploadProgress: opts.onUploadProgress
      ? (e) => opts.onUploadProgress!(e.loaded, e.total ?? 0)
      : undefined,
  })
  return data
}
