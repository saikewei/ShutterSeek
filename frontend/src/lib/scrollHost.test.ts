import { afterEach, describe, expect, it, vi } from 'vitest'
import { getScrollTop, setScrollTop, hostScrollHeight, isScrollHostNearEnd, scrollHostToEnd } from './scrollHost'

afterEach(() => vi.unstubAllGlobals())

describe('explicit editor scroll hosts', () => {
  it('scrolls the transcript without moving the shell', () => {
    const shell = { scrollTop: 12, scrollHeight: 800, clientHeight: 800 } as HTMLElement
    const transcript = { scrollTop: 20, scrollHeight: 1600, clientHeight: 400 } as HTMLElement
    vi.stubGlobal('document', { querySelector: () => shell })
    expect(getScrollTop()).toBe(12)
    expect(getScrollTop(transcript)).toBe(20)
    setScrollTop(300, transcript)
    expect(getScrollTop(transcript)).toBe(300)
    expect(getScrollTop()).toBe(12)
    expect(hostScrollHeight(transcript)).toBe(1600)
    scrollHostToEnd(transcript)
    expect(getScrollTop(transcript)).toBe(1600)
    expect(getScrollTop()).toBe(12)
  })
  it('distinguishes reading history from following new messages', () => {
    const transcript = { scrollTop: 100, scrollHeight: 1600, clientHeight: 400 } as HTMLElement
    expect(isScrollHostNearEnd(transcript)).toBe(false)
    setScrollTop(1160, transcript)
    expect(isScrollHostNearEnd(transcript)).toBe(true)
  })
})
