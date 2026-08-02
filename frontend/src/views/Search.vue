<template>
  <div class="h-full flex flex-col bg-neutral-950 text-white">
    <!-- 搜索条 -->
    <div class="sticky top-0 z-30 px-4 py-3 border-b border-neutral-800 bg-neutral-900/95 backdrop-blur">
      <div class="flex items-center gap-2">
        <input
          v-model="query"
          class="flex-1 min-w-0 rounded-lg bg-neutral-800 border border-neutral-700 px-3 py-2 text-sm outline-none focus:border-neutral-500 disabled:opacity-50"
          placeholder="输入描述，搜索你的照片"
          :disabled="status === 'loading'"
          @keydown.enter="doSearch"
        />
        <button
          class="shrink-0 rounded-lg bg-neutral-200 text-neutral-900 px-4 py-2 text-sm font-medium disabled:opacity-50"
          :disabled="status === 'loading' || !query.trim()"
          @click="doSearch"
        >{{ status === 'loading' ? '搜索中…' : '搜索' }}</button>
      </div>
      <div v-if="albumID" class="mt-2 flex items-center gap-2 text-xs text-neutral-300">
        <span class="rounded-full bg-neutral-800 px-2 py-1">当前范围：{{ albumTitle(albumID) }}</span>
        <button class="text-neutral-500 hover:text-white transition-colors" @click="clearAlbum">× 清除</button>
      </div>
    </div>

    <!-- 内容区 -->
    <div class="flex-1 overflow-auto">
      <div v-if="status === 'idle'" class="h-full flex flex-col items-center justify-center text-neutral-500">
        <p class="text-base">输入描述，搜索你的照片</p>
        <p class="mt-1 text-sm">如：海边、猫、雪景</p>
      </div>

      <div v-else-if="status === 'loading'" class="flex items-center justify-center py-16">
        <div class="animate-spin h-6 w-6 border-2 border-neutral-500 border-t-white rounded-full"></div>
      </div>

      <div v-else-if="status === 'error'" class="p-4">
        <div class="rounded-lg bg-red-900/40 border border-red-800 px-4 py-3 text-sm">{{ errorMsg }}</div>
        <button v-if="canRetry" class="mt-3 text-sm text-neutral-300 underline" @click="doSearch">重试</button>
      </div>

      <div v-else-if="items.length === 0" class="h-full flex items-center justify-center text-sm text-neutral-500">
        未找到匹配的照片，换个词试试
      </div>

      <PhotoGrid
        v-else
        :fetch-fn="wrapFetch"
        :album-titles="albumTitles"
        single-page
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchSearch, type SearchPhotoItem, type SearchResponse } from '@/api/search'
import { fetchAlbums } from '@/api/albums'
import PhotoGrid from '@/components/PhotoGrid.vue'

const route = useRoute()
const router = useRouter()

const query = ref('')
const activeQuery = ref('')
const albumID = ref(0)
const items = ref<SearchPhotoItem[]>([])
const total = ref(0)
const status = ref<'idle' | 'loading' | 'success' | 'error'>('idle')
const errorMsg = ref('')
const canRetry = ref(false)
const albumTitles = ref<Record<number, string>>({})
let controller: AbortController | null = null

function albumTitle(id: number): string {
  return albumTitles.value[id] || `相册 ${id}`
}

// 从 URL 初始化；q 与 activeQuery 相同且 album 未变时跳过（防止 self-replace 重复搜索）
function readURL() {
  const q = typeof route.query.q === 'string' ? route.query.q : ''
  const a = Number(route.query.album_id) || 0
  query.value = q
  const changed = q !== '' && (q !== activeQuery.value || albumID.value !== a)
  albumID.value = a
  if (changed) doSearch()
}

function syncURL() {
  const q: Record<string, string> = {}
  if (activeQuery.value) q.q = activeQuery.value
  if (albumID.value) q.album_id = String(albumID.value)
  router.replace({ path: '/search', query: q })
}

async function doSearch() {
  const q = query.value.trim()
  if (!q) return
  controller?.abort()
  controller = new AbortController()
  activeQuery.value = q
  status.value = 'loading'
  canRetry.value = false
  syncURL()
  try {
    const data = await fetchSearch(
      { q, album_id: albumID.value || undefined, limit: 200 },
      controller.signal,
    )
    items.value = data.items
    total.value = data.total
    status.value = 'success'
  } catch (e: any) {
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    if (e?.response?.status === 403) {
      errorMsg.value = '无权访问该相册'
    } else if (e?.response?.status === 503) {
      errorMsg.value = '搜索服务不可用，请稍后重试'
      canRetry.value = true
    } else {
      errorMsg.value = '搜索失败，请稍后重试'
    }
    status.value = 'error'
  }
}

function clearAlbum() {
  albumID.value = 0
  if (activeQuery.value) doSearch()
}

// 供 Task 4 的 PhotoGrid 使用：适配 fetchFn 签名（补 next_cursor）
function wrapFetch(
  params: { limit: number; cursor?: string },
  signal?: AbortSignal,
): Promise<SearchResponse & { next_cursor: string }> {
  return fetchSearch(
    { q: activeQuery.value, album_id: albumID.value || undefined, limit: params.limit },
    signal,
  ).then((data) => ({ ...data, next_cursor: '' }))
}

watch(() => route.query, readURL)

onMounted(async () => {
  try {
    const data = await fetchAlbums()
    const map: Record<number, string> = {}
    for (const a of data.items) map[a.id] = a.title
    albumTitles.value = map
  } catch {
    // 无相册标题不影响搜索
  }
  readURL()
})
</script>
