<template>
  <nav
    class="fixed inset-x-0 bottom-0 z-40 bg-raised/85 backdrop-blur-xl border-t border-line/70 shadow-[0_-8px_24px_rgba(0,0,0,0.35)]"
    :style="{ paddingBottom: 'env(safe-area-inset-bottom)' }"
    aria-label="主导航"
  >
    <div class="relative flex h-[var(--ss-tabbar-h)]">
      <!-- Sliding highlight: one element translated between slots, so the
           selection slides instead of jumping. -->
      <span
        v-if="activeIndex >= 0"
        class="absolute inset-y-0 left-0 pointer-events-none transition-transform duration-300 ease-out"
        :style="{ width: `${100 / tabs.length}%`, transform: `translateX(${activeIndex * 100}%)` }"
        aria-hidden="true"
      >
        <span class="absolute left-1/2 top-1.5 -translate-x-1/2 w-11 h-7 rounded-full bg-accent-soft" />
      </span>

      <router-link
        v-for="tab in tabs"
        :key="tab.to"
        :to="tab.to"
        class="relative flex-1 flex flex-col items-center justify-center gap-1 transition-colors duration-200"
        :class="isActive(tab) ? 'text-accent-strong' : 'text-ink-3'"
        :aria-current="isActive(tab) ? 'page' : undefined"
        @click="onTap(tab)"
      >
        <svg
          class="w-[22px] h-[22px] transition-transform duration-200"
          :class="isActive(tab) ? 'scale-110' : 'scale-100'"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.6"
          aria-hidden="true"
        >
          <path
            v-for="d in tab.paths"
            :key="d"
            stroke-linecap="round"
            stroke-linejoin="round"
            :d="d"
          />
        </svg>
        <span class="text-[10px] font-medium leading-none">{{ tab.label }}</span>
      </router-link>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { authState } from '@/stores/auth'
import { activeTabIndex, tabsFor, type MobileTab } from '@/lib/tabs'
import { scrollHostToTop } from '@/lib/scrollHost'

const route = useRoute()
const tabs = computed(() => tabsFor(authState.user?.role))
const activeIndex = computed(() => activeTabIndex(tabs.value, route.path))

function isActive(tab: MobileTab): boolean {
  return tab.match(route.path)
}

// Tapping the tab you are already on is the standard "back to top" gesture.
function onTap(tab: MobileTab) {
  if (isActive(tab)) scrollHostToTop(true)
}
</script>
