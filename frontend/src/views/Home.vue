<template>
  <div class="min-h-screen bg-neutral-950 text-white">
    <!-- 头部 -->
    <header class="sticky top-0 z-10 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-3">
      <h1 class="text-lg font-semibold tracking-wide">ShutterSeek</h1>
      <p class="text-xs text-neutral-500">{{ total.toLocaleString() }} photos</p>
    </header>

    <!-- 照片瀑布 -->
    <div class="p-2">
      <div class="columns-2 md:columns-3 lg:columns-4 gap-2 space-y-2">
        <div
          v-for="photo in photos"
          :key="photo.id"
          class="break-inside-avoid group cursor-pointer relative"
        >
          <img
            :src="photo.thumbnail_url"
            :alt="photo.camera_model || 'Photo'"
            :style="{ aspectRatio: photo.width + '/' + photo.height }"
            loading="lazy"
            class="w-full rounded-lg object-cover bg-neutral-800 transition-transform group-hover:scale-[1.02]"
          />
          <!-- EXIF 悬浮提示 -->
          <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/70 to-transparent rounded-b-lg opacity-0 group-hover:opacity-100 transition-opacity">
            <p class="text-xs truncate">{{ photo.camera_make }} {{ photo.camera_model }}</p>
            <p class="text-xs text-neutral-300">{{ photo.focal_length }} {{ photo.aperture }} ISO{{ photo.iso }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部状态 -->
    <div ref="sentinel" class="py-8 text-center text-neutral-500 text-sm">
      <span v-if="loading">Loading...</span>
      <span v-else-if="!hasMore">— End —</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

interface Photo {
  id: number
  thumbnail_url: string
  camera_make: string
  camera_model: string
  focal_length: string
  aperture: string
  iso: number
  width: number
  height: number
}

interface ListResponse {
  items: Photo[]
  next_cursor: string
  total: number
}

const photos = ref<Photo[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const sentinel = ref<HTMLElement | null>(null)
let cursor = ''
let observer: IntersectionObserver | null = null

// ── fetch page ────────────────────────────────────────
async function fetchPage() {
  if (loading.value || !hasMore.value) return
  loading.value = true

  const params = new URLSearchParams()
  params.set('limit', '30')
  if (cursor) params.set('cursor', cursor)

  try {
    const res = await fetch(`/api/v1/photos?${params}`)
    const data: ListResponse = await res.json()
    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''
  } catch (e) {
    console.error('fetch failed', e)
  } finally {
    loading.value = false
  }
}

// ── infinite scroll ───────────────────────────────────
onMounted(() => {
  fetchPage()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting && hasMore.value && !loading.value) {
        fetchPage()
      }
    },
    { rootMargin: '400px' } // 提前 400px 触发
  )
  if (sentinel.value) observer.observe(sentinel.value)
})

onUnmounted(() => {
  observer?.disconnect()
})
</script>
