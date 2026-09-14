<template>
  <header
    class="sticky top-0 z-40 bg-canvas/85 backdrop-blur-xl border-b transition-colors duration-200"
    :class="scrolled ? 'border-line/70' : 'border-transparent'"
    :style="{ paddingTop: 'env(safe-area-inset-top)' }"
  >
    <div class="h-[var(--ss-topbar-h)] px-4 flex items-center gap-3">
      <h1 v-if="isHome" class="font-display text-[15px] font-semibold tracking-wide text-ink">ShutterSeek</h1>
      <h1 v-else class="text-[15px] font-medium text-ink truncate">{{ title }}</h1>

      <button
        type="button"
        class="ml-auto -mr-1.5 w-9 h-9 shrink-0 rounded-full flex items-center justify-center text-ink-3 hover:text-ink hover:bg-white/5 active:scale-95 transition-all duration-150"
        aria-label="账号与设置"
        @click="meOpen = true"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z"
          />
        </svg>
      </button>
    </div>

    <MeSheet :open="meOpen" @close="meOpen = false" />
  </header>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import MeSheet from '@/components/mobile/MeSheet.vue'
import { shellTitle } from '@/lib/tabs'
import { getScrollTop, onHostScroll } from '@/lib/scrollHost'

const route = useRoute()

// The hairline only appears once the page has actually moved: an empty header
// over the top of the grid looks lighter than a permanently drawn border.
const scrolled = ref(false)
const meOpen = ref(false)

const isHome = computed(() => route.path === '/')
const title = computed(() => shellTitle(route.path))

let offScroll: (() => void) | null = null

onMounted(() => {
  offScroll = onHostScroll(() => {
    const next = getScrollTop() > 8
    if (next !== scrolled.value) scrolled.value = next
  })
  scrolled.value = getScrollTop() > 8
})

onUnmounted(() => {
  offScroll?.()
  offScroll = null
})
</script>
