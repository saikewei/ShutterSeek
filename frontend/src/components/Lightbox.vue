<template>
  <Dialog :open="open" @close="$emit('close')" class="relative z-50">
    <div class="fixed inset-0 bg-black/95" aria-hidden="true" />

    <div class="fixed inset-0 flex items-center justify-center overflow-hidden"
         @click.self="$emit('close')">

      <!-- Close -->
      <button @click="$emit('close')"
        class="absolute top-4 z-10 text-white/70 hover:text-white text-2xl w-10 h-10"
        :style="{ right: photo ? '308px' : '16px' }">✕</button>

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
        :style="{ paddingRight: photo ? '288px' : '0' }"
        class="w-full h-full flex items-center justify-center overflow-hidden transition-[padding] duration-200"
        @wheel.prevent="onWheel"
        @mousedown="onMouseDown"
        @mousemove="onMouseMove"
        @mouseup="onMouseUp"
        @mouseleave="onMouseUp"
      >
        <img
          v-if="photo"
          :key="photo.id"
          :src="`/api/v1/photos/${photo.id}/original`"
          :alt="photo.file_name || 'Original'"
          :style="{
            transform: `translate(${x}px, ${y}px) scale(${scale})`,
            transformOrigin: 'center center',
            transition: dragging ? 'none' : 'transform 0.08s ease-out',
          }"
          :class="[
            'select-none max-h-[85vh] max-w-[95vw]',
            photo.height > photo.width ? 'rotate-270' : '',
            dragging ? 'cursor-grabbing' : scale > 1 ? 'cursor-grab' : 'cursor-zoom-in',
          ]"
          @load="loading = false"
          draggable="false"
        />
        <div v-if="loading" class="text-white/50 text-sm absolute">Loading...</div>
      </div>

      <!-- EXIF sidebar -->
      <Transition name="exif">
        <div v-if="photo" class="absolute right-0 top-0 bottom-0 w-72 bg-neutral-900/70 backdrop-blur border-l border-white/10 overflow-y-auto pointer-events-auto">
          <div class="p-4 space-y-3 text-sm">
            <h3 class="text-white/80 font-medium text-base border-b border-white/10 pb-2">{{ photo.file_name }}</h3>

            <div v-if="photo.taken_at" class="flex justify-between">
              <span class="text-white/40">Date</span>
              <span class="text-white/80">{{ photo.taken_at }}</span>
            </div>

            <div v-if="photo.camera_make" class="flex justify-between">
              <span class="text-white/40">Camera</span>
              <span class="text-white/80">{{ photo.camera_make }} {{ photo.camera_model }}</span>
            </div>

            <div v-if="photo.lens_model" class="flex justify-between">
              <span class="text-white/40">Lens</span>
              <span class="text-white/80">{{ photo.lens_model }}</span>
            </div>

            <div class="flex justify-between">
              <span class="text-white/40">Resolution</span>
              <span class="text-white/80">{{ photo.width }} × {{ photo.height }}</span>
            </div>

            <div v-if="photo.focal_length" class="flex justify-between">
              <span class="text-white/40">Focal</span>
              <span class="text-white/80">{{ photo.focal_length }}</span>
            </div>

            <div v-if="photo.aperture" class="flex justify-between">
              <span class="text-white/40">Aperture</span>
              <span class="text-white/80">{{ photo.aperture }}</span>
            </div>

            <div v-if="photo.iso" class="flex justify-between">
              <span class="text-white/40">ISO</span>
              <span class="text-white/80">{{ photo.iso }}</span>
            </div>

            <div class="border-t border-white/10 pt-2 mt-2">
              <p class="text-white/30 text-xs truncate">{{ photo.file_path }}</p>
            </div>
          </div>
        </div>
      </Transition>
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

const isPortrait = () => props.photo && props.photo.height > props.photo.width

// ── Wheel zoom: scale toward cursor ──────────────────
function onWheel(e: WheelEvent) {
  const rect = container.value!.getBoundingClientRect()
  const cx = e.clientX - rect.left - rect.width / 2
  const cy = e.clientY - rect.top - rect.height / 2

  const factor = e.deltaY < 0 ? 1.16 : 1 / 1.16
  const newScale = Math.max(0.3, Math.min(10, scale.value * factor))

  if (isPortrait()) {
    // 270° rotation: screen space → pre-rotation space
    // pre-rot (px, py) maps to screen: sx=py*s, sy=-px*s → px=-sy/s, py=sx/s
    const px = -cy / scale.value
    const py = cx / scale.value
    x.value = x.value + px * (scale.value - newScale)
    y.value = y.value + py * (scale.value - newScale)
  } else {
    x.value = cx - (cx - x.value) * (newScale / scale.value)
    y.value = cy - (cy - y.value) * (newScale / scale.value)
  }
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
  const dx = e.clientX - lastX
  const dy = e.clientY - lastY
  if (isPortrait()) {
    // 270° rotation: screen-right → visual-right = translate-y+, screen-down → visual-down = translate-x-
    x.value -= dy
    y.value += dx
  } else {
    x.value += dx
    y.value += dy
  }
  lastX = e.clientX; lastY = e.clientY
}

function onMouseUp() { dragging.value = false }
</script>
