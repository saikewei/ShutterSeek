<template>
  <AppModal :open="open" :title="title" @close="close">
    <form @submit.prevent="save">
      <label class="block text-xs text-ink-2">作品名称
        <input v-model="name" class="input mt-2" maxlength="100" :disabled="busy" placeholder="给这份照片故事起个名字" />
      </label>
      <p v-if="error" role="alert" class="text-sm text-danger-ink mt-3">{{ error }}</p>
    </form>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn-ghost px-4 py-2 text-sm" :disabled="busy" @click="close">取消</button>
        <button class="btn-primary px-4 py-2 text-sm" :disabled="busy || !name.trim()" @click="save">{{ busy ? '保存中…' : submitLabel }}</button>
      </div>
    </template>
  </AppModal>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import AppModal from '@/components/AppModal.vue'
const props = withDefaults(defineProps<{ open: boolean; initialName?: string; title?: string; submitLabel?: string; busy?: boolean; error?: string }>(), { title: '修改作品名称', submitLabel: '保存名称', busy: false })
const emit = defineEmits<{ close: []; save: [name: string] }>()
const name = ref('')
watch(() => props.open, open => { if (open) name.value = props.initialName || '' }, { immediate: true })
function close() { if (!props.busy) emit('close') }
function save() { if (!props.busy && name.value.trim()) emit('save', name.value.trim()) }
</script>
