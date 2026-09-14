import { describe, expect, it } from 'vitest'
import { loadAheadPx, shouldAutoLoad, type AutoLoadState } from './infiniteScroll'

const base: AutoLoadState = {
  hasMore: true,
  loading: false,
  sentinelTop: 800,
  viewportHeight: 844,
  aheadPx: loadAheadPx(844),
  now: 1000,
  retryAfter: 0,
}

describe('shouldAutoLoad', () => {
  it('loads once the sentinel is inside the preload margin', () => {
    expect(shouldAutoLoad(base)).toBe(true)
  })

  it('stops at the end of the list', () => {
    expect(shouldAutoLoad({ ...base, hasMore: false })).toBe(false)
  })

  it('does not start a second request while one is in flight', () => {
    expect(shouldAutoLoad({ ...base, loading: true })).toBe(false)
  })

  it('recovers the trigger that was dropped while a page was in flight', () => {
    // This is the reported regression: the observer fired while loading, the
    // trigger was discarded, the sentinel never left the margin, and no
    // further callback arrived. Re-evaluating the condition after the page
    // lands must pick it up again.
    const whileBusy = { ...base, loading: true }
    expect(shouldAutoLoad(whileBusy)).toBe(false)
    const afterPage = { ...whileBusy, loading: false }
    expect(shouldAutoLoad(afterPage)).toBe(true)
  })

  it('does not keep loading once the sentinel is far below the viewport', () => {
    expect(shouldAutoLoad({ ...base, sentinelTop: 4000 })).toBe(false)
  })

  it('backs off after a failure until the cooldown expires', () => {
    expect(shouldAutoLoad({ ...base, retryAfter: 2000 })).toBe(false)
    expect(shouldAutoLoad({ ...base, retryAfter: 2000, now: 2000 })).toBe(true)
  })

  it('treats a sentinel level with the viewport bottom as in range', () => {
    const ahead = loadAheadPx(base.viewportHeight)
    expect(shouldAutoLoad({ ...base, sentinelTop: base.viewportHeight + ahead })).toBe(true)
    expect(shouldAutoLoad({ ...base, sentinelTop: base.viewportHeight + ahead + 1 })).toBe(false)
  })
})

describe('loadAheadPx', () => {
  it('preloads two viewports ahead', () => {
    expect(loadAheadPx(800)).toBe(1600)
    expect(loadAheadPx(0)).toBe(0)
  })
})
