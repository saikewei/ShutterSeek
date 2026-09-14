<template>
  <div class="p-3 sm:p-4 max-w-3xl mx-auto">
    <div class="flex items-baseline justify-between gap-3 mb-3">
      <p class="text-xs text-ink-3">已加载 {{ logs.length.toLocaleString() }} 条</p>
      <p class="text-xs text-ink-3">共 {{ total.toLocaleString() }} 条</p>
    </div>

    <!-- Event filter (applies to the rows already loaded) -->
    <div class="no-scrollbar flex items-center gap-1.5 overflow-x-auto pb-3">
      <button
        v-for="f in filters"
        :key="f.key"
        class="shrink-0 px-3.5 py-1.5 text-xs rounded-full transition-colors"
        :class="activeFilter === f.key ? 'bg-ink text-[#1C1208]' : 'bg-surface text-ink-2 hover:text-ink'"
        @click="activeFilter = f.key"
      >{{ f.label }}</button>
    </div>

    <div v-if="loading && logs.length === 0" class="text-center text-ink-3 py-12 text-sm">加载中...</div>
    <div v-else-if="visibleLogs.length === 0" class="text-center text-ink-3 py-12 text-sm">
      {{ activeFilter === 'all' ? '暂无日志' : '已加载的记录里没有这一类' }}
    </div>
    <div v-else class="space-y-1.5">
      <div
        v-for="log in visibleLogs"
        :key="log.id"
        class="rounded-xl bg-surface border border-line px-3.5 py-2.5"
      >
        <div class="flex items-center gap-2">
          <span class="shrink-0 px-2 py-0.5 text-[11px] rounded-full" :class="badgeClass(log.event_type)">
            {{ label(log.event_type) }}
          </span>
          <span class="text-sm text-ink truncate">{{ log.username }}</span>
          <span class="ml-auto shrink-0 text-[11px] text-ink-3 tabular-nums">{{ formatTime(log.created_at) }}</span>
        </div>
        <p class="mt-1 text-[11px] text-ink-3 truncate">IP {{ log.ip || '—' }}</p>
      </div>
    </div>

    <div ref="sentinel" class="py-6 flex justify-center">
      <span v-if="loadingMore" class="text-xs text-ink-3">加载中...</span>
      <span v-else-if="!hasMore && logs.length" class="text-xs text-ink-3">没有更多了</span>
      <button v-else-if="hasMore" class="btn-ghost px-4 py-1.5 text-xs" @click="loadMore">加载更多</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { fetchLogs, type UserLogEntry } from '@/api/auth'

const PAGE_SIZE = 100

const logs = ref<UserLogEntry[]>([])
const total = ref(0)
const loading = ref(true)
const loadingMore = ref(false)
const hasMore = ref(true)
const activeFilter = ref('all')
const sentinel = ref<HTMLElement | null>(null)

let observer: IntersectionObserver | null = null

const filters = [
  { key: 'all', label: '全部' },
  { key: 'login', label: '登录' },
  { key: 'session', label: 'JWT会话' },
  { key: 'upload', label: '上传' },
  { key: 'logout', label: '注销' },
]

const visibleLogs = computed(() =>
  activeFilter.value === 'all'
    ? logs.value
    : logs.value.filter((l) => l.event_type === activeFilter.value),
)

function label(t: string): string {
  if (t === 'login') return '登录'
  if (t === 'session') return 'JWT会话'
  if (t === 'upload') return '上传'
  return '注销'
}

function badgeClass(t: string): string {
  if (t === 'login') return 'bg-success/15 text-success'
  if (t === 'session') return 'bg-accent-soft text-accent-strong'
  if (t === 'upload') return 'bg-white/5 text-ink-2'
  return 'bg-line-strong/40 text-ink-2'
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

onMounted(async () => {
  try {
    const data = await fetchLogs({ limit: PAGE_SIZE })
    logs.value = data.items
    total.value = data.total
    hasMore.value = data.items.length >= PAGE_SIZE
  } catch {
    hasMore.value = false
  } finally {
    loading.value = false
  }

  observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting && hasMore.value && !loadingMore.value) loadMore()
    },
    { rootMargin: '400px' },
  )
  if (sentinel.value) observer.observe(sentinel.value)
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})

async function loadMore() {
  if (loadingMore.value || !hasMore.value || logs.value.length === 0) return
  loadingMore.value = true
  try {
    const last = logs.value[logs.value.length - 1]
    const data = await fetchLogs({ limit: PAGE_SIZE, before_id: last.id })
    logs.value.push(...data.items)
    total.value = data.total
    hasMore.value = data.items.length >= PAGE_SIZE
  } catch {
    hasMore.value = false
  } finally {
    loadingMore.value = false
  }
}
</script>
