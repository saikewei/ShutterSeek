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
            :src="THUMB_BASE + '/' + photo.id + '.jpg'"
            :alt="photo.camera_model || 'Photo'"
            loading="lazy"
            :class="['w-full aspect-square object-cover', photo.height > photo.width ? 'rotate-90 scale-150' : '']"
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
      <span v-if="loading">Loading...</span>
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

async function loadPage() {
  if (loading.value || !hasMore.value) return
  loading.value = true
  try {
    const data = await fetchPhotos({ limit: 30, cursor: cursor || undefined })
    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''
    console.log('loadPage:', { received: data.items.length, cursor, hasMore: hasMore.value, total: data.total })
  } catch (e) {
    console.error('load failed', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadPage()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting && hasMore.value && !loading.value) {
        console.log('observer triggered, loading more...')
        loadPage()
      }
    },
    { rootMargin: '400px' }
  )
  // Observe after next tick so DOM is painted
  setTimeout(() => {
    if (sentinel.value) observer?.observe(sentinel.value)
  }, 1000)
})

function onImgError(photo: Photo) {
  const url = photo.thumbnail_url
  photo.thumbnail_url = ''
  setTimeout(() => { photo.thumbnail_url = url }, 2000)
}

onUnmounted(() => observer?.disconnect())
</script>
