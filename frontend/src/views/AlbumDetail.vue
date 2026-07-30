<template>
  <div>
    <header class="sticky top-0 z-30 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-2 flex items-center gap-3">
      <button @click="$router.push('/albums')" class="text-neutral-400 hover:text-white text-lg">←</button>
      <div class="flex-1">
        <h1 class="text-sm font-medium text-white">{{ album?.title || 'Album' }}</h1>
        <p class="text-xs text-neutral-500">{{ album?.photo_count?.toLocaleString() || 0 }} photos</p>
      </div>
    </header>

    <PhotoGrid ref="gridRef" :fetch-fn="wrapFetch" :sticky-offset="53" @photo-contextmenu="onContextMenu" />

    <!-- Right-click context menu -->
    <Teleport to="body">
      <div v-if="ctxMenu.show" class="fixed inset-0 z-50" @click="ctxMenu.show = false" @contextmenu.prevent="ctxMenu.show = false">
        <div class="absolute bg-neutral-800 border border-neutral-700 rounded-lg py-1 shadow-xl w-40"
          :style="{ top: ctxMenu.y + 'px', left: ctxMenu.x + 'px' }">
          <button @click.stop="setAsCover" class="w-full text-left px-3 py-2 text-sm text-neutral-300 hover:bg-neutral-700 hover:text-white transition-colors">设为封面</button>

          <template v-if="!ctxMenu.confirming">
            <button @click.stop="ctxMenu.confirming = true" class="w-full text-left px-3 py-2 text-sm text-red-400 hover:bg-neutral-700 transition-colors">从相册移除</button>
          </template>
          <div v-else class="px-3 py-2">
            <p class="text-xs text-neutral-400 mb-2">确认移除？</p>
            <div class="flex gap-2">
              <button @click.stop="doRemove" class="px-2 py-0.5 text-xs rounded bg-red-600 text-white hover:bg-red-500">确认</button>
              <button @click.stop="ctxMenu.show = false" class="px-2 py-0.5 text-xs text-neutral-400 hover:text-white">取消</button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, reactive } from 'vue'
import { useRoute } from 'vue-router'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchAlbum, fetchAlbumPhotos, removeAlbumPhoto, updateAlbum, type Album } from '@/api/albums'
import type { Photo } from '@/api/photos'

const route = useRoute()
const albumId = Number(route.params.id)
const album = ref<Album | null>(null)
const gridRef = ref<InstanceType<typeof PhotoGrid> | null>(null)

const ctxMenu = reactive<{
  show: boolean; x: number; y: number; photo: Photo | null; confirming: boolean
}>({ show: false, x: 0, y: 0, photo: null, confirming: false })

function loadAlbum() {
  album.value = null
  fetchAlbum(albumId).then(a => { album.value = a })
}
loadAlbum()

watch(() => route.params.id, () => { loadAlbum() })

function wrapFetch(
  params: { limit: number; cursor?: string; album_id?: string; with_albums?: boolean },
  signal?: AbortSignal
) {
  return fetchAlbumPhotos(albumId, params, signal)
}

function onContextMenu(photo: Photo, event: MouseEvent) {
  ctxMenu.show = true; ctxMenu.x = event.clientX; ctxMenu.y = event.clientY
  ctxMenu.photo = photo; ctxMenu.confirming = false
}

async function doRemove() {
  if (!ctxMenu.photo) return
  const photoId = ctxMenu.photo.id
  await removeAlbumPhoto(albumId, photoId)
  ctxMenu.show = false
  gridRef.value?.removePhotoById(photoId)
  loadAlbum()
}

async function setAsCover() {
  if (!ctxMenu.photo) return
  await updateAlbum(albumId, { cover_photo_id: ctxMenu.photo.id })
  ctxMenu.show = false
  loadAlbum()
}
</script>
