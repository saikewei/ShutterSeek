<template>
  <Dialog :open="open" @close="$emit('close')" class="relative z-50">
    <div class="fixed inset-0 bg-black/95" aria-hidden="true" />

    <div class="fixed inset-0 flex items-center justify-center"
         @click.self="$emit('close')" @keydown="onKey">

      <!-- Close button -->
      <button @click="$emit('close')"
        class="absolute top-4 right-4 z-10 text-white/70 hover:text-white text-2xl w-10 h-10">
        ✕
      </button>

      <!-- Prev -->
      <button v-if="hasPrev" @click.stop="$emit('prev')"
        class="absolute left-2 top-1/2 -translate-y-1/2 z-10 text-white/50 hover:text-white text-4xl w-12 h-12 flex items-center justify-center">
        ‹
      </button>

      <!-- Next -->
      <button v-if="hasNext" @click.stop="$emit('next')"
        class="absolute right-2 top-1/2 -translate-y-1/2 z-10 text-white/50 hover:text-white text-4xl w-12 h-12 flex items-center justify-center">
        ›
      </button>

      <!-- Photo with zoom -->
      <div class="max-h-full max-w-full p-4 flex flex-col items-center"
           @wheel.prevent="onWheel">
        <img
          v-if="photo"
          :src="`/api/v1/photos/${photo.id}/original`"
          :alt="photo.file_name || 'Original'"
          :style="{ transform: `scale(${zoom})` }"
          :class="['max-h-[80vh] max-w-full object-contain cursor-zoom-in', photo.height > photo.width ? 'rotate-270' : '']"
          @load="loading = false"
          @click="zoom = zoom > 1 ? 1 : 2"
        />
        <div v-if="loading" class="text-white/50 text-sm">Loading...</div>

        <!-- EXIF bar -->
        <div v-if="photo" class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-white/60 justify-center">
          <span>{{ photo.file_name }}</span>
          <span v-if="photo.camera_make">{{ photo.camera_make }} {{ photo.camera_model }}</span>
          <span v-if="photo.lens_model">{{ photo.lens_model }}</span>
          <span v-if="photo.focal_length">{{ photo.focal_length }}</span>
          <span v-if="photo.aperture">{{ photo.aperture }}</span>
          <span v-if="photo.iso">ISO {{ photo.iso }}</span>
          <span v-if="photo.taken_at">{{ photo.taken_at }}</span>
          <span>{{ photo.width }}×{{ photo.height }}</span>
        </div>
      </div>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Dialog } from '@headlessui/vue'
import type { Photo } from '@/api/photos'

const props = defineProps<{
  open: boolean
  photo: Photo | null
  hasPrev: boolean
  hasNext: boolean
}>()

defineEmits<{
  close: []
  prev: []
  next: []
}>()

const loading = ref(true)
const zoom = ref(1)

watch(() => props.photo, () => { loading.value = true; zoom.value = 1 })

function onWheel(e: WheelEvent) {
  zoom.value = Math.max(0.5, Math.min(5, zoom.value - e.deltaY * 0.001))
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') { /* Dialog handles this */ }
  if (e.key === 'ArrowLeft' && props.hasPrev) { /* handled by button */ }
  if (e.key === 'ArrowRight' && props.hasNext) { /* handled by button */ }
}
</script>
