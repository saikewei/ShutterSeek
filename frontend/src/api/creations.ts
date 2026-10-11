import { api } from './client'

export interface Creation { id: string; title: string; current_version: string }
export interface CreationRun { id: string; status: string; report: string; action?: string; created_at?: string; proposal?: Proposal; photo_revision?: number }
export interface CreationVersion { id: string; verified: boolean; report: string; used_photo_ids: string; created_at: string }
export interface Proposal { structure: string; style: string; used_photo_ids: number[] }
export interface PhotoSelection { photo_ids: number[]; used_photo_ids: number[]; rotations: Record<string, number>; title?: string }
export interface WorkState {
  photo_ids?: number[]
  selected_photo_ids?: number[]
  photo_edits?: Record<string, number>
  photo_revision?: number
  photos_changed?: boolean
  proposal?: Proposal
  reply?: string
  question_count?: number
  asked_questions?: string[]
  questions?: string[]
  unknowns?: string[]
  observations?: Array<{ photo_id: number; observation: string; unknowns: string[] }>
  user_facts?: Array<{ message_id: string; text: string; display_text?: string }>
}
export interface WorkDetail {
  creation: Creation
  photo_ids: number[]
  state: WorkState
  messages: Array<{ id: string; role: string; content: string }>
  runs: CreationRun[]
  versions: CreationVersion[]
}
export interface ReviewReport {
  rounds?: number
  summary?: string
  error?: string
  issues?: Array<{ severity: string; viewport: string; region: string; element: string; description: string; suggestion: string }>
  screenshots?: Array<{ id: string; viewport: string; kind: string; y?: number }>
  limitations?: string
}
export function parseReport(text?: string): ReviewReport {
  try { return JSON.parse(text || '{}') } catch { return {} }
}
export async function listCreations(): Promise<Creation[]> { return (await api.get('/creations')).data }
export async function createCreation(input: { title?: string; photo_ids?: number[]; album_id?: number; used_photo_ids?: number[]; rotations?: Record<string, number> }): Promise<Creation> {
  return (await api.post('/creations', input)).data
}
export async function getCreation(id: string): Promise<WorkDetail> { return (await api.get('/creations/' + id)).data }
export async function startCreationRun(id: string, input: { action: string; message: string; base_version: string; proposal?: Proposal; photo_revision?: number }): Promise<CreationRun> {
  return (await api.post('/creations/' + id + '/runs', input)).data
}
export async function cancelCreationRun(id: string, run: string) { await api.post('/creations/' + id + '/runs/' + run + '/cancel') }
export async function restoreCreationVersion(id: string, version: string, base: string) {
  await api.post('/creations/' + id + '/versions/' + version + '/restore', { base_version: base })
}
export function previewURL(id: string, version: string): string {
  return '/api/v1/creations/' + encodeURIComponent(id) + '/preview?version=' + encodeURIComponent(version)
}


export async function updateCreationPhotos(id: string, input: PhotoSelection & { base_version: string; base_revision: number; proposal?: Proposal }) {
  await api.patch('/creations/' + encodeURIComponent(id) + '/photos', input)
}
