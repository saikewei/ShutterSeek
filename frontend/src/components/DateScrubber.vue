<template>
  <!-- Desktop: floating collapsible panel on the right -->
  <div v-if="!embedded && yearGroups.length" class="fixed right-3 top-1/2 -translate-y-1/2 z-40 flex">
    <button
      @click="open = !open"
      class="self-center w-5 h-20 rounded-l-lg bg-raised hover:bg-surface text-ink-2 hover:text-ink text-[10px] flex items-center justify-center shrink-0"
    >{{ open ? '◀' : '▶' }}</button>

    <div v-if="open" class="bg-raised/95 backdrop-blur rounded-r-xl border border-line overflow-hidden w-52 max-h-[60vh] flex flex-col">
      <div class="flex-1 overflow-y-auto">
        <template v-for="g in yearGroups" :key="g.year">
          <!-- Year header — sticky within panel -->
          <div class="sticky top-0 z-10 px-4 py-1.5 bg-raised/95 backdrop-blur border-b border-line/60">
            <span class="text-base font-semibold font-display text-ink">{{ g.year }}年</span>
          </div>
          <!-- Months -->
          <button
            v-for="m in g.months"
            :key="m.key"
            @click="$emit('jump', m.key)"
            class="w-full text-left pl-7 pr-4 py-2 transition-colors flex justify-between items-center hover:bg-white/5"
            :class="m.key === activeMonth ? 'text-accent-strong bg-accent-soft font-medium' : 'text-ink-2'"
          >
            <span class="text-base">{{ m.label }}</span>
            <span class="text-xs text-ink-2">{{ m.count }}</span>
          </button>
        </template>
      </div>
    </div>
  </div>

  <!-- Embedded (mobile): always-visible content, no collapse toggle — for use inside a modal -->
  <div v-else-if="yearGroups.length" class="bg-raised/95 backdrop-blur border border-line rounded-xl overflow-hidden w-full max-h-[60vh] flex flex-col">
    <div class="flex-1 overflow-y-auto">
      <template v-for="g in yearGroups" :key="g.year">
        <!-- Year header — sticky within panel -->
        <div class="sticky top-0 z-10 px-4 py-1.5 bg-raised/95 backdrop-blur border-b border-line/60">
          <span class="text-base font-semibold font-display text-ink">{{ g.year }}年</span>
        </div>
        <!-- Months -->
        <button
          v-for="m in g.months"
          :key="m.key"
          @click="$emit('jump', m.key)"
          class="w-full text-left pl-7 pr-4 py-2 transition-colors flex justify-between items-center hover:bg-white/5"
          :class="m.key === activeMonth ? 'text-accent-strong bg-accent-soft font-medium' : 'text-ink-2'"
        >
          <span class="text-base">{{ m.label }}</span>
          <span class="text-xs text-ink-2">{{ m.count }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

export interface DatePoint { date: string; count: number; loaded: boolean }

const props = defineProps<{
  dates: DatePoint[]
  activeMonth?: string
  embedded?: boolean
}>()

defineEmits<{ jump: [monthKey: string] }>()

const open = ref(false)

interface MonthEntry { key: string; label: string; count: number }
interface YearGroup { year: number; months: MonthEntry[] }

const yearGroups = computed<YearGroup[]>(() => {
  const buckets = new Map<string, MonthEntry>()
  for (const d of props.dates) {
    const key = d.date.slice(0, 7)
    const dt = new Date(d.date)
    const label = `${dt.getMonth() + 1}月`
    const existing = buckets.get(key)
    if (existing) {
      existing.count += d.count
    } else {
      buckets.set(key, { key, label, count: d.count })
    }
  }
  const byYear = new Map<number, MonthEntry[]>()
  for (const m of buckets.values()) {
    const year = parseInt(m.key.slice(0, 4))
    const arr = byYear.get(year) || []
    arr.push(m)
    byYear.set(year, arr)
  }
  return Array.from(byYear.entries())
    .sort(([a], [b]) => b - a)
    .map(([year, months]) => ({ year, months }))
})
</script>
