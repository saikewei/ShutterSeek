// Bottom tab bar definition.
//
// Kept free of DOM and component code so the active-tab rules and the
// role-dependent tab set can be unit-tested.

export interface MobileTab {
  to: string
  label: string
  /** Heroicons v2 outline path data, drawn inside a 24x24 viewBox. */
  paths: string[]
  /** True when the given route path belongs to this tab. */
  match: (path: string) => boolean
}

const PHOTOS: MobileTab = {
  to: '/',
  label: '全部照片',
  paths: [
    'M2.25 15.75l5.159-5.159a2.25 2.25 0 0 1 3.182 0l5.159 5.159m-1.5-1.5 1.409-1.409a2.25 2.25 0 0 1 3.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 0 0 1.5-1.5V6a1.5 1.5 0 0 0-1.5-1.5H3.75A1.5 1.5 0 0 0 2.25 6v12a1.5 1.5 0 0 0 1.5 1.5Zm10.5-11.25h.008v.008H12.75V8.25Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z',
  ],
  match: (path) => path === '/',
}

const ALBUMS: MobileTab = {
  to: '/albums',
  label: '相册',
  paths: [
    'M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z',
  ],
  match: (path) => path.startsWith('/albums'),
}

const SEARCH: MobileTab = {
  to: '/search',
  label: '搜索',
  paths: [
    'm21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z',
  ],
  match: (path) => path.startsWith('/search'),
}

// Upload is deliberately absent: the mobile admin console covers everything
// except uploading, which stays a desktop-only flow.
const ADMIN: MobileTab = {
  to: '/admin',
  label: '管理',
  paths: [
    'M10.5 6h9.75M10.5 6a1.5 1.5 0 1 1-3 0m3 0a1.5 1.5 0 1 0-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-9.75 0h9.75',
  ],
  match: (path) => path.startsWith('/admin'),
}

/** Tabs available to the given role, in display order. */
export function tabsFor(role: string | null | undefined): MobileTab[] {
  const tabs = [PHOTOS, ALBUMS, SEARCH]
  if (role === 'admin') tabs.push(ADMIN)
  return tabs
}

/** Index of the tab owning `path`, or -1 when the route belongs to none. */
export function activeTabIndex(tabs: MobileTab[], path: string): number {
  return tabs.findIndex((tab) => tab.match(path))
}

/** Title shown in the mobile top bar; '' means "render the wordmark". */
export function shellTitle(path: string): string {
  if (path === '/') return ''
  if (path.startsWith('/albums')) return '相册'
  if (path.startsWith('/search')) return '搜索'
  if (path.startsWith('/admin/logs')) return '用户日志'
  if (path.startsWith('/admin/invites')) return '邀请码'
  if (path.startsWith('/admin/status')) return '系统状态'
  if (path.startsWith('/admin')) return '管理'
  if (path.startsWith('/upload')) return '上传照片'
  return 'ShutterSeek'
}
