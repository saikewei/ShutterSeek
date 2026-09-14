import { describe, expect, it } from 'vitest'
import { activeTabIndex, shellTitle, tabsFor } from './tabs'

describe('tabsFor', () => {
  it('hides the admin tab from guests', () => {
    expect(tabsFor('guest').map((t) => t.to)).toEqual(['/', '/albums', '/search'])
  })

  it('appends the admin tab for admins', () => {
    expect(tabsFor('admin').map((t) => t.to)).toEqual(['/', '/albums', '/search', '/admin'])
  })

  it('treats an unknown role as a guest', () => {
    expect(tabsFor(undefined).map((t) => t.to)).toEqual(['/', '/albums', '/search'])
  })

  it('never offers upload', () => {
    for (const role of ['guest', 'admin']) {
      expect(tabsFor(role).some((t) => t.to.startsWith('/upload'))).toBe(false)
    }
  })
})

describe('activeTabIndex', () => {
  const tabs = tabsFor('admin')

  it('matches the home route exactly', () => {
    expect(activeTabIndex(tabs, '/')).toBe(0)
  })

  it('keeps the album tab active on album detail pages', () => {
    expect(activeTabIndex(tabs, '/albums/42')).toBe(1)
  })

  it('keeps the admin tab active on admin subpages', () => {
    expect(activeTabIndex(tabs, '/admin/logs')).toBe(3)
  })

  it('reports -1 for routes outside the bar', () => {
    expect(activeTabIndex(tabs, '/upload')).toBe(-1)
  })
})

describe('shellTitle', () => {
  it('is empty on home, so the wordmark is rendered instead', () => {
    expect(shellTitle('/')).toBe('')
  })

  it('names the admin subpages', () => {
    expect(shellTitle('/admin')).toBe('管理')
    expect(shellTitle('/admin/logs')).toBe('用户日志')
    expect(shellTitle('/admin/status')).toBe('系统状态')
  })

  it('falls back for unknown routes', () => {
    expect(shellTitle('/nope')).toBe('ShutterSeek')
  })
})
