import * as UTIF from 'utif'

export class UnsupportedFormatError extends Error {
  constructor(msg: string) {
    super(msg)
    this.name = 'UnsupportedFormatError'
  }
}

const RAW_EXTS = new Set(['.nef', '.cr2', '.cr3', '.arw', '.rw2', '.dng', '.orf', '.raf', '.pef', '.sr2', '.srf'])
const TIFF_EXTS = new Set(['.tif', '.tiff'])
const HEIC_EXTS = new Set(['.heic', '.heif', '.hif'])

export function isRawExt(name: string): boolean {
  return RAW_EXTS.has(extOf(name))
}

export function extOf(name: string): string {
  return '.' + (name.split('.').pop() || '').toLowerCase()
}

/** 解码任意支持格式为 ImageBitmap；EXIF 方向一律不回正（与 Python 端一致）。 */
export async function decodeImage(file: File): Promise<ImageBitmap> {
  const ext = extOf(file.name)
  if (RAW_EXTS.has(ext)) {
    const data = new Uint8Array(await file.arrayBuffer())
    const jpeg = extractEmbeddedJPEG(data)
    if (jpeg) {
      return createImageBitmap(new Blob([jpeg], { type: 'image/jpeg' }), { imageOrientation: 'none' })
    }
    if (ext === '.dng' || ext === '.tif' || ext === '.tiff') {
      return decodeTIFF(data)
    }
    throw new UnsupportedFormatError(`RAW 内没有可用的 JPEG 预览: ${file.name}`)
  }
  if (TIFF_EXTS.has(ext)) {
    return decodeTIFF(new Uint8Array(await file.arrayBuffer()))
  }
  if (HEIC_EXTS.has(ext)) {
    try {
      return await createImageBitmap(file, { imageOrientation: 'none' })
    } catch {
      const { default: heic2any } = await import('heic2any')
      const blob = await heic2any({ blob: file, toType: 'image/jpeg', quality: 0.92 }) as Blob
      return createImageBitmap(blob, { imageOrientation: 'none' })
    }
  }
  return createImageBitmap(file, { imageOrientation: 'none' })
}

function decodeTIFF(data: Uint8Array): Promise<ImageBitmap> {
  const buf = data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength)
  const ifds = UTIF.decode(buf)
  if (!ifds.length) throw new UnsupportedFormatError('TIFF 无法解码')
  UTIF.decodeImage(buf, ifds[0])
  const rgba = UTIF.toRGBA8(ifds[0])
  const canvas = document.createElement('canvas')
  canvas.width = ifds[0].width
  canvas.height = ifds[0].height
  const ctx = canvas.getContext('2d')!
  const img = ctx.createImageData(canvas.width, canvas.height)
  img.data.set(rgba)
  ctx.putImageData(img, 0, 0)
  return createImageBitmap(canvas)
}

/** 从 RAW 二进制中提取最大的内嵌 JPEG 预览（移植自后端 original.go）。 */
export function extractEmbeddedJPEG(data: Uint8Array): Uint8Array | null {
  let best: Uint8Array | null = null
  const tiffJPEG = extractTIFFJPEG(data)
  if (tiffJPEG) best = tiffJPEG
  const scanned = extractJPEG(data)
  if (scanned && (!best || scanned.length > best.length)) best = scanned
  return best
}

function extractTIFFJPEG(data: Uint8Array): Uint8Array | null {
  if (data.length < 8) return null
  let little: boolean
  if (data[0] === 0x49 && data[1] === 0x49) little = true
  else if (data[0] === 0x4d && data[1] === 0x4d) little = false
  else return null
  const u16 = (o: number) => (little ? data[o] | (data[o + 1] << 8) : (data[o] << 8) | data[o + 1])
  const u32 = (o: number) => (little
    ? data[o] | (data[o + 1] << 8) | (data[o + 2] << 16) | (data[o + 3] << 24)
    : (data[o] << 24) | (data[o + 1] << 16) | (data[o + 2] << 8) | data[o + 3])
  if (u16(2) !== 42) return null
  const start = u32(4)
  if (start < 8 || start >= data.length) return null
  let best: Uint8Array | null = null
  const walk = (off: number, depth: number) => {
    if (off < 8 || off + 2 > data.length || depth > 8) return
    const count = u16(off)
    if (count > 512) return
    const dir = off + 2
    for (let i = 0; i < count; i++) {
      const e = dir + i * 12
      if (e + 12 > data.length) return
      const tag = u16(e)
      const typ = u16(e + 2)
      const cnt = u32(e + 4)
      const valOff = e + 8
      if (tag === 0x0201 && typ === 4 && cnt === 1 && valOff + 4 <= data.length) {
        const o = u32(valOff)
        if (o > 0 && o < data.length) {
          const seg = extractJPEG(data.subarray(o))
          if (seg && (!best || seg.length > best.length)) best = seg
        }
      } else if ((tag === 0x8769 || tag === 0x8825) && typ === 4 && cnt === 1 && valOff + 4 <= data.length) {
        walk(u32(valOff), depth + 1)
      } else if (tag === 0x014a && typ === 4 && cnt >= 1 && cnt <= 64) {
        const ptrs: number[] = []
        if (cnt * 4 <= 4) {
          for (let k = 0; k < cnt; k++) ptrs.push(u32(valOff + 4 * k))
        } else {
          const addr = u32(valOff)
          for (let k = 0; k < cnt; k++) {
            const o = addr + 4 * k
            if (o + 4 > data.length) break
            ptrs.push(u32(o))
          }
        }
        for (const p of ptrs) walk(p, depth + 1)
      }
    }
    const next = dir + count * 12
    if (next + 4 <= data.length) {
      const n = u32(next)
      if (n > 0) walk(n, depth + 1)
    }
  }
  walk(start, 0)
  return best
}

function extractJPEG(data: Uint8Array): Uint8Array | null {
  let best: Uint8Array | null = null
  let pos = 0
  while (pos < data.length - 1) {
    if (data[pos] === 0xff && data[pos + 1] === 0xd8) {
      let end = pos + 2
      while (end < data.length - 1) {
        if (data[end] === 0xff && data[end + 1] === 0xd9) {
          const cand = data.slice(pos, end + 2)
          if (isValidJPEG(cand) && (!best || cand.length > best.length)) best = cand
          pos = end + 2
          break
        }
        end++
      }
      if (end >= data.length - 1) break
    }
    pos++
  }
  return best
}

function isValidJPEG(data: Uint8Array): boolean {
  if (data.length < 4 || data[0] !== 0xff || data[1] !== 0xd8) return false
  let hasSOF = false
  let hasSOS = false
  let i = 2
  while (i < data.length - 1) {
    if (data[i] !== 0xff) {
      i++
      continue
    }
    const marker = data[i + 1]
    if (marker === 0x00) {
      i += 2
      continue
    }
    if (marker === 0xff) {
      i++
      continue
    }
    if (marker === 0xd9) break
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4) {
      hasSOF = true
      if (i + 4 > data.length) return false
      i += 2 + ((data[i + 2] << 8) | data[i + 3])
    } else if (marker === 0xda) {
      hasSOS = true
      if (i + 4 > data.length) return false
      i += 2 + ((data[i + 2] << 8) | data[i + 3])
    } else if (marker >= 0xd0 && marker <= 0xd7) {
      i += 2
    } else {
      if (!isJPEGMarker(marker)) return false
      if (i + 4 > data.length) return false
      i += 2 + ((data[i + 2] << 8) | data[i + 3])
    }
  }
  return hasSOF && hasSOS
}

function isJPEGMarker(b: number): boolean {
  if (b === 0xfe) return true
  if (b < 0xc0) return false
  if (b === 0xd8 || b === 0xd9) return false
  if (b >= 0xf0) return false
  return true
}
