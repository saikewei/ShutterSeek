<template>
  <div v-if="ticks.length > 1" class="fixed right-3 top-1/2 -translate-y-1/2 z-40 flex">
    <!-- Collapse toggle -->
    <button
      @click="open = !open"
      class="self-center w-5 h-20 rounded-l-lg bg-neutral-800 hover:bg-neutral-700 text-neutral-400 hover:text-white text-[10px] flex items-center justify-center transition-all shrink-0"
    >
      {{ open ? '◀' : '▶' }}
    </button>

    <!-- Panel -->
    <div
      v-if="open"
      class="bg-neutral-800/95 backdrop-blur rounded-r-xl border border-neutral-700 overflow-hidden w-40 max-h-[65vh] flex flex-col transition-all"
    >
      <div class="px-3 py-2 border-b border-neutral-700 flex items-center justify-between shrink-0">
        <span class="text-xs text-neutral-400">日期</span>
        <span class="text-[10px] text-neutral-600">{{ allDates.length }}</span>
      </div>

      <div
        ref="listRef"
        class="flex-1 overflow-y-auto py-1"
      >
        <button
          v-for="t in ticks"
          :key="t.key"
          @click="$emit('jump', t.label)"
          class="w-full text-left px-3 py-1.5 text-xs transition-colors flex items-center justify-between"
          :class="t.major
            ? 'text-neutral-300 font-medium hover:bg-neutral-700'
            : 'text-neutral-500 pl-6 hover:bg-neutral-700/50 hover:text-neutral-300'"
        >
          <span>{{ t.label }}</span>
          <span v-if="t.major" class="text-[10px] text-neutral-600">{{ t.count }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

export interface DatePoint {
  date: string
  count: number
  loaded: boolean
}

const props = defineProps<{
  dates: DatePoint[]
}>()

defineEmits<{
  jump: [label: string]
}>()

const open = ref(false)
const listRef = ref<HTMLElement | null>(null)

const allDates = computed(() => props.dates)

interface Tick {
  key: string
  label: string
  count: number
  major: boolean
}

const ticks = computed<Tick[]>(() => {
  const raw = allDates.value
  if (raw.length === 0) return []

  const now = new Date()
  const thisYear = now.getFullYear()
  const thisMonth = now.getMonth()

  // Aggregate counts by key
  const buckets = new Map<string, { label: string; count: number; major: boolean; sort: number }>()

  const add = (key: string, label: string, count: number, major: boolean, sort: number) => {
    const existing = buckets.get(key)
    if (existing) {
      existing.count += count
    } else {
      buckets.set(key, { label, count, major, sort })
    }
  }

  for (const d of raw) {
    const dt = new Date(d.date)
    const y = dt.getFullYear()
    const m = dt.getMonth()

    if (y === thisYear && m === thisMonth) {
      add(d.date, `${m + 1}月${dt.getDate()}日`, d.count, false, +dt)
    } else if (y === thisYear) {
      add(`${y}-${m}`, `${m + 1}月`, d.count, true, new Date(y, m, 1).getTime())
    } else if (y >= thisYear - 3) {
      add(`${y}`, `${y}年`, d.count, true, new Date(y, 0, 1).getTime())
    } else {
      const bucket = Math.floor(y / 5) * 5
      add(`${bucket}s`, `${bucket}s`, d.count, true, new Date(bucket, 0, 1).getTime())
    }
  }

  return Array.from(buckets.entries())
    .map(([key, v]) => ({ key, label: v.label, count: v.count, major: v.major }))
    .sort((a, b) => {
      // Sort by original date order (newest first = highest sort)
      const sa = buckets.get(a.key)!.sort
      const sb = buckets.get(b.key)!.sort
      return sb - sa
    })
})
</script>
