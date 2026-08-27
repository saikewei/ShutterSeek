<template>
  <div class="p-4">
    <div class="flex items-center justify-between mb-4 max-w-3xl mx-auto">
      <h1 class="text-base font-medium text-ink">用户日志</h1>
      <p class="text-xs text-ink-3">共 {{ total.toLocaleString() }} 条</p>
    </div>

    <div v-if="loading && logs.length === 0" class="text-center text-ink-3 py-12 text-sm">Loading...</div>
    <div v-else class="max-w-3xl mx-auto">
      <div class="bg-surface rounded-lg border border-line overflow-hidden">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-xs text-ink-3 border-b border-line">
              <th class="px-3 py-2 font-medium">时间</th>
              <th class="px-3 py-2 font-medium">用户</th>
              <th class="px-3 py-2 font-medium">事件</th>
              <th class="px-3 py-2 font-medium">IP</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id" class="border-b border-line last:border-0">
              <td class="px-3 py-2 text-ink-2 whitespace-nowrap">{{ formatTime(log.created_at) }}</td>
              <td class="px-3 py-2 text-ink">{{ log.username }}</td>
              <td class="px-3 py-2">
                <span
                  class="px-2 py-0.5 text-xs rounded-full"
                  :class="badgeClass(log.event_type)"
                >{{ label(log.event_type) }}</span>
              </td>
              <td class="px-3 py-2 text-ink-2">{{ log.ip || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-4 flex justify-center">
        <button
          v-if="logs.length < total"
          @click="loadMore"
          :disabled="loadingMore"
          class="btn-ghost px-4 py-1.5 text-xs disabled:opacity-50 disabled:cursor-not-allowed"
        >{{ loadingMore ? '加载中...' : '加载更多' }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchLogs, type UserLogEntry } from '@/api/auth'

const logs = ref<UserLogEntry[]>([])
const total = ref(0)
const loading = ref(true)
const loadingMore = ref(false)

function label(t: string): string {
  return t === 'login' ? '登录' : t === 'session' ? 'JWT会话' : '注销'
}

function badgeClass(t: string): string {
  if (t === 'login') return 'bg-success/15 text-success'
  if (t === 'session') return 'bg-accent-soft text-accent-strong'
  return 'bg-line-strong/40 text-ink-2'
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

onMounted(async () => {
  try {
    const data = await fetchLogs({ limit: 100 })
    logs.value = data.items
    total.value = data.total
  } catch { /* */ }
  finally { loading.value = false }
})

async function loadMore() {
  if (loadingMore.value || logs.value.length === 0) return
  loadingMore.value = true
  try {
    const last = logs.value[logs.value.length - 1]
    const data = await fetchLogs({ limit: 100, before_id: last.id })
    logs.value.push(...data.items)
    total.value = data.total
  } catch { /* */ }
  finally { loadingMore.value = false }
}
</script>
