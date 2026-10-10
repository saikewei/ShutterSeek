import { describe, expect, it, vi } from 'vitest'
import { parseReport, previewURL, startCreationRun } from './creations'
import { api } from './client'

describe('creation contracts', () => {
  it('contains malformed persisted reports', () => {
    expect(parseReport('invalid')).toEqual({})
    expect(parseReport('{"rounds":3,"issues":[]}')).toEqual({ rounds: 3, issues: [] })
  })
  it('encodes preview selectors without granting sandbox permissions', () => {
    expect(previewURL('a/b', 'v&x')).toBe('/api/v1/creations/a%2Fb/preview?version=v%26x')
  })
  it('sends the confirmed selection and optimistic base version', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { id: 'run', status: 'queued' } })
    const proposal = { structure: 'Story', style: 'Clean', used_photo_ids: [1, 2] }
    await startCreationRun('work', { action: 'confirm', message: '', base_version: 'old', proposal })
    expect(post).toHaveBeenCalledWith('/creations/work/runs', {
      action: 'confirm', message: '', base_version: 'old', proposal,
    })
    post.mockRestore()
  })
})
