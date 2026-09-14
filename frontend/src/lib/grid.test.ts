import { describe, expect, it } from 'vitest'
import { colsFor } from './grid'

describe('colsFor', () => {
  it('follows the Tailwind breakpoints on desktop', () => {
    expect(colsFor(390)).toBe(3)
    expect(colsFor(768)).toBe(4)
    expect(colsFor(1024)).toBe(5)
    expect(colsFor(1280)).toBe(6)
  })

  it('lets the mobile density override win', () => {
    expect(colsFor(390, 4)).toBe(4)
    expect(colsFor(1600, 5)).toBe(5)
  })

  it('ignores a zero override', () => {
    expect(colsFor(1600, 0)).toBe(6)
    expect(colsFor(1600, null)).toBe(6)
  })

  it('never disagrees with the md breakpoint used by the grid classes', () => {
    // Regression guard: 767/768 used to straddle 3 vs 4 columns.
    expect(colsFor(767)).toBe(3)
    expect(colsFor(768)).toBe(4)
  })
})
