import { describe, expect, it } from 'vitest'
import { MOBILE_SHELL_QUERIES, matchesMobileShell } from './device'

// The media-query listeners are skipped in a non-browser environment, so this
// module can be imported and only the pure predicate is exercised here.
describe('matchesMobileShell', () => {
  it('is true for a phone-width viewport', () => {
    expect(matchesMobileShell((q) => q === '(max-width: 767px)')).toBe(true)
  })

  it('is true for a coarse-pointer device inside the tablet range', () => {
    expect(matchesMobileShell((q) => q === '(max-width: 1024px) and (pointer: coarse)')).toBe(true)
  })

  it('is false when neither query matches', () => {
    expect(matchesMobileShell(() => false)).toBe(false)
  })

  it('asks about every declared query', () => {
    const asked: string[] = []
    matchesMobileShell((q) => {
      asked.push(q)
      return false
    })
    expect(asked).toEqual([...MOBILE_SHELL_QUERIES])
  })
})
