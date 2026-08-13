<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" @click.self="$emit('close')">
    <div class="w-full max-w-lg rounded-lg bg-neutral-900 border border-neutral-800 p-4 text-white">
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-sm font-semibold">上传照片</h2>
        <button class="text-neutral-400 text-xs hover:text-white transition-colors" @click="$emit('close')">关闭</button>
      </div>
      <p v-if="capability === 'wasm'" class="text-amber-400 text-xs mb-2">
        本设备未启用 GPU 加速（WebGPU），将使用 WASM 推理，上传会较慢。
      </p>
      <input ref="fileInput" type="file" accept="image/*,.nef,.cr2,.cr3,.arw,.rw2,.dng,.orf,.raf,.pef,.sr2,.srf,.tif,.tiff,.heic,.heif" multiple class="mb-3 text-xs" @change="onPick" />
      <ul class="space-y-2 max-h-72 overflow-auto">
        <li v-for="item in items" :key="item.file.name + item.file.size" class="flex items-center gap-2 text-xs">
          <img v-if="item.preview" :src="item.preview" class="w-10 h-10 object-cover rounded shrink-0" alt="" />
          <div class="flex-1 min-w-0">
            <div class="truncate">{{ item.file.name }}</div>
            <div class="text-neutral-500">{{ stateLabel(item) }}</div>
            <div v-if="item.state === 'error' && item.error" class="text-red-400 truncate">{{ item.error }}</div>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { decodeImage } from '@/lib/imageDecode'
import { uploadPhoto } from '@/api/photos'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'done'): void }>()

interface QueueItem {
  file: File
  preview?: string
  state: 'waiting' | 'decoding' | 'embedding' | 'uploading' | 'done' | 'duplicate' | 'error'
  error?: string
}

const items = ref<QueueItem[]>([])
const capability = ref<'webgpu-fp16' | 'webgpu' | 'wasm'>('wasm')
const processing = ref(false)

watch(() => props.open, async (v) => {
  if (v) {
    const vision = await import('@/lib/visionEmbed')
    capability.value = await vision.detectCapability()
  } else {
    items.value = []
  }
})

function stateLabel(item: QueueItem): string {
  switch (item.state) {
    case 'decoding': return '解码中'
    case 'embedding': return '向量提取中'
    case 'uploading': return '上传中'
    case 'done': return '✓ 完成'
    case 'duplicate': return '重复，未入库'
    case 'error': return '失败'
    default: return '等待'
  }
}

async function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (!files.length || processing.value) return
  items.value = files.map(f => ({ file: f, state: 'waiting' as const }))
  processing.value = true
  try {
    for (const item of items.value) {
      await processItem(item)
    }
    emit('done')
  } finally {
    processing.value = false
    input.value = ''
  }
}

async function processItem(item: QueueItem) {
  try {
    item.state = 'decoding'
    const bitmap = await decodeImage(item.file)
    const canvas = document.createElement('canvas')
    canvas.width = 224
    canvas.height = 224
    canvas.getContext('2d')!.drawImage(bitmap, 0, 0, 224, 224)
    item.preview = canvas.toDataURL('image/jpeg', 0.5)

    item.state = 'embedding'
    const vision = await import('@/lib/visionEmbed')
    const vec = await vision.embed(bitmap)
    bitmap.close()

    item.state = 'uploading'
    const res = await uploadPhoto(item.file, Array.from(vec))
    item.state = res.duplicate ? 'duplicate' : 'done'
  } catch (err: any) {
    item.state = 'error'
    item.error = err?.message || String(err)
  }
}
</script>
