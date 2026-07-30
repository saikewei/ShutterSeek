<template>
  <div v-if="months.length > 1" class="fixed right-3 top-1/2 -translate-y-1/2 z-40 flex">
    <button
      @click="open = !open"
      class="self-center w-5 h-16 rounded-l-lg bg-neutral-800 hover:bg-neutral-700 text-neutral-400 hover:text-white text-[10px] flex items-center justify-center shrink-0"
    >{{ open ? '◀' : '▶' }}</button>

    <div v-if="open" class="bg-neutral-800/95 backdrop-blur rounded-r-xl border border-neutral-700 overflow-hidden w-44 max-h-[60vh] flex flex-col">
      <div class="px-3 py-2 border-b border-neutral-700 text-xs text-neutral-400 shrink-0">
        时间轴
      </div>
      <div class="flex-1 overflow-y-auto py-1">
        <button
          v-for="m in months"
          :key="m.key"
          @click="$emit('jump', m.key)"
          class="w-full text-left px-3 py-1.5 text-xs transition-colors flex justify-between items-center hover:bg-neutral-700/50"
          :class="m.key === activeMonth ? 'text-white bg-neutral-700' : 'text-neutral-400'"
        >
          <span>{{ m.label }}</span>
          <span class="text-[10px] text-neutral-600">{{ m.count }}</span>
        </button>
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

interface MonthTick { key: string; label: string; count: number }

const months = computed<MonthTick[]>(() => {
  const buckets = new Map<string, MonthTick>()
  for (const d of props.dates) {
    const key = d.date.slice(0, 7) // YYYY-MM
    const dt = new Date(d.date)
    const label = `${dt.getFullYear()}年${dt.getMonth() + 1}月`
    const existing = buckets.get(key)
    if (existing) {
      existing.count += d.count
    } else {
      buckets.set(key, { key, label, count: d.count })
    }
  }
  return Array.from(buckets.values())
})
</script>
