<template>
  <div ref="rootEl">
    <!-- Filter bar. Hidden entirely in single-page mode, which has no filter
         semantics. Everything lives on one row: the controls scroll sideways
         when they do not fit, and the day stepper stays pinned on the right so
         it is always reachable. The current day is already shown by the sticky
         date header below, so the stepper does not repeat it. -->
    <div
      v-if="!singlePage"
      ref="filterBar"
      class="sticky z-20 bg-canvas/90 backdrop-blur border-b border-line"
      :style="{ top: topOffset + 'px' }"
    >
      <div class="flex items-center gap-1.5 px-2 py-1.5">
        <div class="no-scrollbar flex-1 min-w-0 flex items-center gap-1.5 overflow-x-auto">
          <button
            @click="filterOpen = true"
            class="relative shrink-0 px-3.5 py-1.5 text-xs rounded-full transition-colors"
            :class="hasActiveFilter ? 'bg-ink text-[#1C1208]' : 'bg-surface text-ink-2 hover:bg-line-strong hover:text-ink'"
          >
            筛选
            <span v-if="hasActiveFilter" class="ml-1 text-[10px]">●</span>
          </button>

          <button
            v-if="isMobileShell && !selectMode && !singlePage"
            @click="monthPickerOpen = true"
            class="shrink-0 px-3.5 py-1.5 text-xs rounded-full bg-surface text-ink-2 hover:bg-line-strong hover:text-ink transition-colors"
          >月份</button>

          <button
            v-if="isAdmin && !selectMode"
            @click="enterSelectMode"
            class="shrink-0 px-3.5 py-1.5 text-xs rounded-full bg-surface text-ink-2 hover:bg-line-strong hover:text-ink transition-colors"
          >选择</button>

          <button
            v-if="!selectMode && (!atTop || hasNewer)"
            @click="scrollToTop"
            class="shrink-0 px-3.5 py-1.5 text-xs rounded-full bg-line-strong text-ink-2 hover:text-ink transition-colors"
          >{{ hasNewer ? '返回最新' : '↑ 顶部' }}</button>

          <template v-if="selectMode">
            <span class="shrink-0 text-xs text-ink-2 whitespace-nowrap">
              已选 {{ selected.size }} 张
              <span v-if="rangeLoading"> · 选择中...</span>
            </span>
            <button
              v-if="rangeFn"
              @click="rangeMode = !rangeMode"
              class="shrink-0 px-3.5 py-1.5 text-xs rounded-full transition-colors"
              :class="rangeMode ? 'bg-accent text-[#1C1208] font-semibold' : 'bg-surface text-ink-2 hover:text-ink'"
              title="打开后，点第二张即可选中区间"
            >范围</button>
            <span v-if="rangeError" class="shrink-0 text-xs text-danger-ink whitespace-nowrap">{{ rangeError }}</span>
          </template>
        </div>

        <!-- Day stepper, pinned so it never scrolls out of reach -->
        <div v-if="!selectMode" class="shrink-0 flex items-center gap-1">
          <button
            @click="prevDay"
            class="h-8 px-2.5 rounded-lg bg-surface hover:bg-line-strong text-ink-2 hover:text-ink transition-colors whitespace-nowrap text-xs"
            title="前一天"
          >{{ isMobileShell ? '‹' : '◀ 前一天' }}</button>
          <button
            @click="nextDay"
            class="h-8 px-2.5 rounded-lg bg-surface hover:bg-line-strong text-ink-2 hover:text-ink transition-colors whitespace-nowrap text-xs"
            title="后一天"
          >{{ isMobileShell ? '›' : '后一天 ▶' }}</button>
        </div>
      </div>

      <div v-if="selectMode" class="flex items-center gap-1.5 px-2 pb-1.5">
        <button v-if="isAdmin && removeFromAlbumId !== undefined" @click="confirmRemoveOpen = true" class="btn-danger-soft px-3 py-1.5 text-xs">
          从相册删除
        </button>
        <button v-if="isAdmin" @click="openAlbumPicker" class="px-3 py-1.5 text-xs rounded-full bg-accent text-[#1C1208] font-semibold hover:bg-accent-strong transition-colors">
          添加到相册
        </button>
        <button @click="exitSelectMode" class="ml-auto px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink transition-colors">取消</button>
      </div>
    </div>

    <!-- Grid with date separators (single-page mode is not grouped) -->
    <div v-for="group in groupCells" :key="group.label || 'all'">
      <div
        v-if="group.label"
        class="sticky z-10 bg-canvas/95 backdrop-blur px-2 h-[42px] flex items-center gap-2.5 border-b border-line"
        :style="{ top: topOffset + filterH + 'px' }"
        :data-date="group.label"
        :data-date-iso="group.cells[0]?.photo?.taken_at?.slice(0, 10) || ''"
      >
        <span class="date-header border-l-2 border-accent pl-2.5">{{ group.label }}</span>
        <span v-if="groupCount(group.label)" class="text-xs text-ink-3 tabular-nums">{{ groupCount(group.label) }} 张</span>
      </div>
      <div
        class="grid grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-[3px] p-[3px]"
        :style="gridStyle"
      >
        <div
          v-for="cell in group.cells"
          :key="cell.photo.id"
          class="group cursor-pointer relative rounded-md overflow-hidden bg-surface"
          :class="{ 'ring-2 ring-accent shadow-[0_0_14px_rgba(201,136,98,0.35)]': selectMode && selected.has(cell.photo.id) }"
          @click="onCellClick(cell, $event)"
          @contextmenu.prevent="onContextMenu(cell, $event)"
          @pointerdown="onPressStart(cell, $event)"
          @pointermove="onPressMove"
          @pointerup="onPressEnd"
          @pointercancel="onPressEnd"
          @pointerleave="onPressEnd"
        >
          <img
            :src="thumbSrc(cell.photo)"
            :alt="cell.photo.camera_model || 'Photo'"
            loading="lazy"
            decoding="async"
            class="thumb-in w-full aspect-square object-cover"
            :class="cell.photo.height > cell.photo.width ? 'rotate-270 scale-150' : ''"
            @error="onThumbError(cell.photo)"
          />

          <!-- Selection checkbox -->
          <div v-if="selectMode" class="absolute top-1.5 left-1.5">
            <div class="w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors"
              :class="selected.has(cell.photo.id) ? 'bg-accent border-accent' : 'border-white/60 bg-black/30'"
            >
              <span v-if="selected.has(cell.photo.id)" class="text-[#1C1208] text-xs">✓</span>
            </div>
          </div>

          <!-- Album tags -->
          <div v-if="!selectMode && albumTags(cell.photo).length" class="absolute top-1 left-1 flex flex-wrap gap-0.5 max-w-[90%]">
            <span
              v-for="tag in albumTags(cell.photo).slice(0, 2)"
              :key="tag"
              class="px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-ink-2 backdrop-blur-[2px] truncate max-w-[80px]"
            >{{ tag }}</span>
            <span v-if="albumTags(cell.photo).length > 2" class="text-[10px] text-ink-3">+{{ albumTags(cell.photo).length - 2 }}</span>
          </div>

          <!-- Burst collapse badge / expand-collapse button -->
          <div v-if="cell.collapsed" class="absolute top-1 right-1">
            <span class="px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-ink-2 backdrop-blur-[2px]">×{{ cell.burstCount }}</span>
          </div>
          <button
            v-else-if="cell.collapseFirst"
            class="absolute top-1 right-1 px-1.5 py-0.5 text-[10px] rounded bg-black/60 text-ink-2 backdrop-blur-[2px] hover:bg-black/80 hover:text-ink transition-colors"
            title="收起连拍"
            @click.stop="toggleBurst(cell.burstId!)"
          >▴ 收起</button>

          <div class="absolute inset-x-0 bottom-0 p-2 bg-gradient-to-t from-[#0c0a09]/90 to-transparent opacity-0 group-hover:opacity-100 transition-opacity">
            <p class="text-xs truncate text-ink">{{ cell.photo.file_name }}</p>
            <p class="text-xs truncate">{{ cell.photo.camera_make }} {{ cell.photo.camera_model }}</p>
            <p class="text-xs text-ink-2">{{ cell.photo.focal_length }} {{ cell.photo.aperture }} ISO{{ cell.photo.iso }}</p>
            <p class="text-xs text-ink-2">{{ cell.photo.width }}×{{ cell.photo.height }}</p>
            <p v-if="cell.burstCount" class="text-xs text-ink-2">连拍 ×{{ cell.burstCount }}</p>
          </div>
          <slot name="photo-action" :photo="cell.photo" />
        </div>
      </div>
    </div>

    <div ref="sentinelTop" class="py-6 text-center text-ink-3 text-sm">
      <span v-if="loadingNewer" class="text-xs">加载中...</span>
    </div>

    <div ref="sentinel" class="py-12 flex flex-col items-center gap-3 text-center text-ink-3 text-sm">
      <!-- A visible way out: if automatic loading ever gives up, the list must
           not become a dead end that only a reload can fix. -->
      <button v-if="hasMore && !loading" class="btn-ghost px-4 py-1.5 text-xs" @click="loadPage()">
        加载更多
      </button>
      <span v-else-if="hasMore" class="text-xs">加载中...</span>
      <span v-else>没有更多了 · 共 {{ total.toLocaleString() }} 张</span>
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
    <AppModal :open="filterOpen" title="筛选设置" @close="filterOpen = false">
      <div class="space-y-5">
        <div>
          <label class="text-xs text-ink-2 block mb-2">日期分组</label>
          <div class="flex gap-1">
            <button
              v-for="opt in [{k:'day',l:'按日'},{k:'month',l:'按月'}]"
              :key="opt.k"
              @click="groupBy = opt.k as 'day'|'month'"
              :class="groupBy === opt.k ? 'bg-line-strong text-ink' : 'bg-surface text-ink-3 hover:text-ink'"
              class="flex-1 py-1.5 text-xs rounded-lg transition-colors"
            >{{ opt.l }}</button>
          </div>
        </div>

        <div v-if="isMobileShell">
          <label class="text-xs text-ink-2 block mb-2">每行张数</label>
          <div class="flex gap-1">
            <button
              v-for="n in [3, 4, 5]"
              :key="n"
              @click="gridCols = n"
              :class="gridCols === n ? 'bg-line-strong text-ink' : 'bg-surface text-ink-3 hover:text-ink'"
              class="flex-1 py-1.5 text-xs rounded-lg transition-colors"
            >{{ n }} 张</button>
          </div>
        </div>

        <label v-if="isAdmin" class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" v-model="uncategorizedOnly" class="rounded accent-accent" />
          <span class="text-xs text-ink-2">仅显示未归类照片</span>
        </label>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button @click="filterOpen = false" class="px-4 py-1.5 text-xs rounded-full bg-accent text-[#1C1208] font-semibold hover:bg-accent-strong">关闭</button>
        </div>
      </template>
    </AppModal>

    <!-- Date scrubber (desktop only; mobile uses the month-picker modal) -->
    <DateScrubber v-if="!isMobileShell && !singlePage" :dates="datePoints" :active-month="activeMonth" @jump="jumpToDate" />

    <!-- Album picker dialog -->
    <AppModal :open="albumPickerOpen" title="添加到相册" @close="albumPickerOpen = false">
      <div class="space-y-1">
        <button
          v-for="album in albumList"
          :key="album.id"
          @click="doBatchAdd(album.id)"
          class="w-full text-left px-3 py-2.5 rounded-lg text-sm text-ink-2 hover:bg-surface hover:text-ink transition-colors flex justify-between"
        >
          <span>{{ album.title }}</span>
          <span class="text-xs text-ink-3">{{ album.photo_count }}</span>
        </button>
      </div>

      <p v-if="addingResult" class="text-xs text-ink-2 mt-3">{{ addingResult }}</p>
    </AppModal>

    <!-- Remove from album confirmation -->
    <AppModal :open="confirmRemoveOpen" title="从相册移除" @close="confirmRemoveOpen = false">
      <p class="text-xs text-ink-2 mb-4">确定从相册移除选中的 {{ selected.size }} 张照片吗？</p>
      <div class="flex justify-end gap-2">
        <button @click="confirmRemoveOpen = false" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink">取消</button>
        <button @click="doRemoveFromAlbum" :disabled="removingFromAlbum" class="btn-danger px-4 py-1.5 text-xs">
          {{ removingFromAlbum ? '移除中...' : '移除' }}
        </button>
      </div>
    </AppModal>

    <!-- Mobile month picker modal -->
    <AppModal :open="monthPickerOpen && !singlePage" title="跳转到月份" @close="monthPickerOpen = false">
      <DateScrubber embedded :dates="datePoints" :active-month="activeMonth" @jump="onMonthJump" />
    </AppModal>

    <!-- Long-press action sheet (touch only) -->
    <ActionSheet
      :open="sheet.open"
      :title="sheet.photo?.file_name || ''"
      :actions="sheetActions"
      @close="sheet.open = false"
      @select="onSheetSelect"
    />
  </div>
</template>

<script setup lang="ts">
import { inject, ref, computed, onMounted, onUnmounted, reactive, watch } from 'vue'
import type { Photo, PhotoListResponse } from '@/api/photos'
import { fetchPhotoDates } from '@/api/photos'
import { THUMB_BASE } from '@/api/client'
import { fetchAlbums, batchAddPhotos, removeAlbumPhoto, removeAlbumPhotos, type Album } from '@/api/albums'
import { isAdmin } from '@/stores/auth'
import { isMobileShell } from '@/stores/device'
import { topOffsetKey, useElementHeight } from '@/lib/chrome'
import { colsFor } from '@/lib/grid'
import { loadAheadPx, shouldAutoLoad } from '@/lib/infiniteScroll'
import {
  getScrollTop,
  hostScrollHeight,
  hostViewportTop,
  onHostScroll,
  scrollHostBy,
  scrollHostToTop,
  setHostVisibility,
  setScrollTop,
} from '@/lib/scrollHost'
import Lightbox from '@/components/Lightbox.vue'
import DateScrubber from '@/components/DateScrubber.vue'
import AppModal from '@/components/AppModal.vue'
import ActionSheet, { type SheetAction } from '@/components/ActionSheet.vue'
import type { DatePoint } from '@/components/DateScrubber.vue'

const props = defineProps<{
  fetchFn: (
    params: { limit: number; cursor?: string; newer_t?: string; newer_id?: number; album_id?: string; with_albums?: boolean; month?: string; date?: string },
    signal?: AbortSignal
  ) => Promise<PhotoListResponse>
  albumTitles?: Record<number, string>
  datesFn?: () => Promise<Array<{ date: string; count: number }>>
  rangeFn?: (fromId: number, toId: number, opts?: { album_id?: string }) => Promise<number[]>
  removeFromAlbumId?: number
  singlePage?: boolean
}>()

const emit = defineEmits<{
  photoContextmenu: [photo: Photo, event: MouseEvent]
  removedFromAlbum: []
  setCover: [photo: Photo]
}>()

// Sticky chrome geometry. `topOffset` is injected by whichever shell/page owns
// the pinned header above the grid; the filter bar height is measured here.
// Everything sticky is expressed in these terms, no hard-coded offsets.
const topOffsetFn = inject(topOffsetKey, () => 0)
const topOffset = computed(topOffsetFn)
const rootEl = ref<HTMLElement | null>(null)
const filterBar = ref<HTMLElement | null>(null)
const filterH = useElementHeight(() => filterBar.value)

// Keep in sync with the h-[42px] date header in the template.
const dateHeaderH = 42

// Thumbnail loading.
//
// The fade-in used to be a JS-gated opacity: the <img> started at opacity 0
// and only `load` could reveal it. Any thumbnail whose load event did not
// arrive (a request the browser retried internally, a decode hiccup, a
// re-created element) stayed invisible forever -- "the thumbnail never loads"
// even though the file had arrived. The fade is now a CSS animation, which
// always reaches opacity 1 on its own, so it cannot get stuck.
//
// A failed image also never retries by itself, so failures get a bounded
// number of attempts with a cache-busting query. (The previous handler mutated
// photo.thumbnail_url, which the <img> does not render -- so it did nothing.)
const THUMB_MAX_RETRY = 2
const thumbRetry = reactive(new Map<number, number>())

function thumbSrc(photo: Photo): string {
  const attempt = thumbRetry.get(photo.id) ?? 0
  return attempt === 0
    ? `${THUMB_BASE}/${photo.id}.webp`
    : `${THUMB_BASE}/${photo.id}.webp?r=${attempt}`
}

function onThumbError(photo: Photo) {
  const attempt = thumbRetry.get(photo.id) ?? 0
  if (attempt >= THUMB_MAX_RETRY) return
  thumbRetry.set(photo.id, attempt + 1)
}

// Mobile density. Persisted so the choice survives a reload.
const GRID_COLS_KEY = 'ss.gridCols'
const gridCols = ref(Number(localStorage.getItem(GRID_COLS_KEY)) || 3)
watch(gridCols, (n) => {
  try { localStorage.setItem(GRID_COLS_KEY, String(n)) } catch { /* private mode */ }
})

const gridStyle = computed(() =>
  isMobileShell.value
    ? { gridTemplateColumns: `repeat(${gridCols.value}, minmax(0, 1fr))` }
    : undefined,
)

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
let offScroll: (() => void) | null = null
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

// Single-page mode (search results, ordered by similarity) is not grouped
// by date: it renders one untitled group.
const displayGroups = computed<Group[]>(() => {
  if (props.singlePage) return [{ label: '', photos: photos.value }]
  return groups.value
})

// ── Burst stacking ────────────────────────────────

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

// Photos sharing one timestamp collapse into a single cell. Single-page
// mode (search) and selection mode never collapse.
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

// ── Long press (touch) ───────────────────────────────
// Touch has no hover and no dependable contextmenu, which used to leave every
// per-photo action unreachable on a phone. A held press opens an action sheet.
// The gesture is passive: it never calls preventDefault, so scrolling and the
// normal tap path keep working, and a moving finger cancels the press.

const LONG_PRESS_MS = 450
const LONG_PRESS_SLOP = 8

const sheet = reactive<{ open: boolean; photo: Photo | null }>({ open: false, photo: null })
let pressTimer: number | null = null
let pressOrigin = { x: 0, y: 0 }

function cancelPress() {
  if (pressTimer !== null) {
    clearTimeout(pressTimer)
    pressTimer = null
  }
}

// Lifting the finger after a long press still produces a synthetic click. By
// then the action sheet covers the screen, so that click lands on the sheet's
// own backdrop and closed it the instant it appeared -- the reported "hold,
// menu shows, let go, menu disappears". Swallow exactly that one click.
//
// It is disarmed by the first pointerdown, because a real tap on the sheet
// starts a fresh gesture while the leftover click does not; the timeout is
// only a safety net so the listener can never stay armed.
let disarmSwallow: (() => void) | null = null

function swallowNextClick() {
  disarmSwallow?.()

  const timer = window.setTimeout(() => cleanup(), 800)

  function cleanup() {
    clearTimeout(timer)
    document.removeEventListener('click', onClick, true)
    document.removeEventListener('pointerdown', onPointerDown, true)
    disarmSwallow = null
  }
  function onClick(e: Event) {
    e.stopPropagation()
    e.preventDefault()
    cleanup()
  }
  function onPointerDown() {
    cleanup()
  }

  document.addEventListener('click', onClick, true)
  document.addEventListener('pointerdown', onPointerDown, true)
  disarmSwallow = cleanup
}

function onPressStart(cell: BurstCell, e: PointerEvent) {
  if (e.pointerType === 'mouse') return
  if (cell.collapsed && cell.burstId) return
  cancelPress()
  pressOrigin = { x: e.clientX, y: e.clientY }
  pressTimer = window.setTimeout(() => {
    pressTimer = null
    sheet.photo = cell.photo
    sheet.open = true
    swallowNextClick()
  }, LONG_PRESS_MS)
}

function onPressMove(e: PointerEvent) {
  if (pressTimer === null) return
  const moved =
    Math.abs(e.clientX - pressOrigin.x) > LONG_PRESS_SLOP ||
    Math.abs(e.clientY - pressOrigin.y) > LONG_PRESS_SLOP
  if (moved) cancelPress()
}

function onPressEnd() {
  cancelPress()
}

function onContextMenu(cell: BurstCell, e: MouseEvent) {
  // On touch the long press owns this, so the desktop menu is not doubled up.
  if (isMobileShell.value) return
  emit('photoContextmenu', cell.photo, e)
}

const sheetActions = computed<SheetAction[]>(() => {
  if (!sheet.photo) return []
  const actions: SheetAction[] = [{ key: 'open', label: '查看原图' }]
  if (isAdmin.value) {
    if (props.removeFromAlbumId !== undefined) {
      actions.push({ key: 'cover', label: '设为封面' })
    }
    actions.push({ key: 'select', label: '选择照片' })
    if (props.removeFromAlbumId !== undefined) {
      actions.push({ key: 'remove', label: '从相册移除', danger: true })
    }
  }
  return actions
})

function onSheetSelect(key: string) {
  const photo = sheet.photo
  if (!photo) return
  if (key === 'open') {
    openLightbox(photo)
  } else if (key === 'cover') {
    emit('setCover', photo)
  } else if (key === 'select') {
    enterSelectMode()
    selected.value = new Set([photo.id])
    anchorId.value = photo.id
  } else if (key === 'remove' && props.removeFromAlbumId !== undefined) {
    const albumId = props.removeFromAlbumId
    removeAlbumPhoto(albumId, photo.id)
      .then(() => {
        removePhotoById(photo.id)
        emit('removedFromAlbum')
      })
      .catch(() => { /* leave the grid untouched so the user can retry */ })
  }
}

function dateLabel(iso: string): string {
  if (!iso) return '未标注日期'
  const d = new Date(iso)
  const y = d.getFullYear()
  const m = d.getMonth() + 1
  if (groupBy.value === 'month') return `${y}年${m}月`
  return `${y}年${m}月${d.getDate()}日`
}

// "N photos" beside a date header, read from the loaded date distribution
// (per day, or summed per month). Display only.
function groupCount(label: string): number {
  if (!label) return 0
  if (groupBy.value === 'month') {
    const m = label.match(/^(\d+)年(\d+)月$/)
    if (!m) return 0
    const key = `${m[1]}-${m[2].padStart(2, '0')}`
    return allDates.value.filter(d => d.date.startsWith(key)).reduce((s, d) => s + d.count, 0)
  }
  const m = label.match(/^(\d+)年(\d+)月(\d+)日$/)
  if (!m) return 0
  const key = `${m[1]}-${m[2].padStart(2, '0')}-${m[3].padStart(2, '0')}`
  return allDates.value.find(d => d.date === key)?.count || 0
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
  // Move the day-navigation anchor to the first photo day of the target
  // month, taken from the whole-library date distribution.
  const first = allDates.value.map(d => d.date).find(d => d.startsWith(monthKey))
  focusDate.value = first || monthKey + '-01'
  jumpCooldown.value = true
  setTimeout(() => { jumpCooldown.value = false }, 500)
  reload()
}

// ── Day navigation (previous / next day) ─────────────

const focusDate = ref('') // YYYY-MM-DD — the day nav anchor
const jumpDate = ref('')  // pending date jump param

// Month-picker highlight: derived from focusDate, which the scroll handler
// keeps in sync with the visible date.
const activeMonth = computed(() => (focusDate.value ? focusDate.value.slice(0, 7) : ''))

// While scrolling, find the date group currently pinned to the top and sync
// the day-navigation anchor to it.
function updateVisibleDate() {
  const headers = Array.from(rootEl.value?.querySelectorAll<HTMLElement>('[data-date-iso]') ?? [])
  if (headers.length === 0) return
  // A date header becomes "current" once its top edge meets the line where
  // stacked sticky headers settle: host top + pinned chrome + filter bar.
  const line = hostViewportTop() + topOffset.value + filterH.value + 1
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
  scrollHostToTop(true)
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
// Touch keyboards have no shift key, so range selection gets an explicit
// toggle: with it on, the second tap selects everything in between.
const rangeMode = ref(false)

function enterSelectMode() { selectMode.value = true; selected.value = new Set(); anchorId.value = null; rangeError.value = '' }
function exitSelectMode() { selectMode.value = false; selected.value = new Set(); anchorId.value = null; rangeError.value = ''; rangeMode.value = false }

function onPhotoClick(photo: Photo, e: MouseEvent) {
  if (selectMode.value) {
    if ((e.shiftKey || rangeMode.value) && anchorId.value !== null && props.rangeFn) {
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
  const cols = colsFor(w, isMobileShell.value ? gridCols.value : null)
  const cellSize = w / cols
  const visibleRows = Math.ceil(window.innerHeight / cellSize)
  return Math.max(30, cols * visibleRows * 3)
}

// ── Infinite scroll ──────────────────────────────────
// See lib/infiniteScroll.ts for why this is a re-checkable condition instead
// of a one-shot observer callback. It is drained from three places: the
// observer, the scroll handler, and the end of every completed page.

const RETRY_COOLDOWN_MS = 2000
let retryAfter = 0

function maybeLoadMore() {
  if (
    !shouldAutoLoad({
      hasMore: hasMore.value,
      loading: loading.value,
      sentinelTop: sentinel.value?.getBoundingClientRect().top ?? Infinity,
      viewportHeight: window.innerHeight,
      aheadPx: loadAheadPx(window.innerHeight),
      now: Date.now(),
      retryAfter,
    })
  ) {
    return
  }
  loadPage()
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

  let appended = 0
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
    appended = data.items.length
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
        const headerOffset = topOffset.value + filterH.value + dateHeaderH
        const hc = headCount.value

        setHostVisibility(false)

        const doScroll = () => {
          const imgs = rootEl.value?.querySelectorAll('img[src*="thumbnails"]') ?? []
          if (imgs.length > hc) {
            const target = imgs[hc] as HTMLElement
            const rect = target.getBoundingClientRect()
            const delta = rect.top - hostViewportTop() - headerOffset
            setScrollTop(Math.max(0, getScrollTop() + delta))
          }
          setHostVisibility(true)
        }

        requestAnimationFrame(doScroll)
        setTimeout(() => setHostVisibility(true), 2000)
      }
    }
  } catch (e: any) {
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    console.error('load failed', e)
    retryAfter = Date.now() + RETRY_COOLDOWN_MS
  } finally {
    // Only the newest request may clear the flag: reload() force-resets it, so
    // an aborted older request must not unlock a third concurrent load.
    if (myLoadId === loadId) loading.value = false
    // Drain: when the page that just landed did not push the sentinel out of
    // range (a short page, or the reader already parked at the bottom), fill
    // again instead of waiting for an intersection transition that may never
    // come. Only chain when progress was actually made.
    if (appended > 0 && hasMore.value) requestAnimationFrame(maybeLoadMore)
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  loadPage()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) maybeLoadMore()
    },
    { rootMargin: `${loadAheadPx(window.innerHeight)}px` },
  )
  if (sentinel.value) observer.observe(sentinel.value)

  // Scroll-up detection for loading newer photos (skipped in single-page mode)
  if (!props.singlePage) {
    let ticking = false
    offScroll = onHostScroll(() => {
      if (ticking) return
      ticking = true
      requestAnimationFrame(() => {
        const top = getScrollTop()
        atTop.value = top < 50
        updateVisibleDate()
        maybeLoadMore()
        if (top < 100 && hasNewer.value && !loadingNewer.value && !jumpCooldown.value) {
          loadNewer()
        }
        ticking = false
      })
    })
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
  const oldHeight = hostScrollHeight()

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
    requestAnimationFrame(() => {
      scrollHostBy(hostScrollHeight() - oldHeight)
    })
  } catch (e: any) {
    if (e?.name === 'CanceledError') return
    console.error('load newer failed', e)
  } finally {
    loadingNewer.value = false
  }
}

function removePhotoById(id: number) {
  const idx = photos.value.findIndex(p => p.id === id)
  if (idx !== -1) { photos.value.splice(idx, 1); total.value = Math.max(0, total.value - 1) }
}

defineExpose({ removePhotoById, reload })

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  observer?.disconnect()
  offScroll?.()
  offScroll = null
  cancelPress()
  disarmSwallow?.()
  controller?.abort()
})
</script>
