<template>
  <AppModal :open="open" title="选择创作相册" @close="close">
    <p class="text-sm text-ink-2 mb-3">选择一个相册，将其中的 1–200 张照片用于新作品。</p>
    <p v-if="loading" class="text-sm text-ink-3 py-6 text-center" role="status">正在加载相册…</p>
    <template v-else>
      <p v-if="error" role="alert" class="text-sm text-danger-ink mb-3">{{ error }}</p>
      <button v-if="loadFailed" class="btn-ghost px-3 py-2" @click="loadAlbums">重新加载</button>
      <p v-else-if="albums.length === 0" class="text-sm text-ink-3 py-6 text-center">暂无相册，请先在相册页面创建相册并添加照片。</p>
      <label v-if="!loadFailed && albums.length" class="block text-xs text-ink-2 mb-4">作品名称
        <input v-model="title" maxlength="100" class="input mt-2" :disabled="creating" :placeholder="selected?.title || '给这份作品起个名字'" />
      </label>
      <div v-if="!loadFailed && albums.length" class="space-y-2">
        <div v-for="album in albums" :key="album.id">
          <button
            type="button"
            :aria-label="'选择相册：' + album.title"
            :aria-pressed="selectedId === album.id"
            :disabled="creating || !eligible(album)"
            class="w-full flex items-center gap-3 rounded-xl border p-2.5 text-left transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            :class="selectedId === album.id ? 'border-accent bg-accent-soft' : 'border-line hover:bg-surface'"
            @click="selectedId = album.id"
          >
            <img v-if="album.cover_url" :src="album.cover_url" alt="" loading="lazy" class="w-14 h-14 shrink-0 object-cover rounded-lg" />
            <span v-else class="w-14 h-14 shrink-0 rounded-lg bg-surface flex items-center justify-center text-ink-3" aria-hidden="true">▧</span>
            <span class="flex-1 min-w-0">
              <span class="block text-sm font-medium text-ink truncate">{{ album.title }}</span>
              <span class="block text-xs text-ink-3 mt-1">{{ album.photo_count }} 张照片</span>
              <span v-if="album.photo_count === 0" class="block text-xs text-ink-3 mt-1">暂无照片</span>
              <span v-else-if="album.photo_count > 200" class="block text-xs text-ink-3 mt-1">超过 200 张，请选取部分照片</span>
            </span>
            <span v-if="selectedId === album.id" class="text-accent-strong shrink-0" aria-hidden="true">✓</span>
          </button>
          <button v-if="album.photo_count > 0 && !creating" class="inline-block text-xs text-accent-strong px-3 py-2" @click="emit('browse', album.id)">从此相册挑选照片</button>
        </div>
      </div>
    </template>

    <template #footer>
      <div class="flex items-center gap-2">
        <p class="flex-1 min-w-0 text-xs text-ink-3">{{ selected ? '将使用 ' + selected.photo_count + ' 张照片' : '请选择相册' }}</p>
        <button class="btn-ghost px-3 py-2 text-sm" :disabled="creating" @click="close">取消</button>
        <button class="btn-primary px-4 py-2 text-sm" :disabled="loading || creating || !selected" @click="createWork">{{ creating ? '创建中…' : '创建作品' }}</button>
      </div>
    </template>
  </AppModal>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import AppModal from '@/components/AppModal.vue'
import { fetchAlbums, type Album } from '@/api/albums'
import { createCreation } from '@/api/creations'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; created: [id: string]; browse: [id: number] }>()
const albums = ref<Album[]>([])
const selectedId = ref<number | null>(null)
const title = ref('')
const loading = ref(false)
const creating = ref(false)
const error = ref('')
const loadFailed = ref(false)
let controller: AbortController | null = null

function eligible(album: Album) { return album.photo_count > 0 && album.photo_count <= 200 }
const selected = computed(() => albums.value.find(album => album.id === selectedId.value && eligible(album)))

function close() { if (!creating.value) emit('close') }
async function loadAlbums() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true
  loadFailed.value = false
  error.value = ''
  try {
    const result = await fetchAlbums(request.signal)
    if (!request.signal.aborted) albums.value = result.items
  } catch {
    if (!request.signal.aborted) { error.value = '相册加载失败，请重试'; loadFailed.value = true }
  } finally {
    if (controller === request) loading.value = false
  }
}
async function createWork() {
  const album = selected.value
  if (!album || loading.value || creating.value) return
  creating.value = true
  error.value = ''
  try {
    const work = await createCreation({ album_id: album.id, title: title.value.trim() || album.title })
    emit('created', work.id)
  } catch (e) {
    const value = e as { response?: { data?: { error?: string } } }
    error.value = value.response?.data?.error || '创建作品失败，请重试'
  } finally { creating.value = false }
}
watch(() => props.open, (open) => {
  if (!open) { controller?.abort(); controller = null; return }
  albums.value = []
  selectedId.value = null; title.value = ''
  void loadAlbums()
}, { immediate: true })
onUnmounted(() => controller?.abort())
</script>
