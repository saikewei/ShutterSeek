<template>
  <div>
    <!-- Date grouping toggle -->
    <div class="sticky top-0 z-20 flex items-center gap-1 px-2 py-2 bg-neutral-950/80 backdrop-blur border-b border-neutral-800">
      <button
        v-for="opt in [{k:'day',l:'按日'},{k:'month',l:'按月'}]"
        :key="opt.k"
        @click="groupBy = opt.k as 'day'|'month'"
        :class="groupBy === opt.k
          ? 'bg-neutral-700 text-white'
          : 'text-neutral-500 hover:text-neutral-300'"
        class="px-3 py-1 text-xs rounded-full transition-colors"
      >{{ opt.l }}</button>
    </div>

    <!-- Grid with sticky date separators -->
    <div v-for="group in groups" :key="group.label">
      <div
        class="sticky z-10 bg-neutral-950/95 backdrop-blur px-2 py-2 text-sm font-semibold tracking-wide border-b border-neutral-800"
        style="top: 37px"
      >
        <span class="border-l-2 border-neutral-500 pl-2.5 text-neutral-200">{{ group.label }}</span>
      </div>
      <div class="grid grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-1 p-1">
        <div
          v-for="photo in group.photos"
          :key="photo.id"
          class="group cursor-pointer relative rounded-lg overflow-hidden bg-neutral-800"
          @click="openLightbox(photo)"
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
          <slot name="photo-action" :photo="photo" />
        </div>
      </div>
    </div>

    <div ref="sentinel" class="py-12 text-center text-neutral-500 text-sm">
      <span v-if="loading && photos.length === 0">Loading...</span>
      <span v-else-if="!hasMore">— End of {{ total.toLocaleString() }} photos —</span>
    </div>

    <Lightbox
      :open="lightbox.open"
      :photo="lightbox.photo"
      :has-prev="lightboxIdx > 0"
      :has-next="lightboxIdx < photos.length - 1 || hasMore"
      @close="lightbox.open = false"
      @prev="lightboxPrev"
      @next="lightboxNext"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, reactive } from 'vue'
import type { Photo, PhotoListResponse } from '@/api/photos'
import { THUMB_BASE } from '@/api/client'
import Lightbox from '@/components/Lightbox.vue'

const props = defineProps<{
  fetchFn: (
    params: { limit: number; cursor?: string },
    signal?: AbortSignal
  ) => Promise<PhotoListResponse>
}>()

const photos = ref<Photo[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const sentinel = ref<HTMLElement | null>(null)
const groupBy = ref<'day' | 'month'>('day')
let cursor = ''
let observer: IntersectionObserver | null = null
let controller: AbortController | null = null
let wasInterrupted = false

// ── Date grouping ────────────────────────────────────
interface Group { label: string; photos: Photo[] }

const groups = computed<Group[]>(() => {
  const result: Group[] = []
  for (const p of photos.value) {
    const label = dateLabel(p.taken_at, groupBy.value)
    const last = result[result.length - 1]
    if (last && last.label === label) {
      last.photos.push(p)
    } else {
      result.push({ label, photos: [p] })
    }
  }
  return result
})

function dateLabel(iso: string, mode: 'day' | 'month'): string {
  if (!iso) return '未标注日期'
  const d = new Date(iso)
  const y = d.getFullYear()
  const m = d.getMonth() + 1
  if (mode === 'month') return `${y}年${m}月`
  return `${y}年${m}月${d.getDate()}日`
}

// ── Lightbox ────────────────────────────────────────
const lightbox = reactive({ open: false, photo: null as Photo | null })
let lightboxIdx = 0

function openLightbox(photo: Photo) {
  lightbox.photo = photo
  lightboxIdx = photos.value.indexOf(photo)
  lightbox.open = true
}

function lightboxPrev() {
  if (lightboxIdx > 0) {
    lightboxIdx--
    lightbox.photo = photos.value[lightboxIdx]
  }
}

function lightboxNext() {
  if (lightboxIdx < photos.value.length - 1) {
    lightboxIdx++
    lightbox.photo = photos.value[lightboxIdx]
  }
}

function onKeyDown(e: KeyboardEvent) {
  if (!lightbox.open) return
  if (e.key === 'ArrowLeft') { e.preventDefault(); lightboxPrev() }
  if (e.key === 'ArrowRight') { e.preventDefault(); lightboxNext() }
}

// ── Dynamic batch size based on viewport ────────────
function calcLimit(): number {
  const w = window.innerWidth
  const cols = w >= 1280 ? 5 : w >= 1024 ? 4 : w >= 768 ? 3 : 2
  const cellSize = w / cols
  const visibleRows = Math.ceil(window.innerHeight / cellSize)
  return Math.max(30, cols * visibleRows * 3)
}

// ── Fetch page with abort support ───────────────────
async function loadPage() {
  if (!hasMore.value) return

  if (loading.value) wasInterrupted = true

  controller?.abort()
  controller = new AbortController()
  const { signal } = controller

  const limit = wasInterrupted ? 200 : calcLimit()
  wasInterrupted = false

  loading.value = true
  try {
    const data = await props.fetchFn(
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
  window.addEventListener('keydown', onKeyDown)
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
  window.removeEventListener('keydown', onKeyDown)
  observer?.disconnect()
  controller?.abort()
})
</script>
