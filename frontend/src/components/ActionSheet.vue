<template>
  <AppModal :open="open" :title="title" @close="close">
    <div class="space-y-1.5">
      <button
        v-for="action in actions"
        :key="action.key"
        type="button"
        class="w-full text-left px-4 py-3 rounded-xl bg-surface text-sm transition-colors duration-150"
        :class="action.danger ? 'text-danger-ink hover:bg-danger-soft' : 'text-ink-2 hover:text-ink hover:bg-line-strong'"
        @click="select(action.key)"
      >{{ action.label }}</button>

      <button
        type="button"
        class="w-full px-4 py-3 rounded-xl text-sm text-ink-3 hover:text-ink transition-colors duration-150"
        @click="close"
      >取消</button>
    </div>
  </AppModal>
</template>

<script setup lang="ts">
import AppModal from '@/components/AppModal.vue'

export interface SheetAction {
  key: string
  label: string
  danger?: boolean
}

defineProps<{
  open: boolean
  title?: string
  actions: SheetAction[]
}>()

const emit = defineEmits<{ close: []; select: [key: string] }>()

function close() {
  emit('close')
}

// `select` fires before `close` so the parent can still read the item the
// sheet was opened for.
function select(key: string) {
  emit('select', key)
  emit('close')
}
</script>
