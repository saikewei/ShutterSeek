import * as ort from 'onnxruntime-web'
import { float16Array } from './fp16'

const MODEL_URL = '/models/vision_encoder/model.onnx'
const MEAN = [0.48145466, 0.4578275, 0.40821073]
const STD = [0.26862954, 0.26130258, 0.27577711]
const SIZE = 224

let sessionPromise: Promise<ort.InferenceSession> | null = null

export function getSession(): Promise<ort.InferenceSession> {
  if (!sessionPromise) {
    ort.env.wasm.wasmPaths = '/ort/'
    sessionPromise = ort.InferenceSession.create(MODEL_URL, {
      executionProviders: ['webgpu', 'wasm'],
    })
  }
  return sessionPromise
}

export async function detectCapability(): Promise<'webgpu-fp16' | 'webgpu' | 'wasm'> {
  try {
    const gpu = (navigator as any).gpu
    if (!gpu) return 'wasm'
    const adapter = await gpu.requestAdapter()
    if (!adapter) return 'wasm'
    if (adapter.features.has('shader-f16')) return 'webgpu-fp16'
    return 'webgpu'
  } catch {
    return 'wasm'
  }
}

/** 预处理：短边 224（高质缩放）→ CenterCrop → CLIP 归一化 → fp16 [1,3,224,224]。 */
export function preprocess(bitmap: ImageBitmap): any {
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

/** 推理并返回 1024 维 float32 向量（模型已 L2 归一化）。 */
export async function embed(bitmap: ImageBitmap): Promise<Float32Array> {
  const session = await getSession()
  const input = preprocess(bitmap)
  const feeds: Record<string, ort.Tensor> = {
    pixel_values: new ort.Tensor('float16', input, [1, 3, SIZE, SIZE]),
  }
  const results = await session.run(feeds)
  const out = results.embedding as ort.Tensor
  return new Float32Array(out.data as any)
}
