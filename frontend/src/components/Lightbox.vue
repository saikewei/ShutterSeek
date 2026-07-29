<template>
  <Dialog :open="open" @close="$emit('close')" class="relative z-50">
    <div class="fixed inset-0 bg-black/95" aria-hidden="true" />

    <div class="fixed inset-0 flex items-center justify-center overflow-hidden"
         @click.self="$emit('close')">

      <!-- Close -->
      <button @click="$emit('close')"
        class="absolute top-4 right-4 z-10 text-white/70 hover:text-white text-2xl w-10 h-10">✕</button>

      <!-- Reset zoom -->
      <button v-if="scale !== 1" @click="resetZoom"
        class="absolute top-4 left-4 z-10 text-white/50 hover:text-white text-sm px-2 py-1">Reset</button>

      <!-- Prev / Next -->
      <button v-if="hasPrev" @click.stop="$emit('prev')"
        class="absolute left-2 top-1/2 -translate-y-1/2 z-10 text-white/50 hover:text-white text-4xl w-12 h-12 flex items-center justify-center">‹</button>
      <button v-if="hasNext" @click.stop="$emit('next')"
        class="absolute right-2 top-1/2 -translate-y-1/2 z-10 text-white/50 hover:text-white text-4xl w-12 h-12 flex items-center justify-center">›</button>

      <!-- Photo container -->
      <div
        ref="container"
        class="w-full h-full flex items-center justify-center overflow-hidden"
        @wheel.prevent="onWheel"
        @mousedown="onMouseDown"
        @mousemove="onMouseMove"
        @mouseup="onMouseUp"
        @mouseleave="onMouseUp"
      >
        <img
          v-if="photo"
          :src="`/api/v1/photos/${photo.id}/original`"
          :alt="photo.file_name || 'Original'"
          :style="{
            transform: `translate(${x}px, ${y}px) scale(${scale})`,
            transformOrigin: 'center center',
            transition: dragging ? 'none' : 'transform 0.08s ease-out',
            maxHeight: scale === 1 ? '85vh' : 'none',
            maxWidth: scale === 1 ? '95vw' : 'none',
          }"
          :class="[
            'select-none',
            photo.height > photo.width ? 'rotate-270' : '',
            dragging ? 'cursor-grabbing' : scale > 1 ? 'cursor-grab' : 'cursor-zoom-in',
          ]"
          @load="loading = false"
          draggable="false"
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

defineEmits<{ close: []; prev: []; next: [] }>()

const loading = ref(true)
const container = ref<HTMLElement | null>(null)

// zoom + pan state
const scale = ref(1)
const x = ref(0)
const y = ref(0)
const dragging = ref(false)
let lastX = 0, lastY = 0

function resetZoom() {
  scale.value = 1; x.value = 0; y.value = 0
}

watch(() => props.photo, () => {
  loading.value = true
  resetZoom()
})

// ── Wheel zoom: scale toward cursor ──────────────────
function onWheel(e: WheelEvent) {
  const rect = container.value!.getBoundingClientRect()
  const cx = e.clientX - rect.left - rect.width / 2
  const cy = e.clientY - rect.top - rect.height / 2

  const factor = e.deltaY < 0 ? 1.06 : 1 / 1.06
  const newScale = Math.max(0.3, Math.min(10, scale.value * factor))

  // Adjust pan so the point under cursor stays fixed
  x.value = cx - (cx - x.value) * (newScale / scale.value)
  y.value = cy - (cy - y.value) * (newScale / scale.value)
  scale.value = newScale
}

// ── Pan drag ──────────────────────────────────────────
function onMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  dragging.value = true
  lastX = e.clientX; lastY = e.clientY
}

function onMouseMove(e: MouseEvent) {
  if (!dragging.value) return
  x.value += e.clientX - lastX
  y.value += e.clientY - lastY
  lastX = e.clientX; lastY = e.clientY
}

function onMouseUp() { dragging.value = false }
</script>
