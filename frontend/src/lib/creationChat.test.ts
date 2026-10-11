import { describe, expect, it } from 'vitest'
import { reactive } from 'vue'
import { snapshotProposal, assistantText, displayChinese, progressText, progressStep, viewportText, regionText } from './creationChat'

describe('creation conversation', () => {
  it('takes a stable request snapshot from a reactive proposal', () => {
    const proposal = reactive({ structure: '旅行故事', style: '简约', used_photo_ids: [1] })
    const snapshot = snapshotProposal(proposal)
    expect(structuredClone(snapshot)).toEqual({ structure: '旅行故事', style: '简约', used_photo_ids: [1] })
    proposal.used_photo_ids.push(2)
    proposal.structure = '新的故事'
    expect(snapshot.structure).toBe('旅行故事')
    expect(snapshot.used_photo_ids).toEqual([1])
  })

  it('renders one question from legacy multi-question replies', () => {
    const message = JSON.stringify({ reply: '我已经记下你的回答。', questions: ['在哪里拍的？', '什么时候？', '和谁？'] })
    expect(assistantText(message)).toBe('我已经记下你的回答。\n\n在哪里拍的？')
  })
  it('keeps English legacy descriptions out of the Chinese interface', () => {
    expect(displayChinese('A mountain landscape', '等待中文更新')).toBe('等待中文更新')
    expect(displayChinese('在 Bali 拍摄的照片', '等待中文更新')).toBe('在 Bali 拍摄的照片')
    expect(assistantText(JSON.stringify({ proposal: { structure: 'Travel story' }, questions: ['Where?'] }))).not.toContain('Where?')
  })
  it('reports phase and observed photo counts without invented percentages', () => {
    expect(progressText({ stage: 'observing', completed: 8, total: 30 }).detail).toContain('8 / 30')
    expect(progressStep('generating')).toBe(2)
    expect(progressText({ stage: 'reviewing', round: 2 }).detail).toContain('第 2 轮')
    expect(progressText({ stage: 'saving' }).title).toBe('正在保存作品')
    expect(progressText({ stage: 'queued' }).title).toContain('排队')
  })
  it('labels diagnostic metadata in Chinese', () => {
    expect(viewportText('mobile')).toBe('手机')
    expect(regionText('interaction')).toBe('交互区域')
    expect(regionText('first')).toBe('首屏')
    expect(regionText('y=900')).toBe('页面位置 900')
  })
})
