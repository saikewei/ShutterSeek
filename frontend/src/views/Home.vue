<template>
  <div>
    <button
      v-if="isAdmin"
      class="fixed top-3 right-3 z-40 text-xs px-3 py-1.5 rounded bg-neutral-800 hover:bg-neutral-700 transition-colors"
      @click="uploadOpen = true"
    >上传</button>
    <PhotoGrid ref="gridRef" :fetch-fn="wrapFetch" :album-titles="albumTitles" :range-fn="rangeFn" />
    <UploadDialog :open="uploadOpen" @close="uploadOpen = false" @done="gridRef?.reload()" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import PhotoGrid from '@/components/PhotoGrid.vue'
import UploadDialog from '@/components/UploadDialog.vue'
import { fetchPhotos, fetchPhotoRange } from '@/api/photos'
import { fetchAlbums } from '@/api/albums'
import { isAdmin } from '@/stores/auth'

const albumTitles = ref<Record<number, string>>({})
const uploadOpen = ref(false)
const gridRef = ref<InstanceType<typeof PhotoGrid> | null>(null)

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
