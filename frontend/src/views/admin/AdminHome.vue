<template>
  <div class="p-3 sm:p-4 max-w-3xl mx-auto">
    <!-- Counters -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
      <div v-for="card in cards" :key="card.label" class="rounded-xl bg-raised border border-line p-3.5">
        <p class="font-display text-2xl leading-none text-ink tabular-nums">{{ card.value }}</p>
        <p class="mt-1.5 text-xs text-ink-3">{{ card.label }}</p>
      </div>
    </div>

    <p v-if="error" class="mt-3 text-xs text-danger-ink">{{ error }}</p>

    <!-- Dependency health -->
    <div class="mt-4 rounded-xl bg-raised border border-line divide-y divide-line overflow-hidden">
      <div v-for="row in health" :key="row.label" class="flex items-center gap-3 px-4 py-3">
        <span class="w-2 h-2 rounded-full shrink-0" :class="row.ok ? 'bg-success' : 'bg-danger'"></span>
        <span class="text-sm text-ink-2">{{ row.label }}</span>
        <span class="ml-auto text-xs tabular-nums" :class="row.ok ? 'text-success' : 'text-danger-ink'">
          {{ row.ok ? '正常' : '异常' }}
        </span>
      </div>
    </div>

    <!-- Entries -->
    <div class="mt-4 rounded-xl bg-raised border border-line overflow-hidden">
      <RouterLink
        v-for="entry in entries"
        :key="entry.to"
        :to="entry.to"
        class="flex items-center gap-3 px-4 py-3.5 border-b border-line last:border-0 hover:bg-white/5 active:bg-white/10 transition-colors"
      >
        <svg class="w-4 h-4 shrink-0 text-ink-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" :d="entry.icon" />
        </svg>
        <span class="text-sm text-ink">{{ entry.label }}</span>
        <span v-if="entry.hint" class="ml-auto text-xs text-ink-3 tabular-nums">{{ entry.hint }}</span>
        <span class="text-ink-3" :class="entry.hint ? 'ml-2' : 'ml-auto'">›</span>
      </RouterLink>
    </div>

    <p class="mt-4 text-[11px] text-ink-3 text-center leading-relaxed">
      已运行 {{ uptime }}<br />
      上传功能仅桌面端可用，移动端管理台不含上传
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { fetchStats, formatUptime, type SystemStats } from '@/api/admin'

const stats = ref<SystemStats | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    stats.value = await fetchStats()
  } catch {
    error.value = '统计数据加载失败，下拉或稍后重试'
  }
})

const cards = computed(() => [
  { label: '照片', value: fmt(stats.value?.photos) },
  { label: '相册', value: fmt(stats.value?.albums) },
  { label: '用户', value: fmt(stats.value?.users) },
  { label: '待用邀请码', value: fmt(stats.value?.invites_pending) },
])

const health = computed(() => [
  { label: 'PostgreSQL', ok: !!stats.value?.db_ok },
  { label: 'Redis', ok: !!stats.value?.redis_ok },
  { label: '向量服务', ok: !!stats.value?.embed_ok },
])

const entries = computed(() => [
  {
    to: '/albums',
    label: '相册管理',
    hint: fmt(stats.value?.albums),
    icon: 'M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z',
  },
  {
    to: '/admin/invites',
    label: '邀请码',
    hint: `${fmt(stats.value?.invites_pending)} / ${fmt(stats.value?.invites_total)}`,
    icon: 'M21 12a2.25 2.25 0 0 0-2.25-2.25H15a3 3 0 1 1-6 0H5.25A2.25 2.25 0 0 0 3 12m18 0v6a2.25 2.25 0 0 1-2.25 2.25H5.25A2.25 2.25 0 0 1 3 18v-6m18 0V9M3 12V9',
  },
  {
    to: '/admin/logs',
    label: '用户日志',
    hint: fmt(stats.value?.logs),
    icon: 'M9 12h6m-6 3.75h6M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5',
  },
  {
    to: '/admin/status',
    label: '系统状态',
    hint: '',
    icon: 'M10.5 6h9.75M10.5 6a1.5 1.5 0 1 1-3 0m3 0a1.5 1.5 0 1 0-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-9.75 0h9.75',
  },
])

const uptime = computed(() => (stats.value ? formatUptime(stats.value.uptime_seconds) : '…'))

function fmt(n: number | undefined): string {
  return n === undefined ? '…' : n.toLocaleString()
}
</script>
