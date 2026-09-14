import { describe, expect, it, vi } from 'vitest'
import { createPreloadScheduler } from './thumbPreload'

/** Loader that records ids and lets the test finish them by hand. */
function manualLoader() {
  const started: number[] = []
  const done: Record<number, () => void> = {}
  const load = (id: number, cb: () => void) => {
    started.push(id)
    done[id] = cb
  }
  return { load, started, finish: (id: number) => done[id]?.() }
}

describe('createPreloadScheduler', () => {
  it('never starts the same id twice', () => {
    const { load, started } = manualLoader()
    const s = createPreloadScheduler(load)
    s.schedule(1, 'ahead')
    s.schedule(1, 'ahead')
    s.schedule(1, 'visible')
    expect(started).toEqual([1])
    expect(s.has(1)).toBe(true)
  })

  it('respects the worker limit', () => {
    const { load, started } = manualLoader()
    const s = createPreloadScheduler(load, 3)
    for (const id of [1, 2, 3, 4, 5]) s.schedule(id, 'ahead')
    expect(started).toEqual([1, 2, 3])
    expect(s.pending()).toBe(5)
  })

  it('starts the next queued id when one finishes', () => {
    const { load, started, finish } = manualLoader()
    const s = createPreloadScheduler(load, 2)
    for (const id of [1, 2, 3]) s.schedule(id, 'ahead')
    expect(started).toEqual([1, 2])
    finish(1)
    expect(started).toEqual([1, 2, 3])
  })

  it('serves visible ids before background ones', () => {
    const { load, started, finish } = manualLoader()
    const s = createPreloadScheduler(load, 1)
    for (const id of [10, 11, 12]) s.schedule(id, 'ahead')
    expect(started).toEqual([10])

    finish(10) // 11 takes the free slot
    expect(started).toEqual([10, 11])

    // 99 arrives on screen while 12 is still waiting behind 11.
    s.schedule(99, 'visible')
    finish(11)
    expect(started).toEqual([10, 11, 99])
  })

  it('promotes an id that was queued ahead and then scrolls into view', () => {
    const { load, started, finish } = manualLoader()
    const s = createPreloadScheduler(load, 1)
    s.schedule(1, 'ahead')
    s.schedule(2, 'ahead')
    s.schedule(3, 'ahead')
    s.schedule(3, 'visible') // still queued, should jump ahead of 2
    finish(1)
    expect(started).toEqual([1, 3])
  })

  it('does not free two slots when a loader calls back twice', () => {
    const { load, started } = manualLoader()
    const s = createPreloadScheduler(load, 1)
    const cb = vi.fn()
    const single = createPreloadScheduler((id, done) => {
      started.push(id)
      cb(id)
      done()
      done() // misbehaving loader
    }, 1)
    single.schedule(1, 'ahead')
    single.schedule(2, 'ahead')
    expect(started).toEqual([1, 2])
  })

  it('reports pending work and can drop what has not started', () => {
    const { load, started } = manualLoader()
    const s = createPreloadScheduler(load, 1)
    s.schedule(1, 'ahead')
    s.schedule(2, 'ahead')
    expect(s.pending()).toBe(2)
    s.clear()
    expect(s.pending()).toBe(1) // the in-flight one still counts
    expect(started).toEqual([1])
  })
})
