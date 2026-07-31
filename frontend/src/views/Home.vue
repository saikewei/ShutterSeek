<template>
  <div>
    <PhotoGrid ref="gridRef" :fetch-fn="wrapFetch" :album-titles="albumTitles" :range-fn="rangeFn" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchPhotos, fetchPhotoRange } from '@/api/photos'
import { fetchAlbums } from '@/api/albums'

const albumTitles = ref<Record<number, string>>({})

onMounted(async () => {
  try {
    const data = await fetchAlbums()
    const map: Record<number, string> = {}
    for (const a of data.items) map[a.id] = a.title
    albumTitles.value = map
  } catch { /* no album titles */ }
})

function wrapFetch(
  params: { limit: number; cursor?: string; newer_t?: string; newer_id?: number; album_id?: string; with_albums?: boolean; month?: string },
  signal?: AbortSignal
) {
  return fetchPhotos(params, signal)
}

function rangeFn(fromId: number, toId: number, opts?: { album_id?: string }) {
  return fetchPhotoRange({ from_id: fromId, to_id: toId, album_id: opts?.album_id }).then(r => r.photo_ids)
}
</script>
