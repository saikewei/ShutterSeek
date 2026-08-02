import { api } from './client'
import type { Photo } from './photos'

export interface SearchPhotoItem extends Photo {
  score: number
}

export interface SearchResponse {
  items: SearchPhotoItem[]
  total: number
}

export async function fetchSearch(
  params: { q: string; album_id?: number; limit?: number },
  signal?: AbortSignal,
): Promise<SearchResponse> {
  const { data } = await api.get<SearchResponse>('/search', {
    params: {
      q: params.q,
      album_id: params.album_id || undefined,
      limit: params.limit || undefined,
    },
    signal,
  })
  return data
}
