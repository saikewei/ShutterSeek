import { api } from './client'

export interface User {
  id: number
  username: string
  role: 'admin' | 'guest'
}

export interface InviteCode {
  id: number
  code: string
  created_by: number
  used_by: number | null
  expires_at: string
  created_at: string
}

export async function login(username: string, password: string): Promise<User> {
  const { data } = await api.post<User>('/auth/login', { username, password })
  return data
}

export async function logout(): Promise<void> {
  await api.post('/auth/logout')
}

export async function getMe(): Promise<User> {
  const { data } = await api.get<User>('/auth/me')
  return data
}

export async function createInvite(): Promise<InviteCode> {
  const { data } = await api.post<InviteCode>('/invites')
  return data
}

export async function listInvites(): Promise<{ items: InviteCode[] }> {
  const { data } = await api.get<{ items: InviteCode[] }>('/invites')
  return data
}

export async function deleteInvite(id: number): Promise<void> {
  await api.delete(`/invites/${id}`)
}

export async function redeemInvite(
  code: string,
  username: string,
  password: string
): Promise<User> {
  const { data } = await api.post<User>('/invites/redeem', { code, username, password })
  return data
}
