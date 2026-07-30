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
      <template #exif-extra="{ photo }">
        <div class="border-t border-white/10 pt-2 mt-2">
          <button
            v-if="removeTarget?.id !== photo.id"
            @click="removeTarget = photo"
            class="text-xs text-neutral-500 hover:text-red-400 transition-colors"
          >从相册移除…</button>
          <div v-else class="flex items-center gap-2">
            <span class="text-xs text-red-400">确认移除？</span>
            <button @click="doRemove(photo)" class="text-xs px-2 py-0.5 rounded bg-red-600 text-white hover:bg-red-500">确认</button>
            <button @click="removeTarget = null" class="text-xs text-neutral-400 hover:text-white">取消</button>
          </div>
        </div>
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
const removeTarget = ref<Photo | null>(null)

function loadAlbum() {
  album.value = null
  fetchAlbum(albumId).then(a => { album.value = a })
}
loadAlbum()

watch(() => route.params.id, () => { loadAlbum() })

function wrapFetch(params: { limit: number; cursor?: string }, signal?: AbortSignal) {
  return fetchAlbumPhotos(albumId, params, signal)
}

async function doRemove(photo: Photo) {
  await removeAlbumPhoto(albumId, photo.id)
  removeTarget.value = null
  loadAlbum()
}
</script>
