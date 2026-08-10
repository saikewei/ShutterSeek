<template>
  <div>
    <!-- Filter bar（单页模式无筛选语义，整栏隐藏） -->
    <div
      v-if="!singlePage"
      class="sticky z-20 flex items-center justify-between px-2 py-1.5 bg-neutral-950/90 backdrop-blur border-b border-neutral-800"
      :style="{ top: (stickyOffset || 0) + 'px' }"
    >
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
          v-if="isGuestMobile && !selectMode && !singlePage"
          @click="monthPickerOpen = true"
          class="px-3 py-1 text-xs rounded-full bg-neutral-800 text-neutral-400 hover:bg-neutral-700 hover:text-white transition-colors"
        >月份</button>

        <button
          v-if="isAdmin && !selectMode"
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
          <span v-if="rangeLoading"> · 选择中...</span>
          <span v-if="rangeError" class="text-xs text-red-400 ml-2">{{ rangeError }}</span>
        </span>
      </div>

      <div v-if="selectMode" class="flex items-center gap-1.5">
        <button v-if="isAdmin && removeFromAlbumId !== undefined" @click="confirmRemoveOpen = true" class="px-3 py-1 text-xs rounded-full bg-red-600/80 text-white hover:bg-red-600 transition-colors">
          从相册删除
        </button>
        <button v-if="isAdmin" @click="openAlbumPicker" class="px-3 py-1 text-xs rounded-full bg-white text-black font-medium hover:bg-neutral-200 transition-colors">
          添加到相册
        </button>
        <button @click="exitSelectMode" class="px-3 py-1 text-xs rounded-full text-neutral-400 hover:text-white transition-colors">取消</button>
      </div>

      <!-- Day navigation (sticky filter bar, always visible; compact on mobile) -->
      <div v-if="!selectMode && !singlePage" class="flex items-center gap-1 text-xs">
        <button
          @click="prevDay"
          class="px-2 py-1 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white transition-colors whitespace-nowrap"
          title="前一天"
        >{{ isGuestMobile ? '◀' : '◀ 前一天' }}</button>
        <button
          @click="nextDay"
          class="px-2 py-1 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white transition-colors whitespace-nowrap"
          title="后一天"
        >{{ isGuestMobile ? '▶' : '后一天 ▶' }}</button>
      </div>
    </div>

    <!-- Grid with date separators (单页模式不分组、不显示日期栏) -->
    <div v-for="group in groupCells" :key="group.label || 'all'">
      <div
        v-if="group.label"
        class="sticky z-10 bg-neutral-950/95 backdrop-blur px-2 py-2 text-sm font-semibold tracking-wide border-b border-neutral-800"
        :style="{ top: (stickyOffset || 0) + 37 + 'px' }"
        :data-date="group.label"
        :data-date-iso="group.cells[0]?.photo?.taken_at?.slice(0, 10) || ''"
      >
        <span class="border-l-2 border-neutral-500 pl-2.5 text-neutral-200">{{ group.label }}</span>
      </div>
      <div class="grid grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-1 p-1">
        <div
          v-for="cell in group.cells"
          :key="cell.photo.id"
          class="group cursor-pointer relative rounded-lg overflow-hidden bg-neutral-800"
          :class="{ 'ring-2 ring-white': selectMode && selected.has(cell.photo.id) }"
          @click="onCellClick(cell, $event)"
          @contextmenu.prevent="$emit('photoContextmenu', cell.photo, $event)"
        >
          <img
            :src="THUMB_BASE + '/' + cell.photo.id + '.webp'"
            :alt="cell.photo.camera_model || 'Photo'"
            loading="lazy"
            :class="['w-full aspect-square object-cover', cell.photo.height > cell.photo.width ? 'rotate-270 scale-150' : '']"
            @error="onImgError(cell.photo)"
          />

          <!-- Selection checkbox -->
          <div v-if="selectMode" class="absolute top-1.5 left-1.5">
            <div class="w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors"
              :class="selected.has(cell.photo.id) ? 'bg-white border-white' : 'border-white/60 bg-black/30'"
            >
              <span v-if="selected.has(cell.photo.id)" class="text-black text-xs">✓</span>
            </div>
          </div>

          <!-- Album tags -->
          <div v-if="!selectMode && albumTags(cell.photo).length" class="absolute top-1 left-1 flex flex-wrap gap-0.5 max-w-[90%]">
            <span
              v-for="tag in albumTags(cell.photo).slice(0, 2)"
              :key="tag"
              class="px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-neutral-300 truncate max-w-[80px]"
            >{{ tag }}</span>
            <span v-if="albumTags(cell.photo).length > 2" class="text-[10px] text-neutral-500">+{{ albumTags(cell.photo).length - 2 }}</span>
          </div>

          <!-- 连拍折叠角标 / 展开收起按钮 -->
          <div v-if="cell.collapsed" class="absolute top-1 right-1">
            <span class="px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-neutral-200">×{{ cell.burstCount }}</span>
          </div>
          <button
            v-else-if="cell.collapseFirst"
            class="absolute top-1 right-1 px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-neutral-200 hover:bg-neutral-600 transition-colors"
            title="收起连拍"
            @click.stop="toggleBurst(cell.burstId!)"
          >▴ 收起</button>

          <div class="absolute inset-x-0 bottom-0 p-2 bg-gradient-to-t from-black/70 to-transparent opacity-0 group-hover:opacity-100 transition-opacity">
            <p class="text-xs truncate text-neutral-400">{{ cell.photo.file_name }}</p>
            <p class="text-xs truncate">{{ cell.photo.camera_make }} {{ cell.photo.camera_model }}</p>
            <p class="text-xs text-neutral-300">{{ cell.photo.focal_length }} {{ cell.photo.aperture }} ISO{{ cell.photo.iso }}</p>
            <p class="text-xs text-neutral-400">{{ cell.photo.width }}×{{ cell.photo.height }}</p>
            <p v-if="cell.burstCount" class="text-xs text-neutral-300">连拍 ×{{ cell.burstCount }}</p>
          </div>
          <slot name="photo-action" :photo="cell.photo" />
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

            <label v-if="isAdmin" class="flex items-center gap-2 cursor-pointer">
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

    <!-- Date scrubber (desktop only; mobile uses the month-picker modal) -->
    <DateScrubber v-if="!isGuestMobile && !singlePage" :dates="datePoints" :active-month="activeMonth" @jump="jumpToDate" />

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

    <!-- Remove from album confirmation -->
    <Teleport to="body">
      <div v-if="confirmRemoveOpen" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="confirmRemoveOpen = false" />
        <div class="relative bg-neutral-800 rounded-xl p-5 w-80 shadow-xl border border-neutral-700">
          <h2 class="text-sm font-medium text-white mb-1">从相册移除</h2>
          <p class="text-xs text-neutral-400 mb-4">确定从相册移除选中的 {{ selected.size }} 张照片吗？</p>
          <div class="flex justify-end gap-2">
            <button @click="confirmRemoveOpen = false" class="px-3 py-1.5 text-xs rounded-full text-neutral-400 hover:text-white">取消</button>
            <button @click="doRemoveFromAlbum" :disabled="removingFromAlbum" class="px-4 py-1.5 text-xs rounded-full bg-red-600 text-white font-medium hover:bg-red-500 disabled:opacity-50">
              {{ removingFromAlbum ? '移除中...' : '移除' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Mobile month picker modal -->
    <Teleport to="body">
      <div v-if="monthPickerOpen && !singlePage" class="fixed inset-0 z-50 flex items-end sm:items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="monthPickerOpen = false" />
        <div class="relative w-full max-h-[70vh] flex flex-col px-3 pb-3">
          <div class="mb-2 flex items-center justify-between">
            <h2 class="text-sm font-medium text-white">跳转到月份</h2>
            <button @click="monthPickerOpen = false" class="text-xs text-neutral-400 hover:text-white">关闭</button>
          </div>
          <DateScrubber embedded :dates="datePoints" :active-month="activeMonth" @jump="onMonthJump" />
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
import { fetchAlbums, batchAddPhotos, removeAlbumPhotos, type Album } from '@/api/albums'
import { isAdmin } from '@/stores/auth'
import { isGuestMobile } from '@/stores/device'
import Lightbox from '@/components/Lightbox.vue'
import DateScrubber from '@/components/DateScrubber.vue'
import type { DatePoint } from '@/components/DateScrubber.vue'

const props = defineProps<{
  fetchFn: (
    params: { limit: number; cursor?: string; newer_t?: string; newer_id?: number; album_id?: string; with_albums?: boolean; month?: string; date?: string },
    signal?: AbortSignal
  ) => Promise<PhotoListResponse>
  albumTitles?: Record<number, string>
  stickyOffset?: number
  datesFn?: () => Promise<Array<{ date: string; count: number }>>
  rangeFn?: (fromId: number, toId: number, opts?: { album_id?: string }) => Promise<number[]>
  removeFromAlbumId?: number
  singlePage?: boolean
}>()

const emit = defineEmits<{
  photoContextmenu: [photo: Photo, event: MouseEvent]
  removedFromAlbum: []
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
let controller: AbortController | null = null
let wasInterrupted = false

const hasActiveFilter = computed(() => uncategorizedOnly.value)

// Watch filter change: reload
watch(uncategorizedOnly, () => { reload() })

function reload() {
  controller?.abort()
  controller = null
  photos.value = []
  total.value = 0
  hasMore.value = true
  headCount.value = 0
  cursor = ''
  wasInterrupted = false
  loading.value = false
  loadingNewer.value = false
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

// 单页模式（搜索结果按相似度排序）不按日期分组，仅渲染无标题的单组
const displayGroups = computed<Group[]>(() => {
  if (props.singlePage) return [{ label: '', photos: photos.value }]
  return groups.value
})

// ── 连拍堆叠 ──────────────────────────────────────

interface BurstCell {
  photo: Photo
  burstId?: string
  burstCount?: number
  collapsed?: boolean
  collapseFirst?: boolean
}

const expandedBursts = ref<Set<string>>(new Set())

function toggleBurst(id: string) {
  const s = new Set(expandedBursts.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedBursts.value = s
}

// 同一秒的连续照片合并为一组；singlePage（搜索）与选择模式不折叠
function buildCells(list: Photo[]): BurstCell[] {
  const out: BurstCell[] = []
  if (props.singlePage || selectMode.value) {
    for (const p of list) out.push({ photo: p })
    return out
  }
  const run: Photo[] = []
  const flush = () => {
    if (run.length === 1) {
      out.push({ photo: run[0] })
    } else if (run.length > 1) {
      const bid = run[0].taken_at + '#' + run[0].id
      if (expandedBursts.value.has(bid)) {
        run.forEach((p, i) => {
          out.push({ photo: p, burstId: bid, burstCount: run.length, collapseFirst: i === 0 })
        })
      } else {
        out.push({ photo: run[0], burstId: bid, burstCount: run.length, collapsed: true })
      }
    }
    run.length = 0
  }
  for (const p of list) {
    const same = !!p.taken_at && run.length > 0 && run[0].taken_at === p.taken_at
    if (!same) flush()
    run.push(p)
  }
  flush()
  return out
}

const groupCells = computed(() =>
  displayGroups.value.map(g => ({ label: g.label, cells: buildCells(g.photos) })),
)

function onCellClick(cell: BurstCell, e: MouseEvent) {
  if (cell.collapsed && cell.burstId) {
    toggleBurst(cell.burstId)
    return
  }
  onPhotoClick(cell.photo, e)
}

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

const headCount = ref(0)

const jumpCooldown = ref(false)

// Mobile month picker
const monthPickerOpen = ref(false)

function jumpToDate(monthKey: string) {
  jumpMonth.value = monthKey
  hasNewer.value = monthKey !== ''
  // 同步日期导航锚点到目标月的第一张照片日期（按全库日期分布）
  const first = allDates.value.map(d => d.date).find(d => d.startsWith(monthKey))
  focusDate.value = first || monthKey + '-01'
  jumpCooldown.value = true
  setTimeout(() => { jumpCooldown.value = false }, 500)
  reload()
}

// ── Day navigation (previous / next day) ─────────────

const focusDate = ref('') // YYYY-MM-DD — the day nav anchor
const jumpDate = ref('')  // pending date jump param

// 月份选择器高亮：跟随当前可见日期（由滚动同步 focusDate 派生）
const activeMonth = computed(() => (focusDate.value ? focusDate.value.slice(0, 7) : ''))

// 滚动时检测当前钉在顶部的日期分组，同步日期导航锚点
function updateVisibleDate() {
  const sp = document.querySelector('.overflow-auto') as HTMLElement
  if (!sp) return
  const headers = Array.from(sp.querySelectorAll<HTMLElement>('[data-date-iso]'))
  if (headers.length === 0) return
  // 日期头 sticky top = stickyOffset + 37（筛选栏高度），换算到视口坐标
  const line = sp.getBoundingClientRect().top + (props.stickyOffset || 0) + 37 + 1
  let current = headers[0]
  for (const h of headers) {
    if (h.getBoundingClientRect().top <= line) current = h
    else break
  }
  const iso = current.dataset.dateIso
  if (iso && iso !== focusDate.value) focusDate.value = iso
}

function fmtDay(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function jumpToDay(dateStr: string) {
  jumpDate.value = dateStr
  hasNewer.value = dateStr !== ''
  jumpCooldown.value = true
  setTimeout(() => { jumpCooldown.value = false }, 500)
  reload()
}

// The nearest day with photos before/after focusDate, from the loaded
// date distribution. Returns null when no photo day exists on that side
// (jump boundary). Falls back to ±1 calendar day while dates haven't loaded.
function nearestDay(dir: 'prev' | 'next'): string | null {
  const dates = allDates.value.map(d => d.date).filter(Boolean)
  if (dates.length > 0) {
    if (dir === 'prev') {
      const earlier = dates.filter(d => d < focusDate.value).sort()
      if (earlier.length > 0) return earlier[earlier.length - 1]
      return null // no older photos
    }
    const later = dates.filter(d => d > focusDate.value).sort()
    if (later.length > 0) return later[0]
    return null // no newer photos
  }
  // fallback while date distribution isn't loaded yet
  const d = new Date(focusDate.value + 'T00:00:00')
  d.setDate(d.getDate() + (dir === 'prev' ? -1 : 1))
  return fmtDay(d)
}

function prevDay() {
  if (!focusDate.value) return
  const target = nearestDay('prev')
  if (!target) return // already at the oldest photo day
  focusDate.value = target
  jumpToDay(target)
}

function nextDay() {
  if (!focusDate.value) return
  const target = nearestDay('next')
  if (!target) return // already at the newest photo day
  focusDate.value = target
  jumpToDay(target)
}

function onMonthJump(monthKey: string) {
  monthPickerOpen.value = false
  jumpToDate(monthKey)
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
  const fn = props.datesFn || fetchPhotoDates
  try { allDates.value = await fn() } catch { /* no scrubber */ }
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
const anchorId = ref<number | null>(null)
const rangeLoading = ref(false)
const rangeError = ref('')

function enterSelectMode() { selectMode.value = true; selected.value = new Set(); anchorId.value = null; rangeError.value = '' }
function exitSelectMode() { selectMode.value = false; selected.value = new Set(); anchorId.value = null; rangeError.value = '' }

function onPhotoClick(photo: Photo, e: MouseEvent) {
  if (selectMode.value) {
    if (e.shiftKey && anchorId.value !== null && props.rangeFn) {
      doRangeSelect(anchorId.value, photo.id)
      return
    }
    const s = new Set(selected.value)
    if (s.has(photo.id)) s.delete(photo.id)
    else s.add(photo.id)
    selected.value = s
    anchorId.value = photo.id
  } else {
    openLightbox(photo)
  }
}

async function doRangeSelect(fromId: number, toId: number) {
  if (rangeLoading.value) return
  rangeLoading.value = true
  rangeError.value = ''
  try {
    const ids = await props.rangeFn!(fromId, toId, { album_id: uncategorizedOnly.value ? 'none' : undefined })
    if (!selectMode.value) return // 期间退出了选择模式，丢弃结果
    const s = new Set(selected.value)
    for (const id of ids) s.add(id)
    selected.value = s
    anchorId.value = toId
  } catch (e: any) {
    console.error('range select failed', e)
    rangeError.value = e?.response?.data?.error === 'range too large'
      ? 'Range too large (max 5000)'
      : 'Range select failed'
  } finally {
    rangeLoading.value = false
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

// ── Remove from album (batch) ────────────────────────

const confirmRemoveOpen = ref(false)
const removingFromAlbum = ref(false)

async function doRemoveFromAlbum() {
  if (props.removeFromAlbumId === undefined || selected.value.size === 0) return
  if (removingFromAlbum.value) return
  removingFromAlbum.value = true
  const ids = Array.from(selected.value)
  try {
    await removeAlbumPhotos(props.removeFromAlbumId, ids)
    // Remove from the visible list
    for (const id of ids) removePhotoById(id)
    emit('removedFromAlbum')
    exitSelectMode()
  } catch {
    // keep selection so the user can retry
  } finally {
    confirmRemoveOpen.value = false
    removingFromAlbum.value = false
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

let loadId = 0

async function loadPage() {
  if (!hasMore.value) return
  if (loading.value) { wasInterrupted = true; return }

  const myLoadId = ++loadId
  const monthParam = jumpMonth.value || undefined
  const dateParam = jumpDate.value || undefined
  const jumpParam = dateParam || monthParam // date takes precedence

  controller?.abort()
  controller = new AbortController()
  const { signal } = controller

  const limit = wasInterrupted ? 200 : jumpParam ? 80 : calcLimit()
  wasInterrupted = false

  loading.value = true
  try {
    const data = await props.fetchFn(
      {
        limit,
        cursor: cursor || undefined,
        album_id: uncategorizedOnly.value ? 'none' : undefined,
        with_albums: !!props.albumTitles,
        month: monthParam,
        date: dateParam,
      },
      signal
    )

    // Discard if a newer load has started
    if (myLoadId !== loadId) return

    photos.value.push(...data.items)
    total.value = data.total
    cursor = data.next_cursor
    hasMore.value = data.next_cursor !== ''
    if (props.singlePage) hasMore.value = false

    // Initialize the day-nav anchor from the first photo on first load
    if (!focusDate.value && photos.value.length > 0) {
      const first = photos.value[0].taken_at
      if (first) focusDate.value = first.slice(0, 10)
    }

    headCount.value = (data as any).head_count || 0
    if (jumpParam) {
      jumpMonth.value = ''
      jumpDate.value = ''
      if (headCount.value > 0) {
        const headerOffset = (props.stickyOffset || 0) + 37 + 38
        const hc = headCount.value

        const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
        if (scrollParent) scrollParent.style.visibility = 'hidden'

        const doScroll = () => {
          if (!scrollParent) return
          const imgs = scrollParent.querySelectorAll('img[src*="thumbnails"]')
          if (imgs.length > hc) {
            const target = imgs[hc] as HTMLElement
            const rect = target.getBoundingClientRect()
            const containerRect = scrollParent.getBoundingClientRect()
            scrollParent.scrollTop = Math.max(0, rect.top - containerRect.top + scrollParent.scrollTop - headerOffset)
          }
          scrollParent.style.visibility = ''
        }

        requestAnimationFrame(() => { doScroll() })
        setTimeout(() => { if (scrollParent) scrollParent.style.visibility = '' }, 2000)
      }
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

  // Scroll-up detection for loading newer photos（单页模式跳过）
  if (!props.singlePage) {
    const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
    if (scrollParent) {
      let ticking = false
      scrollParent.addEventListener('scroll', () => {
        if (ticking) return
        ticking = true
        requestAnimationFrame(() => {
          atTop.value = scrollParent.scrollTop < 50
          updateVisibleDate()
          if (scrollParent.scrollTop < 100 && hasNewer.value && !loadingNewer.value && !jumpCooldown.value) {
            loadNewer()
          }
          ticking = false
        })
      }, { passive: true })
    }
  }
})

async function loadNewer() {
  if (photos.value.length === 0) return
  if (jumpMonth.value) return // don't run during a jump
  const first = photos.value[0]
  if (!first.taken_at) return
  const newerT = first.taken_at
  const newerID = first.id

  // Save scroll position before prepending
  const scrollParent = document.querySelector('.overflow-auto') as HTMLElement
  const oldHeight = scrollParent?.scrollHeight || 0

  loadingNewer.value = true
  try {
    const data = await props.fetchFn(
      { limit: 50, newer_t: newerT, newer_id: newerID, with_albums: !!props.albumTitles },
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
  controller?.abort()
})
</script>
