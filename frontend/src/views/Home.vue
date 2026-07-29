<template>
  <div class="min-h-screen bg-neutral-950 text-white">
    <header class="sticky top-0 z-10 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-3">
      <h1 class="text-lg font-semibold tracking-wide">ShutterSeek</h1>
      <p class="text-xs text-neutral-500">{{ total.toLocaleString() }} photos</p>
    </header>

    <div class="p-2">
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-2">
        <div
          v-for="photo in photos"
          :key="photo.id"
          class="group cursor-pointer relative rounded-lg overflow-hidden bg-neutral-800"
        >
          <img
            :src="THUMB_BASE + '/' + photo.id + '.webp'"
            :alt="photo.camera_model || 'Photo'"
            loading="lazy"
            :class="['w-full aspect-square object-cover', photo.height > photo.width ? 'rotate-270 scale-150' : '']"
            @error="onImgError(photo)"
          />
          <div class="absolute inset-x-0 bottom-0 p-2 bg-gradient-to-t from-black/70 to-transparent opacity-0 group-hover:opacity-100 transition-opacity">
            <p class="text-xs truncate text-neutral-400">{{ photo.file_name }}</p>
            <p class="text-xs truncate">{{ photo.camera_make }} {{ photo.camera_model }}</p>
            <p class="text-xs text-neutral-300">{{ photo.focal_length }} {{ photo.aperture }} ISO{{ photo.iso }}</p>
            <p class="text-xs text-neutral-400">{{ photo.width }}×{{ photo.height }}</p>
          </div>
        </div>
      </div>
    </div>

    <div ref="sentinel" class="py-12 text-center text-neutral-500 text-sm">
      <span v-if="loading && photos.length === 0">Loading...</span>
      <span v-else-if="!hasMore">— End of {{ total.toLocaleString() }} photos —</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { fetchPhotos, type Photo } from '@/api/photos'
import { THUMB_BASE } from '@/api/client'

const photos = ref<Photo[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const sentinel = ref<HTMLElement | null>(null)
let cursor = ''
let observer: IntersectionObserver | null = null
let controller: AbortController | null = null
let wasInterrupted = false // user scrolled past before page finished

// ── Dynamic batch size based on viewport ────────────
function calcLimit(): number {
  const w = window.innerWidth
  const cols = w >= 1280 ? 5 : w >= 1024 ? 4 : w >= 768 ? 3 : 2
  // Approximate image height = grid cell width (aspect-square)
  const cellSize = w / cols
  const visibleRows = Math.ceil(window.innerHeight / cellSize)
  return Math.max(30, cols * visibleRows * 3) // 3 viewports worth
}

// ── Fetch page with abort support ───────────────────
async function loadPage() {
  if (!hasMore.value) return

  // User scrolled past before previous page finished — use max batch next time
  if (loading.value) wasInterrupted = true

  controller?.abort()
  controller = new AbortController()
  const { signal } = controller

  // Bump to max batch if user is scrolling faster than we can load
  const limit = wasInterrupted ? 200 : calcLimit()
  wasInterrupted = false

  loading.value = true
  try {
    const data = await fetchPhotos(
      { limit, cursor: cursor || undefined },
      signal
    )
    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''
  } catch (e: any) {
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    console.error('load failed', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadPage()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting && hasMore.value && !loading.value) loadPage()
    },
    { rootMargin: `${window.innerHeight * 2}px` }
  )
  setTimeout(() => {
    if (sentinel.value) observer?.observe(sentinel.value)
  }, 1000)
})

function onImgError(photo: Photo) {
  const url = photo.thumbnail_url
  photo.thumbnail_url = ''
  setTimeout(() => { photo.thumbnail_url = url }, 2000)
}

onUnmounted(() => {
  observer?.disconnect()
  controller?.abort()
})
</script>
