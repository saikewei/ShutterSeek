<template>
  <div>
    <header class="sticky top-0 z-30 bg-canvas/80 backdrop-blur border-b border-line px-4 py-2 flex items-center gap-3">
      <button @click="$router.push('/albums')" class="text-ink-3 hover:text-ink text-lg transition-colors">←</button>
      <div class="flex-1">
        <h1 class="text-sm font-medium font-display text-ink">{{ album?.title || 'Album' }}</h1>
        <p class="text-xs text-ink-3">{{ album?.photo_count?.toLocaleString() || 0 }} photos</p>
      </div>
      <button
        @click="$router.push('/search?album_id=' + albumId)"
        class="shrink-0 text-xs text-ink-2 hover:text-ink border border-line-strong rounded-lg px-3 py-1.5 transition-colors"
      >搜索相册</button>
    </header>

    <PhotoGrid
      :key="albumId"
      ref="gridRef"
      :fetch-fn="wrapFetch"
      :dates-fn="albumDatesFn"
      :album-titles="albumTitles"
      :sticky-offset="53"
      :range-fn="rangeFn"
      :remove-from-album-id="albumId"
      @photo-contextmenu="(photo, event) => isAdmin && onContextMenu(photo, event)"
      @removed-from-album="refreshAlbum"
      @upload="uploadOpen = true"
    />
    <UploadDialog :open="uploadOpen" @close="uploadOpen = false" @done="onUploadDone" />

    <!-- Right-click context menu -->
    <Teleport to="body">
      <div v-if="ctxMenu.show" class="fixed inset-0 z-50" @click="ctxMenu.show = false" @contextmenu.prevent="ctxMenu.show = false">
        <div class="absolute bg-raised border border-line rounded-lg py-1 shadow-xl w-40"
          :style="{ top: ctxMenu.y + 'px', left: ctxMenu.x + 'px' }">
          <button @click.stop="setAsCover" class="w-full text-left px-3 py-2 text-sm text-ink-2 hover:bg-surface hover:text-ink transition-colors">设为封面</button>
          <template v-if="!ctxMenu.confirming">
            <button @click.stop="ctxMenu.confirming = true" class="w-full text-left px-3 py-2 text-sm text-danger-ink hover:bg-surface transition-colors">从相册移除</button>
          </template>
          <div v-else class="px-3 py-2">
            <p class="text-xs text-ink-2 mb-2">确认移除？</p>
            <div class="flex gap-2">
              <button @click.stop="doRemove" class="px-2 py-0.5 text-xs rounded bg-danger text-white hover:bg-[#D06A6A]">确认</button>
              <button @click.stop="ctxMenu.show = false" class="px-2 py-0.5 text-xs text-ink-3 hover:text-ink">取消</button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRoute } from 'vue-router'
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchAlbum, fetchAlbumDates, updateAlbum, removeAlbumPhoto } from '@/api/albums'
import { fetchPhotos, fetchPhotoRange } from '@/api/photos'
import type { Photo } from '@/api/photos'
import UploadDialog from '@/components/UploadDialog.vue'
import { isAdmin } from '@/stores/auth'

const route = useRoute()
const albumId = Number(route.params.id)
const album = ref<any>(null)
const gridRef = ref<InstanceType<typeof PhotoGrid> | null>(null)
const albumTitles: Record<number, string> = {}
const uploadOpen = ref(false)

function onUploadDone() {
  gridRef.value?.reload()
  refreshAlbum()
}

// Load album info
function refreshAlbum() {
  fetchAlbum(albumId).then(a => {
    album.value = a
    albumTitles[a.id] = a.title
  })
}
refreshAlbum()

const ctxMenu = reactive<{
  show: boolean; x: number; y: number; photo: Photo | null; confirming: boolean
}>({ show: false, x: 0, y: 0, photo: null, confirming: false })

function albumDatesFn() {
  return fetchAlbumDates(albumId)
}

function wrapFetch(params: any, signal?: AbortSignal) {
  return fetchPhotos({ ...params, album_id: String(albumId) }, signal)
}

function rangeFn(fromId: number, toId: number, opts?: { album_id?: string }) {
  return fetchPhotoRange({ from_id: fromId, to_id: toId, album_id: opts?.album_id ?? String(albumId) }).then(r => r.photo_ids)
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
  fetchAlbum(albumId).then(a => { album.value = a })
}

async function setAsCover() {
  if (!ctxMenu.photo) return
  await updateAlbum(albumId, { cover_photo_id: ctxMenu.photo.id })
  ctxMenu.show = false
  fetchAlbum(albumId).then(a => { album.value = a })
}
</script>
