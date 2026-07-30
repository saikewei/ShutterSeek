<template>
  <div>
    <header class="sticky top-0 z-10 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-2 flex items-center gap-3">
      <button @click="$router.push('/albums')" class="text-neutral-400 hover:text-white text-lg">←</button>
      <div>
        <h1 class="text-sm font-medium text-white">{{ album?.title || 'Album' }}</h1>
        <p class="text-xs text-neutral-500">{{ album?.photo_count?.toLocaleString() || 0 }} photos</p>
      </div>
    </header>
    <PhotoGrid :key="albumId" :fetch-fn="wrapFetch">
      <template #photo-action="{ photo }">
        <button
          class="absolute top-1 right-1 w-6 h-6 rounded-full bg-red-600/80 text-white text-xs flex items-center justify-center opacity-0 group-hover:opacity-100 hover:bg-red-500 transition-all"
          @click.stop="removePhoto(photo)"
        >✕</button>
      </template>
    </PhotoGrid>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchAlbum, fetchAlbumPhotos, removeAlbumPhoto, type Album } from '@/api/albums'
import type { Photo } from '@/api/photos'

const route = useRoute()
const albumId = Number(route.params.id)
const album = ref<Album | null>(null)

function loadAlbum() {
  album.value = null
  fetchAlbum(albumId).then(a => { album.value = a })
}
loadAlbum()

watch(() => route.params.id, () => { loadAlbum() })

function wrapFetch(params: { limit: number; cursor?: string }, signal?: AbortSignal) {
  return fetchAlbumPhotos(albumId, params, signal)
}

async function removePhoto(photo: Photo) {
  await removeAlbumPhoto(albumId, photo.id)
  loadAlbum()
}
</script>
