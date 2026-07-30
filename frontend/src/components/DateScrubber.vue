<template>
  <div v-if="yearGroups.length" class="fixed right-3 top-1/2 -translate-y-1/2 z-40 flex">
    <button
      @click="open = !open"
      class="self-center w-5 h-16 rounded-l-lg bg-neutral-800 hover:bg-neutral-700 text-neutral-400 hover:text-white text-[10px] flex items-center justify-center shrink-0"
    >{{ open ? '◀' : '▶' }}</button>

    <div v-if="open" class="bg-neutral-800/95 backdrop-blur rounded-r-xl border border-neutral-700 overflow-hidden w-40 max-h-[60vh] flex flex-col">
      <div class="sticky top-0 z-10 px-3 py-1.5 bg-neutral-800/95 backdrop-blur border-b border-neutral-700/50 text-xs font-medium text-neutral-300 shrink-0">
        {{ activeYear }}年
      </div>
      <div ref="listRef" class="flex-1 overflow-y-auto" @scroll="onScroll">
        <template v-for="g in yearGroups" :key="g.year">
          <button
            v-for="m in g.months"
            :key="m.key"
            :data-month-key="m.key"
            @click="$emit('jump', m.key)"
            class="w-full text-left px-4 py-1.5 transition-colors flex justify-between items-center hover:bg-neutral-700/50"
            :class="m.key === activeMonth ? 'text-white bg-neutral-700' : 'text-neutral-400'"
          >
            <span class="text-sm">{{ m.label }}</span>
            <span class="text-[10px] text-neutral-600">{{ m.count }}</span>
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

export interface DatePoint { date: string; count: number; loaded: boolean }

const props = defineProps<{
  dates: DatePoint[]
  activeMonth?: string
}>()

defineEmits<{ jump: [monthKey: string] }>()

const open = ref(false)
const listRef = ref<HTMLElement | null>(null)
const activeYear = ref(new Date().getFullYear())

interface MonthEntry { key: string; label: string; count: number; year: number }
interface YearGroup { year: number; months: MonthEntry[] }

const yearGroups = computed<YearGroup[]>(() => {
  const buckets = new Map<string, MonthEntry>()
  for (const d of props.dates) {
    const key = d.date.slice(0, 7)
    const dt = new Date(d.date)
    const year = dt.getFullYear()
    const label = `${dt.getMonth() + 1}月`
    const existing = buckets.get(key)
    if (existing) {
      existing.count += d.count
    } else {
      buckets.set(key, { key, label, count: d.count, year })
    }
  }
  const byYear = new Map<number, MonthEntry[]>()
  for (const m of buckets.values()) {
    const arr = byYear.get(m.year) || []
    arr.push(m)
    byYear.set(m.year, arr)
  }
  return Array.from(byYear.entries())
    .sort(([a], [b]) => b - a)
    .map(([year, months]) => ({ year, months }))
})

function onScroll() {
  const el = listRef.value
  if (!el) return
  // Find the first visible month button to determine active year
  const btns = el.querySelectorAll('button')
  const containerTop = el.getBoundingClientRect().top + 32 // sticky header offset
  for (const b of btns) {
    const rect = b.getBoundingClientRect()
    if (rect.bottom > containerTop) {
      // Parse year from the month's data attribute
      const key = (b as HTMLElement).dataset.monthKey
      if (key) activeYear.value = parseInt(key.slice(0, 4))
      break
    }
  }
}
</script>
