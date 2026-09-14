import { api } from './client'

export interface SystemStats {
  photos: number
  embeddings: number
  albums: number
  public_albums: number
  users: number
  admins: number
  invites_pending: number
  invites_total: number
  logs: number
  db_ok: boolean
  redis_ok: boolean
  embed_ok: boolean
  started_at: string
  uptime_seconds: number
}

/** Read-only system snapshot for the admin console. Admin only. */
export async function fetchStats(): Promise<SystemStats> {
  const { data } = await api.get<SystemStats>('/admin/stats')
  return data
}

/** "3 天 4 小时" style uptime, built from the server's own second count. */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分`
  if (minutes > 0) return `${minutes} 分`
  return `${Math.floor(seconds)} 秒`
}
