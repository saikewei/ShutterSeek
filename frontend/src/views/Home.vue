<template>
  <div>
    <PhotoGrid ref="gridRef" :fetch-fn="wrapFetch" :album-titles="albumTitles" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchPhotos } from '@/api/photos'
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
  params: { limit: number; cursor?: string; album_id?: string; with_albums?: boolean },
  signal?: AbortSignal
) {
  return fetchPhotos(params, signal)
}
</script>
