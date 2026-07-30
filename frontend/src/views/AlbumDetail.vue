<template>
  <div>
    <header class="sticky top-0 z-10 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-2 flex items-center gap-3">
      <button @click="$router.push('/albums')" class="text-neutral-400 hover:text-white text-lg">←</button>
      <div>
        <h1 class="text-sm font-medium text-white">{{ album?.title || 'Album' }}</h1>
        <p class="text-xs text-neutral-500">{{ album?.photo_count?.toLocaleString() || 0 }} photos</p>
      </div>
    </header>
    <PhotoGrid :key="albumId" :fetch-fn="wrapFetch" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchAlbum, fetchAlbumPhotos, type Album } from '@/api/albums'

const route = useRoute()
const albumId = Number(route.params.id)
const album = ref<Album | null>(null)

fetchAlbum(albumId).then(a => { album.value = a })

watch(() => route.params.id, (id) => {
  album.value = null
  fetchAlbum(Number(id)).then(a => { album.value = a })
})

function wrapFetch(params: { limit: number; cursor?: string }, signal?: AbortSignal) {
  return fetchAlbumPhotos(albumId, params, signal)
}
</script>
