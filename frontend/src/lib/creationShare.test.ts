import { describe, expect, it } from 'vitest'
import { sharedLoginTarget } from './creationShare'
describe('authenticated sharing login return', () => {
  it('returns to the selected shared creation', () => {
    const path = '/creations/shared/11111111-1111-4111-8111-111111111111'
    expect(sharedLoginTarget(path)).toBe(path)
    expect(sharedLoginTarget(path + '?view=mobile')).toBe(path + '?view=mobile')
  })
  it('rejects external URLs, other routes and malformed share IDs', () => {
    for (const value of ['https://example.com', '//example.com', '/admin', '/creations/shared/not-an-id', ['/?next=evil'], null]) {
      expect(sharedLoginTarget(value)).toBe('/')
    }
  })
})
