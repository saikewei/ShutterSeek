import { describe, expect, it, vi } from 'vitest'
import { createUploadQueue, type QueueDeps, type QueueItem } from './uploadQueue'
import type { BatchUploadItem, BatchUploadResponse } from '@/api/photos'

// node 环境没有 File/Bitmap 这些浏览器对象，用最小替身即可：
// 队列引擎只用到 file.name / file.size。
function fakeFile(name: string, size = 1024): File {
  return { name, size, type: 'image/jpeg' } as unknown as File
}

function fakeBitmap() {
  return { width: 4000, height: 3000, close: vi.fn() }
}

function okResponse(ids: number[]): BatchUploadResponse {
  return {
    results: ids.map((id, index) => ({
      index,
      filename: `f${index}`,
      status: 'created' as const,
      id,
      thumbnail: true,
      duplicate: false,
    })),
    summary: { created: ids.length, duplicate: 0, failed: 0, elapsed_ms: 1 },
  }
}

interface Harness {
  deps: QueueDeps
  uploadCalls: BatchUploadItem[][]
  decodeConcurrent: { max: number }
}

/** 造一套可观测的依赖替身：记录每批上传的内容与解码并发峰值。 */
function makeHarness(overrides: Partial<QueueDeps> = {}): Harness {
  const uploadCalls: BatchUploadItem[][] = []
  const decodeConcurrent = { max: 0 }
  let decoding = 0
  let nextId = 100

  const deps: QueueDeps = {
    decode: async () => {
      decoding++
      decodeConcurrent.max = Math.max(decodeConcurrent.max, decoding)
      await new Promise((r) => setTimeout(r, 5))
      decoding--
      return fakeBitmap() as unknown as ImageBitmap
    },
    preprocess: async () => ({ tensor: true }),
    embed: async () => new Float32Array(1024).fill(0.03125),
    derivePreview: async () => ({}),
    upload: async (items) => {
      uploadCalls.push(items)
      await new Promise((r) => setTimeout(r, 5))
      return okResponse(items.map(() => nextId++))
    },
    ...overrides,
  }
  return { deps, uploadCalls, decodeConcurrent }
}

async function waitIdle(q: ReturnType<typeof createUploadQueue>, timeoutMs = 3000) {
  const t0 = Date.now()
  const busy = (i: QueueItem) => i.state === 'waiting' || i.state === 'decoding' || i.state === 'embedding' || i.state === 'uploading'
  while (q.running.value || q.items.value.some(busy)) {
    if (Date.now() - t0 > timeoutMs) throw new Error('queue did not settle')
    await new Promise((r) => setTimeout(r, 5))
  }
}

describe('uploadQueue', () => {
  it('三级流水线：全部文件走到 done，并逐项带向量', async () => {
    const h = makeHarness()
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('a.jpg'), fakeFile('b.jpg'), fakeFile('c.jpg')])
    q.start()
    await waitIdle(q)

    expect(q.items.value.map((i) => i.state)).toEqual(['done', 'done', 'done'])
    expect(q.items.value.every((i) => i.photoId)).toBe(true)
    const sent = h.uploadCalls.flat()
    expect(sent.length).toBe(3)
    expect(sent[0].vector.length).toBe(1024)
    expect(q.stats.value.done).toBe(3)
    expect(q.stats.value.queued).toBe(0)
  })

  it('大文件单独成批（换字节级进度），小文件成批不超过 batchMax', async () => {
    const h = makeHarness()
    const q = createUploadQueue({ deps: h.deps, batchMax: 4, largeFileBytes: 8 * 1024 * 1024 })
    const big = fakeFile('big.nef', 20 * 1024 * 1024)
    q.add([big, fakeFile('s1.jpg'), fakeFile('s2.jpg'), fakeFile('s3.jpg'), fakeFile('s4.jpg'), fakeFile('s5.jpg')])
    q.start()
    await waitIdle(q)

    const sizes = h.uploadCalls.map((b) => b.length)
    expect(sizes).toContain(1) // 大文件单独一批
    expect(sizes.filter((n) => n > 1).every((n) => n <= 4)).toBe(true)
    expect(h.uploadCalls.find((b) => b.length === 1)![0].file.name).toBe('big.nef')
    expect(h.uploadCalls.flat().length).toBe(6)
  })

  it('小文件不带 preview 字段，大文件才带上（避免给小图加传输量）', async () => {
    const h = makeHarness({
      derivePreview: async () => ({ previewBlob: { type: 'image/jpeg' } as Blob }),
    })
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('small.jpg', 1024), fakeFile('big.jpg', 20 * 1024 * 1024)])
    q.start()
    await waitIdle(q)

    const all = h.uploadCalls.flat()
    expect(all.find((i) => i.file.name === 'small.jpg')!.preview).toBeFalsy()
    expect(all.find((i) => i.file.name === 'big.jpg')!.preview).toBeTruthy()
  })

  it('服务端的 duplicate 判为重复而不是失败', async () => {
    const h = makeHarness({
      upload: async (items) => ({
        results: items.map((_, index) => ({
          index,
          filename: 'x',
          status: 'duplicate' as const,
          duplicate: true,
          existing_id: 42,
        })),
        summary: { created: 0, duplicate: items.length, failed: 0, elapsed_ms: 1 },
      }),
    })
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('dup.jpg')])
    q.start()
    await waitIdle(q)

    expect(q.items.value[0].state).toBe('duplicate')
    expect(q.stats.value.duplicate).toBe(1)
    expect(q.stats.value.failed).toBe(0)
  })

  it('单批里逐项成败：一项失败不影响同批其它项', async () => {
    const h = makeHarness({
      upload: async (items) => ({
        results: items.map((it, index) => {
          // 服务端按项返回结果；用什么字段判成败由服务端决定，这里按文件名模拟
          const bad = it.file.name === 'bad.jpg'
          return {
            index,
            filename: it.file.name,
            status: bad ? ('error' as const) : ('created' as const),
            duplicate: false,
            error: bad ? 'invalid vector' : undefined,
            id: bad ? undefined : 7,
          }
        }),
        summary: { created: 1, duplicate: 0, failed: 1, elapsed_ms: 1 },
      }),
    })
    const q = createUploadQueue({ deps: h.deps, decodeConcurrency: 2 })
    q.add([fakeFile('ok.jpg'), fakeFile('bad.jpg')])
    q.start()
    await waitIdle(q)

    const ok = q.items.value.find((i) => i.file.name === 'ok.jpg')!
    const bad = q.items.value.find((i) => i.file.name === 'bad.jpg')!
    expect(ok.state).toBe('done')
    expect(bad.state).toBe('error')
    expect(bad.error).toBe('invalid vector')
    expect(q.stats.value.failed).toBe(1)
  })

  it('上传抛错 → 该项 error，重试后能成功且不重跑解码', async () => {
    let fail = true
    const decode = vi.fn(async () => fakeBitmap() as unknown as ImageBitmap)
    const h = makeHarness({
      decode,
      upload: async (items) => {
        if (fail) throw new Error('network down')
        return okResponse(items.map((_, i) => 900 + i))
      },
    })
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('retry.jpg')])
    q.start()
    await waitIdle(q)
    expect(q.items.value[0].state).toBe('error')
    expect(q.items.value[0].error).toBe('network down')

    fail = false
    q.retryFailed()
    await waitIdle(q)
    expect(q.items.value[0].state).toBe('done')
    expect(decode).toHaveBeenCalledTimes(1) // 向量已算好，重试只重发
  })

  it('解码并发不超过配置上限', async () => {
    const h = makeHarness()
    const q = createUploadQueue({ deps: h.deps, decodeConcurrency: 2 })
    q.add(Array.from({ length: 8 }, (_, i) => fakeFile(`c${i}.jpg`)))
    q.start()
    await waitIdle(q)
    expect(h.decodeConcurrent.max).toBeLessThanOrEqual(2)
    expect(q.stats.value.done).toBe(8)
  })

  it('cancelAll 后不再发出新上传，未完成项标记取消', async () => {
    const h = makeHarness()
    const q = createUploadQueue({ deps: h.deps, decodeConcurrency: 1 })
    q.add(Array.from({ length: 5 }, (_, i) => fakeFile(`x${i}.jpg`)))
    q.start()
    q.cancelAll()
    await waitIdle(q)
    expect(q.items.value.every((i) => i.state === 'canceled' || i.state === 'done')).toBe(true)
    expect(q.items.value.some((i) => i.state === 'canceled')).toBe(true)
  })

  it('超过 200MB 的文件不入队', async () => {
    const h = makeHarness()
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('huge.jpg', 201 * 1024 * 1024), fakeFile('ok.jpg')])
    expect(q.items.value.map((i) => i.file.name)).toEqual(['ok.jpg'])
  })

  it('remove/clearFinished 会释放预览 objectURL', async () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const h = makeHarness({ derivePreview: async () => ({ thumbUrl: 'blob:fake' }) })
    const q = createUploadQueue({ deps: h.deps })
    q.add([fakeFile('p.jpg')])
    q.start()
    await waitIdle(q)
    q.clearFinished()
    expect(revoke).toHaveBeenCalledWith('blob:fake')
    revoke.mockRestore()
  })
})
