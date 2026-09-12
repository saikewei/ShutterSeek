import { ref, computed, type Ref } from 'vue'
import { decodeImage } from './imageDecode'
import {
  uploadBatch,
  type BatchUploadItem,
  type BatchUploadResponse,
} from '@/api/photos'

/** 队列项的可见状态。解码/推理阶段之间会回到 waiting（张量或向量已就绪），
 *  界面据此显示「已解码 / 已向量化」之类的副标签。 */
export type ItemState =
  | 'waiting'
  | 'decoding'
  | 'embedding'
  | 'uploading'
  | 'done'
  | 'duplicate'
  | 'error'
  | 'canceled'

export interface QueueItem {
  id: string
  file: File
  state: ItemState
  error?: string
  /** 队列列表里的小方图（objectURL，移除时 revoke） */
  previewUrl?: string
  /** fp16 [3,224,224]，解码阶段产出；有它才进推理 */
  tensor?: any
  /** 1024 维向量，推理阶段产出；有它才进上传 */
  vector?: Float32Array
  /** 上传给服务端的缩略图源（长边 ≤1080 的 JPEG），可缺省 */
  previewBlob?: Blob
  /** 单文件批次时的字节进度 0..1 */
  progress: number
  photoId?: number
}

export interface QueueStats {
  total: number
  queued: number
  done: number
  duplicate: number
  failed: number
  canceled: number
  active: boolean
  /** 已处理的字节（含在途进度）/ 总字节 */
  processedBytes: number
  totalBytes: number
  bytesPerSec: number
  etaSec: number
}

export interface QueueOptions {
  /** 并发批请求数（默认 2） */
  uploadConcurrency?: number
  /** 单批最多文件数（默认 4） */
  batchMax?: number
  /** 超过这个大小就单独成批（默认 8MB），以便拿到字节级进度 */
  largeFileBytes?: number
  /** 解码并发（默认 2；移动端 1，32MP 位图一张就 130MB） */
  decodeConcurrency?: number
  /** 注入实现，单测用 */
  deps?: Partial<QueueDeps>
}

export interface QueueDeps {
  decode: (file: File) => Promise<ImageBitmap>
  preprocess: (bitmap: ImageBitmap) => Promise<any>
  embed: (tensor: any) => Promise<Float32Array>
  upload: (
    items: BatchUploadItem[],
    opts: { signal: AbortSignal; onUploadProgress?: (loaded: number, total: number) => void }
  ) => Promise<BatchUploadResponse>
  /** 从位图派生：队列小方图 + 上传用缩略图源。必须在 bitmap.close() 之前调用。 */
  derivePreview: (bitmap: ImageBitmap, file: File) => Promise<{ thumbUrl?: string; previewBlob?: Blob }>
}

const MAX_FILE_BYTES = 200 * 1024 * 1024 // 与服务端单文件上限一致
const DEFAULT_PREVIEW_FILE_BYTES = 3 * 1024 * 1024 // 小图自己解码更快，别加传输量

function isMobileUX(): boolean {
  if (typeof navigator === 'undefined') return false // 单测在 node 环境跑
  if (/Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent)) return true
  const mem = (navigator as any).deviceMemory
  return typeof mem === 'number' && mem <= 4
}

function toBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob((b) => resolve(b), type, quality))
}

/** 中心裁剪成 size×size 的 JPEG（与网格里的 object-cover 视觉一致）。 */
async function defaultThumb(bitmap: ImageBitmap, size = 224): Promise<string | undefined> {
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  if (!ctx) return undefined
  const short = Math.min(bitmap.width, bitmap.height)
  const sx = Math.max(0, (bitmap.width - short) / 2)
  const sy = Math.max(0, (bitmap.height - short) / 2)
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'medium'
  ctx.drawImage(bitmap, sx, sy, short, short, 0, 0, size, size)
  const blob = await toBlob(canvas, 'image/jpeg', 0.6)
  return blob ? URL.createObjectURL(blob) : undefined
}

/** 长边缩到 target（不放大）的 JPEG：服务端拿它直接 cwebp，省掉整张原图解码。 */
async function defaultDownscale(bitmap: ImageBitmap, target = 1080, quality = 0.9): Promise<Blob | undefined> {
  const long = Math.max(bitmap.width, bitmap.height)
  const scale = long > target ? target / long : 1
  const w = Math.max(1, Math.round(bitmap.width * scale))
  const h = Math.max(1, Math.round(bitmap.height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) return undefined
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(bitmap, 0, 0, w, h)
  const blob = await toBlob(canvas, 'image/jpeg', quality)
  // 浏览器不支持 JPEG 编码时会静默回落成 PNG：任其传上去只会更慢，直接放弃
  if (!blob || blob.type !== 'image/jpeg') return undefined
  return blob
}

export function createUploadQueue(opts: QueueOptions = {}) {
  // 并发是 ref：界面可以边跑边改，pump 每次都读最新值
  const uploadConcurrency = ref(Math.max(1, opts.uploadConcurrency ?? 2))
  const decodeConcurrency = ref(Math.max(1, opts.decodeConcurrency ?? (isMobileUX() ? 1 : 2)))
  const batchMax = Math.max(1, opts.batchMax ?? 4)
  const largeFileBytes = opts.largeFileBytes ?? 8 * 1024 * 1024

  // onnxruntime-web 有 1MB+ 的 wasm 运行时，必须懒加载：默认依赖里用动态 import
  const vision = () => import('./visionEmbed')
  const injected = opts.deps ?? {}
  const deps: QueueDeps = {
    decode: injected.decode ?? decodeImage,
    preprocess: injected.preprocess ?? (async (b) => (await vision()).preprocessToTensor(b)),
    embed: injected.embed ?? (async (t) => (await vision()).embedTensor(t)),
    upload:
      injected.upload ??
      ((items, o) => uploadBatch(items, { signal: o.signal, onUploadProgress: o.onUploadProgress })),
    derivePreview:
      injected.derivePreview ??
      (async (bitmap, file) => ({
        thumbUrl: await defaultThumb(bitmap, 224),
        // 小图服务端自己解码也很快，不值得为它多传 100KB+ 的 JPEG
        previewBlob:
          file.size > DEFAULT_PREVIEW_FILE_BYTES ? await defaultDownscale(bitmap, 1080, 0.9) : undefined,
      })),
  }

  const items = ref<QueueItem[]>([])
  const running = ref(false)
  const paused = ref(false)
  const startedAt = ref(0)
  const tick = ref(0) // 触发统计重算

  let generation = 0 // 取消/清空时自增，在途任务据此丢弃结果
  let decoding = 0
  let embedding = 0
  let uploading = 0
  let seq = 0
  const inflight = new Set<AbortController>()

  const totalBytes = computed(() => items.value.reduce((s, i) => s + i.file.size, 0))

  const processedBytes = computed(() => {
    void tick.value
    let sum = 0
    for (const i of items.value) {
      if (i.state === 'done' || i.state === 'duplicate' || i.state === 'error' || i.state === 'canceled') {
        sum += i.file.size
      } else if (i.state === 'uploading' && i.progress > 0) {
        sum += i.file.size * i.progress
      }
    }
    return sum
  })

  const stats = computed<QueueStats>(() => {
    const list = items.value
    const elapsed = startedAt.value ? Math.max(0.001, (Date.now() - startedAt.value) / 1000) : 0
    const processed = processedBytes.value
    const total = totalBytes.value
    const rate = elapsed > 0 && processed > 0 ? processed / elapsed : 0
    return {
      total: list.length,
      queued: list.filter((i) => i.state === 'waiting').length,
      done: list.filter((i) => i.state === 'done').length,
      duplicate: list.filter((i) => i.state === 'duplicate').length,
      failed: list.filter((i) => i.state === 'error').length,
      canceled: list.filter((i) => i.state === 'canceled').length,
      active: running.value,
      processedBytes: processed,
      totalBytes: total,
      bytesPerSec: rate,
      etaSec: rate > 0 ? Math.max(0, (total - processed) / rate) : 0,
    }
  })

  function add(files: File[]) {
    for (const file of files) {
      if (!file || file.size === 0 || file.size > MAX_FILE_BYTES) continue
      items.value.push({
        id: `u${++seq}`,
        file,
        state: 'waiting',
        progress: 0,
      })
    }
  }

  function releaseItem(item: QueueItem) {
    if (item.previewUrl) {
      URL.revokeObjectURL(item.previewUrl)
      item.previewUrl = undefined
    }
  }

  function remove(id: string) {
    const idx = items.value.findIndex((i) => i.id === id)
    if (idx === -1) return
    const item = items.value[idx]
    if (item.state === 'done' || item.state === 'duplicate') return // 已入库的不给删
    item.state = 'canceled'
    releaseItem(item)
    items.value.splice(idx, 1)
  }

  function clearFinished() {
    items.value = items.value.filter((i) => {
      const finished = i.state === 'done' || i.state === 'duplicate' || i.state === 'error' || i.state === 'canceled'
      if (finished) releaseItem(i)
      return !finished
    })
  }

  function clearAll() {
    generation++
    cancelInflight()
    for (const i of items.value) releaseItem(i)
    items.value = []
    running.value = false
    paused.value = false
  }

  function cancelInflight() {
    for (const c of inflight) c.abort()
    inflight.clear()
  }

  function start() {
    if (running.value) {
      paused.value = false
      return
    }
    running.value = true
    paused.value = false
    startedAt.value = Date.now()
    pump()
  }

  function pause() {
    paused.value = true
  }

  function cancelAll() {
    generation++
    cancelInflight()
    for (const i of items.value) {
      if (i.state !== 'done' && i.state !== 'duplicate') {
        i.state = 'canceled'
        i.progress = 0
      }
    }
    running.value = false
    paused.value = false
  }

  function retryFailed() {
    for (const i of items.value) {
      if (i.state === 'canceled') {
        i.state = 'waiting'
        i.progress = 0
        continue
      }
      if (i.state === 'error') {
        i.state = 'waiting'
        i.error = undefined
        i.progress = 0
        // 解码失败 → 连张量都要重来；推理/上传失败 → 保留已有阶段产物
      }
    }
    start()
  }

  /** 被取消/清空的在途任务：结果一律丢弃。 */
  function stale(gen: number): boolean {
    return gen !== generation
  }

  function pump() {
    tick.value++
    if (!running.value || paused.value) return
    pumpDecode()
    pumpEmbed()
    pumpUpload()
    if (decoding === 0 && embedding === 0 && uploading === 0) {
      const pending = items.value.some((i) => i.state === 'waiting')
      if (!pending) running.value = false
    }
  }

  /** 已解码但还没上传出去的项数：解码不能无限跑在前面。 */
  function downstreamBuffered(): number {
    let n = 0
    for (const i of items.value) {
      if (i.state === 'uploading' || (i.state === 'waiting' && i.tensor)) n++
    }
    return n
  }

  function pumpDecode() {
    const maxBuffered = Math.max(8, batchMax * Math.max(1, uploadConcurrency.value) * 2)
    while (running.value && !paused.value && decoding < decodeConcurrency.value) {
      if (downstreamBuffered() >= maxBuffered) break
      // 已有向量的项（例如上传失败后重试）绝不能再解码一遍
      const item = items.value.find((i) => i.state === 'waiting' && !i.tensor && !i.vector)
      if (!item) break
      const gen = generation
      item.state = 'decoding'
      decoding++
      runDecode(item, gen).finally(() => {
        decoding--
        pump()
      })
    }
  }

  async function runDecode(item: QueueItem, gen: number) {
    try {
      const bitmap = await deps.decode(item.file)
      try {
        if (stale(gen)) {
          item.state = 'canceled'
          return
        }
        // 预览只是锦上添花（服务端有完整兜底），画布失败不该让整张照片失败
        try {
          const preview = await deps.derivePreview(bitmap, item.file)
          if (preview.thumbUrl) item.previewUrl = preview.thumbUrl
          if (preview.previewBlob) item.previewBlob = preview.previewBlob
        } catch { /* 忽略：无预览仍可上传 */ }
        // 预处理必须在位图存活期间完成；之后就只剩 300KB 的张量
        item.tensor = await deps.preprocess(bitmap)
      } finally {
        bitmap.close?.()
      }
      if (stale(gen)) {
        item.state = 'canceled'
        return
      }
      item.state = 'waiting'
    } catch (err: any) {
      item.state = 'error'
      item.error = err?.message || String(err)
    }
  }

  function pumpEmbed() {
    if (!running.value || paused.value || embedding > 0) return
    const item = items.value.find((i) => i.state === 'waiting' && i.tensor && !i.vector)
    if (!item) return
    const gen = generation
    item.state = 'embedding'
    embedding++
    runEmbed(item, gen).finally(() => {
      embedding--
      pump()
    })
  }

  async function runEmbed(item: QueueItem, gen: number) {
    try {
      const vec = await deps.embed(item.tensor)
      if (stale(gen)) {
        item.state = 'canceled'
        return
      }
      item.vector = vec
      item.tensor = undefined // 向量到手，张量可以扔了
      item.state = 'waiting'
    } catch (err: any) {
      item.state = 'error'
      item.error = err?.message || String(err)
    }
  }

  function pumpUpload() {
    while (running.value && !paused.value && uploading < uploadConcurrency.value) {
      const batch = takeBatch()
      if (batch.length === 0) break
      const gen = generation
      for (const item of batch) item.state = 'uploading'
      uploading++
      runUpload(batch, gen).finally(() => {
        uploading--
        pump()
      })
    }
  }

  /** 就绪队列头部取一批：队首是大文件就单独成批（换字节级进度）。 */
  function takeBatch(): QueueItem[] {
    const ready = items.value.filter((i) => i.state === 'waiting' && i.vector)
    if (ready.length === 0) return []
    if (ready[0].file.size > largeFileBytes) return [ready[0]]
    return ready.slice(0, batchMax)
  }

  async function runUpload(batch: QueueItem[], gen: number) {
    const controller = new AbortController()
    inflight.add(controller)
    const single = batch.length === 1
    try {
      const payload: BatchUploadItem[] = batch.map((i) => ({
        file: i.file,
        vector: i.vector!,
        preview: i.file.size > DEFAULT_PREVIEW_FILE_BYTES ? i.previewBlob ?? null : null,
      }))
      const res = await deps.upload(payload, {
        signal: controller.signal,
        onUploadProgress: single
          ? (loaded, total) => {
              batch[0].progress = total > 0 ? Math.min(1, loaded / total) : 0
            }
          : undefined,
      })
      if (stale(gen)) {
        for (const i of batch) i.state = 'canceled'
        return
      }
      for (const r of res.results) {
        const item = batch[r.index]
        if (!item) continue
        item.progress = 1
        if (r.status === 'created') {
          item.state = 'done'
          item.photoId = r.id
        } else if (r.status === 'duplicate') {
          item.state = 'duplicate'
        } else {
          item.state = 'error'
          item.error = r.error || '上传失败'
        }
      }
      // 服务端漏回某项时不要让它永远挂着
      for (const i of batch) {
        if (i.state === 'uploading') {
          i.state = 'error'
          i.error = '服务端未返回该项结果'
        }
      }
    } catch (err: any) {
      const aborted = err?.name === 'CanceledError' || err?.code === 'ERR_CANCELED' || controller.signal.aborted
      for (const i of batch) {
        if (aborted) {
          i.state = 'canceled'
        } else if (i.state === 'uploading') {
          i.state = 'error'
          i.error = err?.message || String(err)
        }
      }
      if (aborted) throw err
    } finally {
      inflight.delete(controller)
    }
  }

  return {
    items,
    stats,
    running,
    paused,
    decodeConcurrency,
    uploadConcurrency,
    add,
    start,
    pause,
    resume: start,
    cancelAll,
    retryFailed,
    clearFinished,
    clearAll,
    remove,
  }
}

export type UploadQueue = ReturnType<typeof createUploadQueue>
export type { Ref }
