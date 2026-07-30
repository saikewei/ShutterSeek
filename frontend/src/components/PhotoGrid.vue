<template>
  <div>
    <!-- Filter bar -->
    <div class="sticky z-20 flex items-center justify-between px-2 py-1.5 bg-neutral-950/90 backdrop-blur border-b border-neutral-800" :style="{ top: (stickyOffset || 0) + 'px' }">
      <div class="flex items-center gap-1.5">
        <button
          @click="filterOpen = true"
          class="relative px-3 py-1 text-xs rounded-full transition-colors"
          :class="hasActiveFilter ? 'bg-neutral-200 text-black' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'"
        >
          筛选
          <span v-if="hasActiveFilter" class="ml-1 text-[10px]">●</span>
        </button>

        <button
          v-if="!selectMode"
          @click="enterSelectMode"
          class="px-3 py-1 text-xs rounded-full bg-neutral-800 text-neutral-400 hover:bg-neutral-700 hover:text-white transition-colors"
        >选择</button>

        <button
          v-if="!selectMode && (!atTop || hasNewer)"
          @click="scrollToTop"
          class="px-3 py-1 text-xs rounded-full bg-neutral-700 text-neutral-300 hover:bg-neutral-600 hover:text-white transition-colors"
        >{{ hasNewer ? '返回最新' : '↑ 顶部' }}</button>

        <span v-if="selectMode" class="text-xs text-neutral-400">
          已选 {{ selected.size }} 张
        </span>
      </div>

      <div v-if="selectMode" class="flex items-center gap-1.5">
        <button @click="openAlbumPicker" class="px-3 py-1 text-xs rounded-full bg-white text-black font-medium hover:bg-neutral-200 transition-colors">
          添加到相册
        </button>
        <button @click="exitSelectMode" class="px-3 py-1 text-xs rounded-full text-neutral-400 hover:text-white transition-colors">取消</button>
      </div>
    </div>

    <!-- Grid with date separators -->
    <div v-for="group in groups" :key="group.label">
      <div class="sticky z-10 bg-neutral-950/95 backdrop-blur px-2 py-2 text-sm font-semibold tracking-wide border-b border-neutral-800" :style="{ top: (stickyOffset || 0) + 37 + 'px' }" :data-date="group.label">
        <span class="border-l-2 border-neutral-500 pl-2.5 text-neutral-200">{{ group.label }}</span>
      </div>
      <div class="grid grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-1 p-1">
        <div
          v-for="photo in group.photos"
          :key="photo.id"
          class="group cursor-pointer relative rounded-lg overflow-hidden bg-neutral-800"
          :class="{ 'ring-2 ring-white': selectMode && selected.has(photo.id) }"
          @click="onPhotoClick(photo)"
          @contextmenu.prevent="$emit('photoContextmenu', photo, $event)"
        >
          <img
            :src="THUMB_BASE + '/' + photo.id + '.webp'"
            :alt="photo.camera_model || 'Photo'"
            loading="lazy"
            :class="['w-full aspect-square object-cover', photo.height > photo.width ? 'rotate-270 scale-150' : '']"
            @error="onImgError(photo)"
          />

          <!-- Selection checkbox -->
          <div v-if="selectMode" class="absolute top-1.5 left-1.5">
            <div class="w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors"
              :class="selected.has(photo.id) ? 'bg-white border-white' : 'border-white/60 bg-black/30'"
            >
              <span v-if="selected.has(photo.id)" class="text-black text-xs">✓</span>
            </div>
          </div>

          <!-- Album tags -->
          <div v-if="!selectMode && albumTags(photo).length" class="absolute top-1 left-1 flex flex-wrap gap-0.5 max-w-[90%]">
            <span
              v-for="tag in albumTags(photo).slice(0, 2)"
              :key="tag"
              class="px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-neutral-300 truncate max-w-[80px]"
            >{{ tag }}</span>
            <span v-if="albumTags(photo).length > 2" class="text-[10px] text-neutral-500">+{{ albumTags(photo).length - 2 }}</span>
          </div>

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

    <div ref="sentinelTop" class="py-6 text-center text-neutral-500 text-sm">
      <span v-if="loadingNewer" class="text-xs">加载中...</span>
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
    >
      <template #exif-extra="{ photo }">
        <slot name="exif-extra" :photo="photo" />
      </template>
    </Lightbox>

    <!-- Filter dialog -->
    <Teleport to="body">
      <div v-if="filterOpen" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="filterOpen = false" />
        <div class="relative bg-neutral-800 rounded-xl p-5 w-72 shadow-xl border border-neutral-700">
          <h2 class="text-sm font-medium text-white mb-4">筛选设置</h2>

          <div class="space-y-4">
            <div>
              <label class="text-xs text-neutral-400 block mb-2">日期分组</label>
              <div class="flex gap-1">
                <button
                  v-for="opt in [{k:'day',l:'按日'},{k:'month',l:'按月'}]"
                  :key="opt.k"
                  @click="groupBy = opt.k as 'day'|'month'"
                  :class="groupBy === opt.k ? 'bg-neutral-600 text-white' : 'bg-neutral-700 text-neutral-400 hover:text-white'"
                  class="flex-1 py-1.5 text-xs rounded-lg transition-colors"
                >{{ opt.l }}</button>
              </div>
            </div>

            <label class="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" v-model="uncategorizedOnly" class="rounded accent-white" />
              <span class="text-xs text-neutral-300">仅显示未归类照片</span>
            </label>
          </div>

          <div class="flex justify-end mt-4">
            <button @click="filterOpen = false" class="px-4 py-1.5 text-xs rounded-full bg-white text-black font-medium hover:bg-neutral-200">关闭</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Date scrubber -->
    <DateScrubber :dates="datePoints" :active-month="activeMonth" @jump="jumpToDate" />

    <!-- Album picker dialog -->
    <Teleport to="body">
      <div v-if="albumPickerOpen" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="albumPickerOpen = false" />
        <div class="relative bg-neutral-800 rounded-xl p-5 w-80 shadow-xl border border-neutral-700 max-h-[70vh] flex flex-col">
          <h2 class="text-sm font-medium text-white mb-3">添加到相册</h2>

          <div class="flex-1 overflow-y-auto space-y-1 mb-3">
            <button
              v-for="album in albumList"
              :key="album.id"
              @click="doBatchAdd(album.id)"
              class="w-full text-left px-3 py-2 rounded-lg text-sm text-neutral-300 hover:bg-neutral-700 hover:text-white transition-colors flex justify-between"
            >
              <span>{{ album.title }}</span>
              <span class="text-xs text-neutral-500">{{ album.photo_count }}</span>
            </button>
          </div>

          <div v-if="addingResult" class="text-xs text-neutral-400 mb-2">
            {{ addingResult }}
          </div>

          <button @click="albumPickerOpen = false" class="text-xs text-neutral-500 hover:text-white self-end">关闭</button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, reactive, watch } from 'vue'
import type { Photo, PhotoListResponse } from '@/api/photos'
import { fetchPhotoDates } from '@/api/photos'
import { THUMB_BASE } from '@/api/client'
import { fetchAlbums, batchAddPhotos, type Album } from '@/api/albums'
import Lightbox from '@/components/Lightbox.vue'
import DateScrubber from '@/components/DateScrubber.vue'
import type { DatePoint } from '@/components/DateScrubber.vue'

const props = defineProps<{
  fetchFn: (
    params: { limit: number; cursor?: string; album_id?: string; with_albums?: boolean; month?: string },
    signal?: AbortSignal
  ) => Promise<PhotoListResponse>
  albumTitles?: Record<number, string>
  stickyOffset?: number
}>()

defineEmits<{
  photoContextmenu: [photo: Photo, event: MouseEvent]
}>()

const jumpMonth = ref('')
const atTop = ref(true)

const photos = ref<Photo[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const sentinelTop = ref<HTMLElement | null>(null)
const sentinel = ref<HTMLElement | null>(null)
const groupBy = ref<'day' | 'month'>('day')
const uncategorizedOnly = ref(false)
const filterOpen = ref(false)
const loadingNewer = ref(false)
const hasNewer = ref(false)
let cursor = ''
let observer: IntersectionObserver | null = null
let observerTop: IntersectionObserver | null = null
let controller: AbortController | null = null
let wasInterrupted = false

const hasActiveFilter = computed(() => uncategorizedOnly.value)

// Watch filter change: reload
watch(uncategorizedOnly, () => { reload() })

function reload() {
  controller?.abort()
  photos.value = []
  total.value = 0
  hasMore.value = true
  cursor = ''
  loadPage()
}

// ── Date grouping ────────────────────────────────────

interface Group { label: string; photos: Photo[] }

const groups = computed<Group[]>(() => {
  const result: Group[] = []
  for (const p of photos.value) {
    const label = dateLabel(p.taken_at)
    const last = result[result.length - 1]
    if (last && last.label === label) {
      last.photos.push(p)
    } else {
      result.push({ label, photos: [p] })
    }
  }
  return result
})

function dateLabel(iso: string): string {
  if (!iso) return '未标注日期'
  const d = new Date(iso)
  const y = d.getFullYear()
  const m = d.getMonth() + 1
  if (groupBy.value === 'month') return `${y}年${m}月`
  return `${y}年${m}月${d.getDate()}日`
}

function albumTags(photo: Photo): string[] {
  if (!photo.album_ids?.length || !props.albumTitles) return []
  return photo.album_ids.map(id => props.albumTitles![id] || `#${id}`).filter(Boolean)
}

// ── Date scrubber ────────────────────────────────────

const allDates = ref<Array<{ date: string; count: number }>>([])

const datePoints = computed<DatePoint[]>(() => {
  const loadedSet = new Set(groups.value.map(g => g.label))
  return allDates.value.map(d => ({
    ...d,
    loaded: loadedSet.has(dateLabel(d.date)),
  }))
})

const activeMonth = computed(() => {
  const first = groups.value[0]
  if (!first?.photos.length) return ''
  const t = first.photos[0].taken_at
  if (!t) return ''
  return t.slice(0, 7) // YYYY-MM
})

function jumpToDate(monthKey: string) {
  jumpMonth.value = monthKey
  hasNewer.value = monthKey !== ''
  reload()
}

function scrollToTop() {
  const sp = document.querySelector('.overflow-auto') as HTMLElement
  if (sp) sp.scrollTo({ top: 0, behavior: 'smooth' })
  // Also reset any month filter
  if (hasNewer.value) {
    jumpMonth.value = ''
    hasNewer.value = false
    reload()
  }
}

onMounted(async () => {
  try { allDates.value = await fetchPhotoDates() } catch { /* no scrubber without dates */ }
})

// ── Lightbox ────────────────────────────────────────
const lightbox = reactive({ open: false, photo: null as Photo | null })
let lightboxIdx = 0

function openLightbox(photo: Photo) {
  if (selectMode.value) return
  lightbox.photo = photo
  lightboxIdx = photos.value.indexOf(photo)
  lightbox.open = true
}

function lightboxPrev() {
  if (lightboxIdx > 0) { lightboxIdx--; lightbox.photo = photos.value[lightboxIdx] }
}

function lightboxNext() {
  if (lightboxIdx < photos.value.length - 1) { lightboxIdx++; lightbox.photo = photos.value[lightboxIdx] }
}

function onKeyDown(e: KeyboardEvent) {
  if (!lightbox.open) return
  if (e.key === 'ArrowLeft') { e.preventDefault(); lightboxPrev() }
  if (e.key === 'ArrowRight') { e.preventDefault(); lightboxNext() }
}

// ── Selection mode ──────────────────────────────────

const selectMode = ref(false)
const selected = ref<Set<number>>(new Set())

function enterSelectMode() { selectMode.value = true; selected.value = new Set() }
function exitSelectMode() { selectMode.value = false; selected.value = new Set() }

function onPhotoClick(photo: Photo) {
  if (selectMode.value) {
    const s = new Set(selected.value)
    if (s.has(photo.id)) s.delete(photo.id)
    else s.add(photo.id)
    selected.value = s
  } else {
    openLightbox(photo)
  }
}

// ── Batch add ───────────────────────────────────────

const albumPickerOpen = ref(false)
const albumList = ref<Album[]>([])
const addingResult = ref('')

async function openAlbumPicker() {
  if (selected.value.size === 0) return
  try { albumList.value = (await fetchAlbums()).items }
  catch { albumList.value = [] }
  addingResult.value = ''
  albumPickerOpen.value = true
}

async function doBatchAdd(albumId: number) {
  const ids = Array.from(selected.value)
  try {
    const r = await batchAddPhotos(albumId, ids)
    addingResult.value = `已添加 ${r.added} 张` + (r.skipped > 0 ? `，${r.skipped} 张已存在` : '')
  } catch {
    addingResult.value = '添加失败'
  }
}

// ── Fetch ───────────────────────────────────────────

function calcLimit(): number {
  const w = window.innerWidth
  const cols = w >= 1280 ? 5 : w >= 1024 ? 4 : w >= 768 ? 3 : 2
  const cellSize = w / cols
  const visibleRows = Math.ceil(window.innerHeight / cellSize)
  return Math.max(30, cols * visibleRows * 3)
}

async function loadPage() {
  if (!hasMore.value) return
  if (loading.value) wasInterrupted = true

  controller?.abort()
  controller = new AbortController()
  const { signal } = controller

  const limit = wasInterrupted ? 200 : jumpMonth.value ? 80 : calcLimit()
  wasInterrupted = false

  loading.value = true
  try {
    const data = await props.fetchFn(
      {
        limit,
        cursor: cursor || undefined,
        album_id: uncategorizedOnly.value ? 'none' : undefined,
        with_albums: !!props.albumTitles,
        month: jumpMonth.value || undefined,
      },
      signal
    )

    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''

    // If head photos were preloaded, scroll past them before first paint
    if (jumpMonth.value && (data as any).head_count > 0) {
      jumpMonth.value = ''
      const headCount = (data as any).head_count as number
      const headerOffset = (props.stickyOffset || 0) + 37 + 38

      const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
      // Hide until positioned to prevent flash
      if (scrollParent) scrollParent.style.visibility = 'hidden'

      const doScroll = () => {
        if (!scrollParent) return
        const imgs = scrollParent.querySelectorAll('img[src*="thumbnails"]')
        if (imgs.length > headCount) {
          const target = imgs[headCount] as HTMLElement
          const rect = target.getBoundingClientRect()
          const containerRect = scrollParent.getBoundingClientRect()
          scrollParent.scrollTop = Math.max(0, rect.top - containerRect.top + scrollParent.scrollTop - headerOffset)
        }
        scrollParent.style.visibility = ''
      }

      // Position after DOM renders, before browser paints
      requestAnimationFrame(() => { doScroll() })
      // Safety: show content after 2s even if scroll fails
      setTimeout(() => { if (scrollParent) scrollParent.style.visibility = '' }, 2000)
    }
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
  setTimeout(() => { if (sentinel.value) observer?.observe(sentinel.value) }, 1000)

  // Scroll-up detection for loading newer photos
  const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
  if (scrollParent) {
    let ticking = false
    scrollParent.addEventListener('scroll', () => {
      if (ticking) return
      ticking = true
      requestAnimationFrame(() => {
        atTop.value = scrollParent.scrollTop < 50
        if (scrollParent.scrollTop < 100 && hasNewer.value && !loadingNewer.value) {
          loadNewer()
        }
        ticking = false
      })
    }, { passive: true })
  }
})

async function loadNewer() {
  if (photos.value.length === 0) return
  const first = photos.value[0]
  if (!first.taken_at) return
  const newerThan = first.taken_at + ',' + first.id

  // Save scroll position before prepending
  const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
  const oldHeight = scrollParent?.scrollHeight || 0

  loadingNewer.value = true
  try {
    const data = await props.fetchFn(
      { limit: 50, newer_than: newerThan, with_albums: !!props.albumTitles },
      undefined
    )
    if (data.items.length === 0) {
      hasNewer.value = false
      return
    }
    // Reverse ASC results and prepend
    photos.value.unshift(...data.items.reverse())
    total.value = data.total

    // Restore scroll position so content doesn't jump
    if (scrollParent) {
      requestAnimationFrame(() => {
        const newHeight = scrollParent.scrollHeight
        scrollParent.scrollTop += newHeight - oldHeight
      })
    }
  } catch (e: any) {
    if (e?.name === 'CanceledError') return
    console.error('load newer failed', e)
  } finally {
    loadingNewer.value = false
  }
}

function onImgError(photo: Photo) {
  const url = photo.thumbnail_url
  photo.thumbnail_url = ''
  setTimeout(() => { photo.thumbnail_url = url }, 2000)
}

function removePhotoById(id: number) {
  const idx = photos.value.findIndex(p => p.id === id)
  if (idx !== -1) { photos.value.splice(idx, 1); total.value = Math.max(0, total.value - 1) }
}

defineExpose({ removePhotoById })

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  observer?.disconnect()
  observerTop?.disconnect()
  controller?.abort()
})
</script>
