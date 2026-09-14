<template>
  <!-- Shared dialog shell: centred card on desktop, bottom sheet on mobile. -->
  <Dialog :open="open" @close="close" class="relative z-50">
    <div class="fixed inset-0 bg-black/60 fade-in" aria-hidden="true" />

    <div
      class="fixed inset-0 flex items-end sm:items-center justify-center sm:p-4"
      @click.self="close"
    >
      <DialogPanel
        class="relative w-full sm:max-w-md flex flex-col bg-raised border border-line
               shadow-2xl shadow-black/50
               rounded-t-2xl border-b-0
               sm:rounded-xl sm:border-b
               max-h-[85svh] sm:max-h-[80svh]
               pb-[env(safe-area-inset-bottom)] sm:pb-0"
      >
        <div class="shrink-0 flex items-center gap-3 px-5 pt-4 pb-3">
          <span class="sm:hidden absolute left-1/2 -translate-x-1/2 top-1.5 h-1 w-9 rounded-full bg-line-strong" aria-hidden="true" />
          <DialogTitle v-if="title" class="font-display text-sm font-medium text-ink">{{ title }}</DialogTitle>
          <slot name="header" />
          <button
            type="button"
            class="ml-auto shrink-0 w-8 h-8 -mr-1.5 flex items-center justify-center rounded-full text-ink-3 hover:text-ink transition-colors duration-150"
            aria-label="关闭"
            @click="close"
          >✕</button>
        </div>

        <div class="flex-1 min-h-0 overflow-y-auto overscroll-contain px-5 pb-5">
          <slot />
        </div>

        <div v-if="$slots.footer" class="shrink-0 px-5 py-3 border-t border-line">
          <slot name="footer" />
        </div>
      </DialogPanel>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import { Dialog, DialogPanel, DialogTitle } from '@headlessui/vue'

defineProps<{ open: boolean; title?: string }>()
const emit = defineEmits<{ close: [] }>()

function close() {
  emit('close')
}
</script>
