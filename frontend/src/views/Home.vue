<template>
  <div class="min-h-screen bg-neutral-950 text-white">
    <header class="sticky top-0 z-10 bg-neutral-950/80 backdrop-blur border-b border-neutral-800 px-4 py-3">
      <h1 class="text-lg font-semibold tracking-wide">ShutterSeek</h1>
      <p class="text-xs text-neutral-500">{{ total.toLocaleString() }} photos</p>
    </header>

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
            loading="lazy"
            class="w-full rounded-lg object-cover bg-neutral-800 transition-transform group-hover:scale-[1.02]"
            @error="onImgError(photo)"
          />
          <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/70 to-transparent rounded-b-lg opacity-0 group-hover:opacity-100 transition-opacity">
            <p class="text-xs truncate">{{ photo.camera_make }} {{ photo.camera_model }}</p>
            <p class="text-xs text-neutral-300">{{ photo.focal_length }} {{ photo.aperture }} ISO{{ photo.iso }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- sentinel 放在 columns 外面，确保 IntersectionObserver 正确触发 -->
    <div ref="sentinel" class="py-12 text-center text-neutral-500 text-sm">
      <span v-if="loading">Loading...</span>
      <span v-else-if="!hasMore">— End of 74,577 photos —</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { fetchPhotos, type Photo } from '@/api/photos'

const photos = ref<Photo[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const sentinel = ref<HTMLElement | null>(null)
let cursor = ''
let observer: IntersectionObserver | null = null

async function loadPage() {
  if (loading.value || !hasMore.value) return
  loading.value = true
  try {
    const data = await fetchPhotos({ limit: 30, cursor: cursor || undefined })
    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''
  } catch (e) {
    console.error('load photos failed', e)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadPage()
  // Observe sentinel only after first page loads so it's below the fold
  if (sentinel.value) {
    observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore.value && !loading.value) loadPage()
      },
      { rootMargin: '200px' }
    )
    observer.observe(sentinel.value)
  }
})

function onImgError(photo: Photo) {
  // Retry once after 1s — often recovers from transient network issues
  const url = photo.thumbnail_url
  photo.thumbnail_url = ''
  setTimeout(() => { photo.thumbnail_url = url }, 1000)
}

onUnmounted(() => observer?.disconnect())
</script>
