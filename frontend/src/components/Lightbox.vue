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

      <!-- Zoom button -->
      <button @click="fitScreen = !fitScreen"
        class="absolute top-4 left-4 z-10 text-white/50 hover:text-white text-sm px-2 py-1">
        {{ fitScreen ? '1:1' : 'Fit' }}
      </button>

      <!-- Photo area -->
      <div
        v-if="photo"
        :class="[
          'max-h-full max-w-full',
          fitScreen ? 'overflow-auto' : 'overflow-hidden flex items-center justify-center'
        ]"
        @click.self="fitScreen = false"
      >
        <img
          :src="`/api/v1/photos/${photo.id}/original`"
          :alt="photo.file_name || 'Original'"
          :class="[
            photo.height > photo.width ? 'rotate-270' : '',
            fitScreen ? 'max-w-none max-h-none cursor-grab active:cursor-grabbing' : 'max-h-[85vh] max-w-full object-contain cursor-zoom-in'
          ]"
          @load="loading = false"
          @click.stop="fitScreen = !fitScreen"
        />
        <div v-if="loading" class="text-white/50 text-sm absolute">Loading...</div>
      </div>

      <!-- EXIF bar -->
      <div v-if="photo" class="absolute bottom-4 left-0 right-0 flex flex-wrap gap-x-4 gap-y-1 text-xs text-white/50 justify-center px-4">
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
const fitScreen = ref(false)

watch(() => props.photo, () => { loading.value = true; fitScreen.value = false })

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowLeft' && props.hasPrev) { e.preventDefault(); /* handled */ }
  if (e.key === 'ArrowRight' && props.hasNext) { e.preventDefault(); /* handled */ }
}
</script>
