import { describe, expect, it } from 'vitest'
import { formatUptime } from './admin'

describe('formatUptime', () => {
  it('formats days, hours, minutes and seconds', () => {
    expect(formatUptime(0)).toBe('0 秒')
    expect(formatUptime(45)).toBe('45 秒')
    expect(formatUptime(90)).toBe('1 分')
    expect(formatUptime(3 * 3600 + 120)).toBe('3 小时 2 分')
    expect(formatUptime(2 * 86400 + 5 * 3600)).toBe('2 天 5 小时')
  })

  it('never renders a negative or invalid value', () => {
    expect(formatUptime(-1)).toBe('—')
    expect(formatUptime(Number.NaN)).toBe('—')
  })
})
