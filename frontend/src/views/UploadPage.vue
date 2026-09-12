<template>
  <div class="min-h-full pb-16">
    <!-- 顶栏：标题 + 能力提示 + 目标相册 -->
    <header class="sticky top-0 z-30 bg-raised/95 backdrop-blur border-b border-line px-4 py-3">
      <div class="max-w-5xl mx-auto flex items-center gap-3 flex-wrap">
        <h1 class="font-display text-base font-semibold text-ink">上传照片</h1>
        <span v-if="capability === 'wasm'" class="chip bg-accent-soft text-accent-strong">WASM 推理（较慢）</span>
        <span v-else-if="capability === 'webgpu'" class="chip-ghost">WebGPU</span>
        <span v-else-if="capability === 'webgpu-fp16'" class="chip-ghost">WebGPU fp16</span>
        <div class="flex-1"></div>
        <label class="text-xs text-ink-3">目标相册</label>
        <select v-model.number="albumId" class="input w-40 py-1 text-xs">
          <option :value="0">不加入相册</option>
          <option v-for="a in albums" :key="a.id" :value="a.id">{{ a.title }}</option>
        </select>
      </div>
    </header>

    <div class="max-w-5xl mx-auto p-4 space-y-4">
      <!-- 投放区 -->
      <div
        class="rounded-xl border border-dashed bg-raised/60 transition-colors duration-150 cursor-pointer text-center"
        :class="dragging ? 'border-accent bg-accent-soft' : 'border-line-strong hover:bg-surface'"
        @click="pickFiles"
        @dragover.prevent="dragging = true"
        @dragleave.prevent="dragging = false"
        @drop.prevent="onDrop"
      >
        <input ref="fileInput" type="file" multiple class="hidden" @change="onPick" />
        <input ref="dirInput" type="file" multiple class="hidden" @change="onPick" />
        <div :class="queue.items.value.length ? 'py-6' : 'py-14'">
          <svg class="w-8 h-8 mx-auto text-ink-3" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 16.5V4.5m0 0L7.5 9M12 4.5 16.5 9M4.5 19.5h15" />
          </svg>
          <p class="mt-3 text-sm text-ink">把照片拖到这里，或点击选择</p>
          <p class="mt-1 text-xs text-ink-3">
            JPG · PNG · HEIC · TIFF · RAW（NEF/CR2/CR3/ARW/DNG…），单张最大 200MB，可整文件夹选择
          </p>
          <div class="mt-4 flex items-center justify-center gap-2">
            <button class="btn-ghost px-3 py-1.5 text-xs" @click.stop="pickFiles">选择照片</button>
            <button class="btn-ghost px-3 py-1.5 text-xs" @click.stop="pickDir">选择文件夹</button>
          </div>
          <p class="mt-2 text-[11px] text-ink-3">也可以直接 Ctrl/⌘+V 粘贴截图</p>
        </div>
      </div>

      <!-- 总进度 -->
      <div v-if="queue.items.value.length" class="panel p-4">
        <div class="flex items-center justify-between text-xs text-ink-2 gap-3 flex-wrap">
          <span>
            已处理 <span class="text-ink font-semibold">{{ stats.done + stats.duplicate }}</span>/{{ stats.total }}
            <span v-if="stats.done"> · 新建 {{ stats.done }}</span>
            <span v-if="stats.duplicate"> · 重复 {{ stats.duplicate }}</span>
            <span v-if="stats.failed" class="text-danger-ink"> · 失败 {{ stats.failed }}</span>
            <span v-if="stats.canceled"> · 已取消 {{ stats.canceled }}</span>
          </span>
          <span class="text-ink-3">
            {{ fmtBytes(stats.processedBytes) }} / {{ fmtBytes(stats.totalBytes) }}
            <template v-if="stats.bytesPerSec"> · {{ fmtBytes(stats.bytesPerSec) }}/s</template>
            <template v-if="stats.etaSec"> · 剩余约 {{ fmtEta(stats.etaSec) }}</template>
          </span>
        </div>
        <div class="mt-3 h-1.5 rounded-full bg-surface overflow-hidden">
          <div class="h-full bg-accent transition-[width] duration-300" :style="{ width: percent + '%' }"></div>
        </div>

        <div class="mt-3 flex flex-wrap items-center gap-2">
          <button v-if="!queue.running.value" class="btn-primary px-4 py-1.5 text-xs" :disabled="!canStart" @click="queue.start()">
            开始上传
          </button>
          <button v-else-if="queue.paused.value" class="btn-primary px-4 py-1.5 text-xs" @click="queue.resume()">继续</button>
          <button v-else class="btn-ghost px-4 py-1.5 text-xs" @click="queue.pause()">暂停</button>
          <button
            class="btn-ghost px-3 py-1.5 text-xs"
            :disabled="!queue.running.value"
            @click="queue.cancelAll()"
          >取消全部</button>
          <button
            class="btn-ghost px-3 py-1.5 text-xs"
            :disabled="!stats.failed && !stats.canceled"
            @click="queue.retryFailed()"
          >重试失败项</button>
          <button
            class="btn-ghost px-3 py-1.5 text-xs"
            :disabled="!stats.done && !stats.duplicate && !stats.failed"
            @click="queue.clearFinished()"
          >清空已结束</button>
          <div class="flex-1"></div>
          <label class="text-xs text-ink-3">并发</label>
          <select v-model.number="concurrency" class="input w-20 py-1 text-xs">
            <option :value="1">1</option>
            <option :value="2">2</option>
            <option :value="4">4</option>
            <option :value="6">6</option>
          </select>
        </div>

        <p v-if="albumHint" class="mt-2 text-xs text-success">{{ albumHint }}</p>
      </div>

      <!-- 队列 -->
      <ul v-if="queue.items.value.length" class="panel divide-y divide-line overflow-hidden">
        <li v-for="item in visibleItems" :key="item.id" class="flex items-center gap-3 px-3 py-2">
          <img
            v-if="item.previewUrl"
            :src="item.previewUrl"
            class="w-10 h-10 rounded object-cover bg-surface shrink-0"
            alt=""
          />
          <div v-else class="w-10 h-10 rounded bg-surface shrink-0"></div>
          <div class="flex-1 min-w-0">
            <div class="truncate text-xs text-ink">{{ item.file.name }}</div>
            <div class="flex items-center gap-2 text-[11px] text-ink-3">
              <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="dotClass(item.state)"></span>
              <span class="shrink-0">{{ stateLabel(item) }}</span>
              <span class="shrink-0">{{ fmtBytes(item.file.size) }}</span>
              <span v-if="item.error" class="text-danger-ink truncate">{{ item.error }}</span>
            </div>
            <div v-if="item.state === 'uploading' && item.progress > 0" class="mt-1 h-1 rounded bg-surface overflow-hidden">
              <div class="h-full bg-accent" :style="{ width: item.progress * 100 + '%' }"></div>
            </div>
          </div>
          <button
            v-if="item.state === 'error' || item.state === 'canceled'"
            class="text-xs text-ink-3 hover:text-ink transition-colors shrink-0"
            @click="queue.retryFailed()"
          >重试</button>
          <button
            v-if="item.state !== 'done'"
            class="text-xs text-ink-3 hover:text-ink transition-colors shrink-0"
            @click="queue.remove(item.id)"
          >移除</button>
        </li>
      </ul>
      <p v-if="hiddenCount" class="text-center text-xs text-ink-3">还有 {{ hiddenCount }} 项未显示（队列过长时只显示前 300 项）</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { createUploadQueue, type QueueItem } from '@/lib/uploadQueue'
import { fetchAlbums, batchAddPhotos, type Album } from '@/api/albums'

const route = useRoute()

const queue = createUploadQueue()
const stats = queue.stats
const concurrency = ref(queue.uploadConcurrency.value)
watch(concurrency, (v) => { queue.uploadConcurrency.value = v; localStorage.setItem('ss_upload_concurrency', String(v)) })

const capability = ref<'webgpu-fp16' | 'webgpu' | 'wasm' | ''>('')
const albums = ref<Album[]>([])
const albumId = ref(0)
const albumHint = ref('')
const dragging = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const dirInput = ref<HTMLInputElement | null>(null)

const MAX_VISIBLE = 300
const visibleItems = computed(() => queue.items.value.slice(0, MAX_VISIBLE))
const hiddenCount = computed(() => Math.max(0, queue.items.value.length - MAX_VISIBLE))
const canStart = computed(() => queue.items.value.some((i) => i.state === 'waiting'))
const percent = computed(() => {
  const s = stats.value
  return s.totalBytes > 0 ? Math.min(100, Math.round((s.processedBytes / s.totalBytes) * 100)) : 0
})

onMounted(async () => {
  // webkitdirectory 只能靠 setAttribute 打开，模板里直接写会被类型检查拦下
  dirInput.value?.setAttribute('webkitdirectory', '')
  const saved = Number(localStorage.getItem('ss_upload_concurrency') || '')
  if (saved >= 1 && saved <= 8) {
    concurrency.value = saved
  }
  const albumParam = Number(route.query.album || '')
  try {
    const res = await fetchAlbums()
    albums.value = res.items
    if (albumParam && res.items.some((a) => a.id === albumParam)) albumId.value = albumParam
  } catch { /* 相册列表拿不到不影响上传 */ }
  try {
    const vision = await import('@/lib/visionEmbed')
    capability.value = await vision.detectCapability()
  } catch { /* 能力探测失败按未知处理 */ }
  document.addEventListener('paste', onPaste)
})

onUnmounted(() => {
  document.removeEventListener('paste', onPaste)
  queue.cancelAll()
})

function pickFiles() { fileInput.value?.click() }
function pickDir() { dirInput.value?.click() }

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  queue.add(Array.from(input.files || []))
  input.value = ''
  if (!queue.running.value) queue.start()
}

function onDrop(e: DragEvent) {
  dragging.value = false
  const files = Array.from(e.dataTransfer?.files || [])
  if (!files.length) return
  queue.add(files)
  if (!queue.running.value) queue.start()
}

function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files || [])
  if (!files.length) return
  queue.add(files)
  if (!queue.running.value) queue.start()
}

// 一轮跑完把新建的照片补进目标相册（用既有的批量加相册接口，失败不影响上传结果）
const addedToAlbum = new Set<number>()
watch(
  () => queue.running.value,
  async (running) => {
    if (running || albumId.value <= 0) return
    const pending = queue.items.value
      .filter((i) => i.state === 'done' && i.photoId && !addedToAlbum.has(i.photoId))
      .map((i) => i.photoId as number)
    if (!pending.length) return
    try {
      for (let i = 0; i < pending.length; i += 200) {
        const chunk = pending.slice(i, i + 200)
        await batchAddPhotos(albumId.value, chunk)
        chunk.forEach((id) => addedToAlbum.add(id))
      }
      const album = albums.value.find((a) => a.id === albumId.value)
      albumHint.value = `已把 ${pending.length} 张加入「${album?.title || albumId.value}」`
      setTimeout(() => { albumHint.value = '' }, 4000)
    } catch {
      albumHint.value = '加入相册失败（照片已上传成功）'
    }
  }
)

function stateLabel(item: QueueItem): string {
  switch (item.state) {
    case 'decoding': return '解码中'
    case 'embedding': return '向量提取中'
    case 'uploading': return '上传中'
    case 'done': return '✓ 已完成'
    case 'duplicate': return '重复，未入库'
    case 'error': return '失败'
    case 'canceled': return '已取消'
    default:
      if (item.vector) return '排队上传'
      if (item.tensor) return '排队向量化'
      return '等待'
  }
}

function dotClass(state: QueueItem['state']): string {
  switch (state) {
    case 'done': return 'bg-success'
    case 'duplicate': return 'bg-ink-3'
    case 'error': return 'bg-danger'
    case 'canceled': return 'bg-ink-3'
    case 'embedding':
    case 'uploading': return 'bg-accent-strong'
    case 'decoding': return 'bg-ink-2'
    default: return 'bg-ink-3'
  }
}

function fmtBytes(n: number): string {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

function fmtEta(sec: number): string {
  if (!isFinite(sec) || sec <= 0) return ''
  if (sec < 60) return `${Math.round(sec)} 秒`
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m} 分 ${Math.round(sec % 60)} 秒`
  return `${Math.floor(m / 60)} 小时 ${m % 60} 分`
}
</script>
