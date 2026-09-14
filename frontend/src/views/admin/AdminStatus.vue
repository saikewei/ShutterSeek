<template>
  <div class="p-3 sm:p-4 max-w-3xl mx-auto">
    <div class="flex items-center justify-between mb-3">
      <p class="text-xs text-ink-3">只读诊断信息</p>
      <button
        class="btn-ghost px-3 py-1.5 text-xs"
        :disabled="loading"
        @click="load"
      >{{ loading ? '刷新中...' : '刷新' }}</button>
    </div>

    <p v-if="error" class="mb-3 text-xs text-danger-ink">{{ error }}</p>

    <!-- Dependencies -->
    <h2 class="text-xs text-ink-3 mb-2">依赖服务</h2>
    <div class="rounded-xl bg-raised border border-line divide-y divide-line overflow-hidden">
      <div v-for="row in health" :key="row.label" class="flex items-center gap-3 px-4 py-3">
        <span class="w-2 h-2 rounded-full shrink-0" :class="row.ok ? 'bg-success' : 'bg-danger'"></span>
        <div class="min-w-0">
          <p class="text-sm text-ink-2">{{ row.label }}</p>
          <p class="text-[11px] text-ink-3">{{ row.detail }}</p>
        </div>
        <span class="ml-auto shrink-0 text-xs" :class="row.ok ? 'text-success' : 'text-danger-ink'">
          {{ row.ok ? '正常' : '异常' }}
        </span>
      </div>
    </div>

    <!-- Counters -->
    <h2 class="text-xs text-ink-3 mt-5 mb-2">数据量</h2>
    <div class="rounded-xl bg-raised border border-line divide-y divide-line overflow-hidden">
      <div v-for="row in counters" :key="row.label" class="flex items-center px-4 py-3">
        <span class="text-sm text-ink-2">{{ row.label }}</span>
        <span class="ml-auto text-sm text-ink tabular-nums">{{ row.value }}</span>
      </div>
    </div>

    <!-- Process -->
    <h2 class="text-xs text-ink-3 mt-5 mb-2">进程</h2>
    <div class="rounded-xl bg-raised border border-line divide-y divide-line overflow-hidden">
      <div class="flex items-center px-4 py-3">
        <span class="text-sm text-ink-2">已运行</span>
        <span class="ml-auto text-sm text-ink tabular-nums">{{ uptime }}</span>
      </div>
      <div class="flex items-center px-4 py-3">
        <span class="text-sm text-ink-2">启动于</span>
        <span class="ml-auto text-sm text-ink tabular-nums">{{ startedAt }}</span>
      </div>
    </div>

    <p class="mt-4 text-[11px] text-ink-3 text-center leading-relaxed">
      本页只读，不会修改任何数据
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { fetchStats, formatUptime, type SystemStats } from '@/api/admin'

const stats = ref<SystemStats | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    stats.value = await fetchStats()
  } catch {
    error.value = '读取失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function n(v: number | undefined): string {
  return v === undefined ? '—' : v.toLocaleString()
}

const health = computed(() => [
  { label: 'PostgreSQL', detail: '照片、相册与用户数据', ok: !!stats.value?.db_ok },
  { label: 'Redis', detail: '首页缓存与查询向量缓存', ok: !!stats.value?.redis_ok },
  { label: '向量服务 /healthz', detail: '文本搜图的 embedding sidecar', ok: !!stats.value?.embed_ok },
])

const counters = computed(() => [
  { label: '照片总数', value: n(stats.value?.photos) },
  { label: '图片向量', value: n(stats.value?.embeddings) },
  { label: '相册（公开）', value: `${n(stats.value?.albums)}（${n(stats.value?.public_albums)}）` },
  { label: '用户（管理员）', value: `${n(stats.value?.users)}（${n(stats.value?.admins)}）` },
  { label: '邀请码（未使用）', value: `${n(stats.value?.invites_total)}（${n(stats.value?.invites_pending)}）` },
  { label: '日志条数', value: n(stats.value?.logs) },
])

const uptime = computed(() => (stats.value ? formatUptime(stats.value.uptime_seconds) : '—'))
const startedAt = computed(() => {
  if (!stats.value?.started_at) return '—'
  const d = new Date(stats.value.started_at)
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
})
</script>
