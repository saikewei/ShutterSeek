import * as ort from 'onnxruntime-web'
import { float16Array } from './fp16'

const MODEL_URL = '/models/vision_encoder/model.onnx'
const MEAN = [0.48145466, 0.4578275, 0.40821073]
const STD = [0.26862954, 0.26130258, 0.27577711]
const SIZE = 224

let sessionPromise: Promise<ort.InferenceSession> | null = null

// ── 诊断 ──────────────────────────────────────────────────
// 起因：executionProviders 是「优先级列表」，WebGPU EP 初始化失败时会**静默回退**
// 到 WASM，只看 navigator.gpu 判断不出实际用了哪个后端。这里改成逐个 EP 显式
// 尝试（先只给 webgpu，失败再给 wasm），把真实结果和失败原因都记下来；
// 同时采集并行度相关的环境事实（crossOriginIsolated / WASM 线程数）。

export type EpName = 'webgpu' | 'wasm'

export interface EpAttempt {
  ep: EpName
  ok: boolean
  ms: number
  error?: string
}

export interface AdapterInfo {
  vendor?: string
  architecture?: string
  device?: string
  description?: string
  f16: boolean
  maxBufferSize?: number
  maxStorageBufferBindingSize?: number
}

export interface EpReport {
  status: 'idle' | 'creating' | 'ready' | 'failed'
  /** 真正建成功的执行后端（null = 还没建过会话） */
  active: EpName | null
  attempts: EpAttempt[]
  gpuApi: boolean
  adapter: AdapterInfo | null
  wasm: {
    crossOriginIsolated: boolean
    sharedArrayBuffer: boolean
    hardwareConcurrency: number
    /** ORT 实际生效的线程数 */
    numThreads: number
    /** 按 ORT 的规则推算的期望值（隔离后 min(4, ceil(核数/2))，否则 1） */
    expectedThreads: number
  }
  /** 每张照片的推理耗时（最近 EMBED_WINDOW 次） */
  embed: { samples: number; lastMs: number; avgMs: number }
  /** 主动探测（probe）的结果 */
  probe: { status: 'idle' | 'running' | 'ok' | 'error'; ms?: number; error?: string }
  error?: string
}

const EMBED_WINDOW = 10
const embedTimes: number[] = []
let attempts: EpAttempt[] = []
let activeEp: EpName | null = null
let status: EpReport['status'] = 'idle'
let fatalError: string | undefined
let adapterInfo: AdapterInfo | null = null
let gpuApi = typeof navigator !== 'undefined' && !!(navigator as any).gpu
let probeState: EpReport['probe'] = { status: 'idle' }

const listeners = new Set<() => void>()
function notify() {
  for (const l of listeners) l()
}

/** 订阅诊断变化（建会话成功/回退、每次推理完成时触发）。 */
export function onEpReportChange(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

function now(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

function errText(e: any): string {
  const m = e?.message || e?.toString?.() || String(e)
  return m.length > 200 ? m.slice(0, 200) + '…' : m
}

function describeAdapter(a: any): AdapterInfo {
  const info = a?.info || {} // 新 Chrome/Safari 同步暴露；老实现只有 requestAdapterInfo()
  const lim = a?.limits || {}
  return {
    vendor: info.vendor,
    architecture: info.architecture,
    device: info.device,
    description: info.description,
    f16: !!a?.features?.has?.('shader-f16'),
    maxBufferSize: lim.maxBufferSize,
    maxStorageBufferBindingSize: lim.maxStorageBufferBindingSize,
  }
}

let adapterPromise: Promise<any | null> | null = null

/** 探测 GPU 适配器（只探一次，结果缓存）。 */
function getAdapter(): Promise<any | null> {
  if (!adapterPromise) {
    adapterPromise = (async () => {
      try {
        const gpu = (navigator as any).gpu
        if (!gpu) {
          gpuApi = false
          return null
        }
        const adapter = await gpu.requestAdapter()
        if (adapter) adapterInfo = describeAdapter(adapter)
        return adapter
      } catch {
        return null
      }
    })()
  }
  return adapterPromise
}

/** WASM 侧的并行度事实：没开 cross-origin isolation 时 ORT 强制单线程。 */
function wasmFacts() {
  const isolated = typeof self !== 'undefined' && (self as any).crossOriginIsolated === true
  const cores = (typeof navigator !== 'undefined' && navigator.hardwareConcurrency) || 1
  const expected = isolated ? Math.min(4, Math.ceil(cores / 2)) : 1
  const raw = ort.env?.wasm?.numThreads
  return {
    crossOriginIsolated: isolated,
    sharedArrayBuffer: typeof SharedArrayBuffer !== 'undefined',
    hardwareConcurrency: cores,
    numThreads: typeof raw === 'number' && raw > 0 ? raw : expected,
    expectedThreads: expected,
  }
}

export function epReport(): EpReport {
  const n = embedTimes.length
  const last = n ? embedTimes[n - 1] : 0
  const avg = n ? embedTimes.reduce((s, v) => s + v, 0) / n : 0
  return {
    status,
    active: activeEp,
    attempts: attempts.map((a) => ({ ...a })),
    gpuApi,
    adapter: adapterInfo ? { ...adapterInfo } : null,
    wasm: wasmFacts(),
    embed: { samples: n, lastMs: Math.round(last), avgMs: Math.round(avg) },
    probe: { ...probeState },
    error: fatalError,
  }
}

function recordEmbedMs(ms: number) {
  embedTimes.push(ms)
  if (embedTimes.length > EMBED_WINDOW) embedTimes.shift()
  notify()
}

/**
 * 建会话：先只声明 webgpu，失败再声明 wasm。
 * 显式分开是为了拿到「到底用了哪个后端」和「为什么回退」，而不是让 ORT 悄悄换掉。
 */
async function createSession(): Promise<ort.InferenceSession> {
  ort.env.wasm.wasmPaths = '/ort/'
  status = 'creating'
  attempts = []
  notify()

  const adapter = await getAdapter()
  if (adapter) {
    const t0 = now()
    try {
      const s = await ort.InferenceSession.create(MODEL_URL, { executionProviders: ['webgpu'] })
      attempts.push({ ep: 'webgpu', ok: true, ms: Math.round(now() - t0) })
      activeEp = 'webgpu'
      status = 'ready'
      notify()
      return s
    } catch (e: any) {
      attempts.push({ ep: 'webgpu', ok: false, ms: Math.round(now() - t0), error: errText(e) })
      notify()
    }
  } else {
    attempts.push({
      ep: 'webgpu',
      ok: false,
      ms: 0,
      error: gpuApi ? 'requestAdapter() 没返回设备' : 'navigator.gpu 不可用',
    })
    notify()
  }

  const t1 = now()
  try {
    const s = await ort.InferenceSession.create(MODEL_URL, { executionProviders: ['wasm'] })
    attempts.push({ ep: 'wasm', ok: true, ms: Math.round(now() - t1) })
    activeEp = 'wasm'
    status = 'ready'
    notify()
    return s
  } catch (e: any) {
    attempts.push({ ep: 'wasm', ok: false, ms: Math.round(now() - t1), error: errText(e) })
    status = 'failed'
    fatalError = errText(e)
    notify()
    throw e
  }
}

export function getSession(): Promise<ort.InferenceSession> {
  if (!sessionPromise) sessionPromise = createSession()
  return sessionPromise
}

/** 主动跑一次（真实模型 + 全零张量），一键确认实际后端与耗时。 */
export async function probe(): Promise<EpReport> {
  probeState = { status: 'running' }
  notify()
  try {
    const session = await getSession()
    const input = float16Array(3 * SIZE * SIZE)
    const t0 = now()
    await session.run({ pixel_values: new ort.Tensor('float16', input, [1, 3, SIZE, SIZE]) })
    probeState = { status: 'ok', ms: Math.round(now() - t0) }
  } catch (e: any) {
    probeState = { status: 'error', error: errText(e) }
  }
  notify()
  return epReport()
}

export async function detectCapability(): Promise<'webgpu-fp16' | 'webgpu' | 'wasm'> {
  const adapter = await getAdapter()
  if (!adapter) return 'wasm'
  return adapterInfo?.f16 ? 'webgpu-fp16' : 'webgpu'
}

/** 预处理：短边 224（高质缩放）→ CenterCrop → CLIP 归一化 → fp16 [3,224,224]。
 *
 * 注意：这一段必须与建库时的 Python 管线逐位一致，任何改动都会让新照片的
 * 向量与库里 6.7 万条既有向量不在同一分布上——不要动数值。
 */
export function preprocessToTensor(bitmap: ImageBitmap): any {
  const w = bitmap.width
  const h = bitmap.height
  let nw: number
  let nh: number
  if (w < h) {
    nw = SIZE
    nh = Math.floor((h * SIZE) / w)
  } else {
    nh = SIZE
    nw = Math.floor((w * SIZE) / h)
  }
  const canvas = document.createElement('canvas')
  canvas.width = nw
  canvas.height = nh
  const ctx = canvas.getContext('2d')!
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(bitmap, 0, 0, nw, nh)
  const left = Math.round((nw - SIZE) / 2)
  const top = Math.round((nh - SIZE) / 2)
  const crop = ctx.getImageData(left, top, SIZE, SIZE).data
  const out = float16Array(3 * SIZE * SIZE)
  for (let y = 0; y < SIZE; y++) {
    for (let x = 0; x < SIZE; x++) {
      const p = (y * SIZE + x) * 4
      const i = y * SIZE + x
      out[i] = (crop[p] / 255 - MEAN[0]) / STD[0]
      out[SIZE * SIZE + i] = (crop[p + 1] / 255 - MEAN[1]) / STD[1]
      out[2 * SIZE * SIZE + i] = (crop[p + 2] / 255 - MEAN[2]) / STD[2]
    }
  }
  return out
}

/** 兼容包装：从位图直接得到预处理张量。 */
export function preprocess(bitmap: ImageBitmap): any {
  return preprocessToTensor(bitmap)
}

/** 推理并返回 1024 维 float32 向量（模型已 L2 归一化）。
 *
 * 只接受预处理好的张量：这样解码阶段一结束就能 bitmap.close()，
 * 全尺寸位图不必跨越流水线存活（32MP 位图 ≈ 130MB）。
 */
export async function embedTensor(input: any): Promise<Float32Array> {
  const session = await getSession()
  const t0 = now()
  const feeds: Record<string, ort.Tensor> = {
    pixel_values: new ort.Tensor('float16', input, [1, 3, SIZE, SIZE]),
  }
  const results = await session.run(feeds)
  recordEmbedMs(now() - t0)
  const out = results.embedding as ort.Tensor
  return new Float32Array(out.data as any)
}

/** 兼容包装：解码 + 推理一步到位。 */
export async function embed(bitmap: ImageBitmap): Promise<Float32Array> {
  return embedTensor(preprocessToTensor(bitmap))
}
