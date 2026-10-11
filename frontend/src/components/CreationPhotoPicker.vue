<template>
  <AppModal :open="open" :wide="true" :title="creating ? '选择创作照片' : '管理创作照片'" @close="close">
    <label v-if="creating" class="block text-xs text-ink-2 mb-4">作品标题
      <input v-model="title" maxlength="100" class="input mt-2" placeholder="给这份作品起个名字" />
    </label>
    <div class="flex gap-1 border-b border-line pb-3 mb-4">
      <button v-for="item in ['当前照片', '找照片']" :key="item" class="pill" :class="tab === item ? 'bg-accent-soft text-accent-strong' : 'text-ink-3'" :aria-pressed="tab === item" @click="tab = item">{{ item }}<span v-if="item === '当前照片'"> · {{ ids.length }}</span></button>
    </div>
    <p v-if="error" role="alert" class="text-sm text-danger-ink mb-3">{{ error }}</p>
    <template v-if="tab === '找照片'">
      <form class="space-y-2 mb-4" @submit.prevent="find">
        <label class="sr-only" for="creation-photo-query">搜索创作照片</label>
        <div class="flex gap-2">
          <input id="creation-photo-query" ref="queryInput" v-model="query" class="input min-w-0 flex-1" placeholder="如：海边、教室、跳跃…" />
          <button class="btn-primary px-4 text-sm" :disabled="loading">搜索</button>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <select v-model="album" class="input min-w-0" aria-label="找照片的范围" @change="changeAlbum">
            <option value="">全部照片</option><option v-for="item in albums" :key="item.id" :value="String(item.id)">{{ item.title }}</option>
          </select>
          <select v-model="month" class="input min-w-0" aria-label="跳转到月份" :disabled="datesLoading" @change="jumpMonth">
            <option value="">{{ datesLoading ? '加载月份…' : '最新照片' }}</option>
            <option v-for="item in months" :key="item.month" :value="item.month">{{ item.month.slice(0, 4) }}年{{ Number(item.month.slice(5)) }}月 · {{ item.count }}张</option>
          </select>
        </div>
        <button v-if="datesError" type="button" class="text-xs text-ink-3 underline" @click="loadDates">月份加载失败，点击重试</button>
      </form>
      <p v-if="loading" role="status" class="text-xs text-ink-3 mb-3">正在寻找照片…</p>
      <p v-else-if="!results.length" class="text-xs text-ink-3 py-6 text-center">暂无照片，换个月份、描述或相册试试。</p>
      <div class="grid grid-cols-3 sm:grid-cols-4 gap-3">
        <button v-for="photo in results" :key="photo.id" type="button" class="text-left rounded-xl overflow-hidden border border-line bg-surface disabled:cursor-default" :class="{ 'creation-photo-added': ids.includes(photo.id) }" :aria-label="'添加照片 ' + photo.id" :disabled="ids.includes(photo.id) || ids.length >= 200 || busy" @click="add(photo.id)">
          <img :src="thumb(photo.id)" :alt="photo.file_name || '照片 ' + photo.id" loading="lazy" class="aspect-square w-full object-cover" />
          <span class="block text-xs px-2 py-2" :class="ids.includes(photo.id) ? 'text-ink-3' : 'text-ink-2'">{{ ids.includes(photo.id) ? '已加入' : '+ 加入作品' }}</span>
        </button>
      </div>
      <button v-if="cursor && !query.trim()" class="btn-ghost w-full py-2 text-xs mt-4" :disabled="loading" @click="more">加载更多照片</button>
    </template>
    <template v-else>
      <p class="text-xs text-ink-3 leading-relaxed mb-4">勾选用于成品的照片。旋转和移除仅影响这份作品，原照片保留。</p>
      <button v-if="!ids.length" class="btn-primary px-4 py-2 text-sm" @click="tab = '找照片'">寻找并添加照片</button>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <article v-for="id in ids" :key="id" class="rounded-xl border border-line overflow-hidden bg-surface">
          <div class="aspect-square relative overflow-hidden bg-black/15">
            <img :src="thumb(id)" :alt="'创作照片 ' + id" loading="lazy" class="w-full h-full object-contain" :style="{ transform: 'rotate(' + (rotations[id] || 0) + 'deg)' }" />
            <label class="absolute left-2 top-2 rounded-full bg-canvas/90 px-2 py-1 flex items-center gap-1 text-xs"><input v-model="selected" type="checkbox" :value="id" :disabled="busy" :aria-label="'用于成品 ' + id" />用于成品</label>
          </div>
          <div class="px-2 pt-2 text-xs text-ink-3">照片 {{ id }}<span v-if="rotations[id]"> · {{ rotations[id] }}°</span></div>
          <div class="flex gap-1 p-2">
            <button class="btn-ghost flex-1 py-2 min-h-10 text-xs" :aria-label="'旋转照片 ' + id" :disabled="busy" @click="rotate(id)">↻ 旋转</button>
            <button class="btn-ghost flex-1 py-1.5 text-xs text-danger-ink" :aria-label="'从作品移除照片 ' + id" :disabled="busy" @click="remove(id)">移除</button>
          </div>
        </article>
      </div>
    </template>
    <template #footer>
      <p class="text-xs text-ink-3 mb-3">{{ ids.length }}/200 张 · 成品使用 {{ selected.length }} 张</p>
      <div class="flex gap-2">
        <button class="btn-ghost px-4 py-2 text-sm" :disabled="busy" @click="close">取消</button>
        <button class="btn-primary flex-1 py-2 text-sm" :disabled="busy || !ids.length || !selected.length" @click="save">{{ busy ? '保存中…' : creating ? '创建作品' : '保存选图' }}</button>
      </div>
    </template>
  </AppModal>
</template>
<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import AppModal from '@/components/AppModal.vue'
import { THUMB_BASE } from '@/api/client'
import { fetchPhotos, fetchPhotoDates, type DateCount, type Photo } from '@/api/photos'
import { fetchSearch } from '@/api/search'
import { fetchAlbums, fetchAlbumDates, type Album } from '@/api/albums'
import type { PhotoSelection } from '@/api/creations'
const props = withDefaults(defineProps<{ open: boolean; creating?: boolean; photoIds?: number[]; usedPhotoIds?: number[]; photoEdits?: Record<string, number>; initialAlbum?: number; busy?: boolean; saveError?: string }>(), { creating: false, busy: false })
const emit = defineEmits<{ close: []; save: [selection: PhotoSelection] }>()
const ids = ref<number[]>([]), selected = ref<number[]>([])
const rotations = ref<Record<string, number>>({})
const queryInput = ref<HTMLInputElement>()
const title = ref(''), tab = ref('当前照片'), query = ref(''), album = ref(''), month = ref('')
const albums = ref<Album[]>([]), results = ref<Photo[]>([])
const cursor = ref(''), loading = ref(false), error = ref('')
const dates = ref<DateCount[]>([]), datesLoading = ref(false), datesError = ref(false)
const months = computed(() => {
  const counts = new Map<string, number>()
  for (const item of dates.value) {
    const key = item.date.slice(0, 7)
    counts.set(key, (counts.get(key) || 0) + item.count)
  }
  return [...counts].sort(([a], [b]) => b.localeCompare(a)).map(([month, count]) => ({ month, count }))
})
let controller: AbortController | null = null
let datesController: AbortController | null = null
let albumsController: AbortController | null = null
function abortRequests() { controller?.abort(); datesController?.abort(); albumsController?.abort() }
async function loadDates() {
  datesController?.abort()
  const request = new AbortController(); datesController = request
  dates.value = []; datesLoading.value = true; datesError.value = false
  try {
    const data = album.value ? await fetchAlbumDates(Number(album.value), request.signal) : await fetchPhotoDates(request.signal)
    if (!request.signal.aborted) dates.value = data
  } catch {
    if (!request.signal.aborted) datesError.value = true
  } finally { if (datesController === request) datesLoading.value = false }
}
function thumb(id: number) { return THUMB_BASE + '/' + id + '.webp' }
function close() { if (!props.busy) emit('close') }
function add(id: number) { if (!ids.value.includes(id) && ids.value.length < 200) { ids.value.push(id); selected.value.push(id) } }
function rotate(id: number) { rotations.value[id] = ((rotations.value[id] || 0) + 90) % 360 }
function remove(id: number) { ids.value = ids.value.filter(x => x !== id); selected.value = selected.value.filter(x => x !== id); delete rotations.value[id] }
function save() { emit('save', { photo_ids: [...ids.value], used_photo_ids: [...selected.value], rotations: { ...rotations.value }, ...(props.creating ? { title: title.value.trim() || '照片作品' } : {}) }) }
async function search(append = false) {
  controller?.abort()
  const request = new AbortController(); controller = request
  loading.value = true; error.value = ''
  if (!append) { results.value = []; cursor.value = '' }
  try {
    const q = query.value.trim()
    if (q) {
      const data = await fetchSearch({ q, album_id: album.value ? Number(album.value) : undefined, limit: 60 }, request.signal)
      if (!request.signal.aborted) { results.value = data.items; cursor.value = '' }
    } else {
      const params = { limit: 60, cursor: append ? cursor.value : undefined, album_id: album.value || undefined, month: month.value || undefined }
      const data = await fetchPhotos(params, request.signal)
      if (!request.signal.aborted) {
        // Timeline jumps preload newer neighbours; start this picker at the chosen month.
        const items = data.items.slice(data.head_count || 0)
        results.value = append ? [...results.value, ...items] : items
        cursor.value = data.next_cursor
      }
    }
  } catch {
    if (!request.signal.aborted) error.value = '照片加载失败，请重试搜索。'
  } finally { if (controller === request) loading.value = false }
}
function find() { if (query.value.trim()) month.value = ''; queryInput.value?.blur(); void search() }
function jumpMonth() { query.value = ''; find() }
function changeAlbum() { month.value = ''; void loadDates(); find() }
function more() { void search(true) }
watch(() => props.saveError, value => { if (value) error.value = value })
watch(() => props.open, async open => {
  abortRequests()
  if (!open) return
  ids.value = [...(props.photoIds || [])]; selected.value = [...(props.usedPhotoIds || props.photoIds || [])]
  rotations.value = { ...(props.photoEdits || {}) }; title.value = ''; query.value = ''; month.value = ''
  tab.value = ids.value.length ? '当前照片' : '找照片'; album.value = props.initialAlbum ? String(props.initialAlbum) : ''
  error.value = ''; results.value = []; cursor.value = ''
  void search(); void loadDates()
  const request = new AbortController(); albumsController = request
  try { const data = await fetchAlbums(request.signal); if (!request.signal.aborted) albums.value = data.items } catch { /* Photo browsing remains usable if albums fail. */ }
})
onUnmounted(abortRequests)
</script>

<style scoped>
.creation-photo-added { opacity: .5; filter: grayscale(1); }
</style>
