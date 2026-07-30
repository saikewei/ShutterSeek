<template>
  <div
    v-if="dates.length > 1"
    class="fixed right-0 top-0 bottom-0 w-5 z-40 group"
    @mouseleave="hovered = null"
  >
    <!-- Rail -->
    <div class="absolute right-1 top-2 bottom-2 w-1 bg-neutral-800 rounded-full">
      <!-- Markers -->
      <div
        v-for="(d, i) in markers"
        :key="d.date"
        class="absolute left-1/2 -translate-x-1/2 w-1 h-1 rounded-full transition-all"
        :class="d.loaded ? 'bg-neutral-400' : 'bg-neutral-700'"
        :style="{ top: pct(i) + '%' }"
        @mouseenter="hovered = d"
      />
    </div>

    <!-- Handle (visible on hover) -->
    <div
      v-if="hovered"
      class="absolute right-0.5 w-3 h-3 -translate-y-1/2 rounded-full bg-white shadow-lg transition-all"
      :style="{ top: pct(hoveredIdx) + '%' }"
    />

    <!-- Tooltip -->
    <Teleport to="body">
      <div
        v-if="hovered"
        class="fixed z-50 pointer-events-none text-xs bg-neutral-800 text-white px-2 py-1 rounded shadow-lg whitespace-nowrap transition-transform"
        :style="{ right: '28px', top: pct(hoveredIdx) + '%', transform: 'translateY(-50%)' }"
      >
        {{ hovered.date }} · {{ hovered.count }}
      </div>
    </Teleport>
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
  activeDate?: string  // currently visible date at top of viewport
}>()

const emit = defineEmits<{
  jump: [date: string]
}>()

const hovered = ref<DatePoint | null>(null)

const hoveredIdx = computed(() => {
  if (!hovered.value) return 0
  return props.dates.findIndex(d => d.date === hovered.value!.date)
})

// Decimate markers to avoid too many dots (~200 max)
const markers = computed(() => {
  const all = props.dates
  if (all.length <= 200) return all
  const step = Math.ceil(all.length / 200)
  return all.filter((_, i) => i % step === 0)
})

function pct(i: number): number {
  return (i / Math.max(props.dates.length - 1, 1)) * 100
}

function jumpTo(date: string) {
  emit('jump', date)
}
</script>
