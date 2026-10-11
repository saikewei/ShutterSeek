import type { Proposal, ReviewReport } from '@/api/creations'

export interface CreationProgress {
  stage?: string
  completed?: number
  total?: number
  round?: number
  attempt?: number
  status?: string
}

export function needsChinese(text: string): boolean {
  return /[A-Za-z]/.test(text) && !/[\u3400-\u9fff]/.test(text)
}

export function displayChinese(text: string | undefined, fallback: string): string {
  return text?.trim() && !needsChinese(text) ? text : fallback
}

export function assistantText(text: string): string {
  try {
    const value = JSON.parse(text)
    const reply = value.reply || (value.proposal ? '我已经整理好当前方案，你可以继续调整，也可以确认后开始生成。' : '')
    const question = value.questions?.[0]
    return [displayChinese(reply, '我们继续完善这份作品。'), question ? displayChinese(question, '你还想补充什么故事，或调整什么风格？') : ''].filter(Boolean).join('\n\n')
  } catch {
    return displayChinese(text, '方案已更新。你可以继续聊想法，或查看当前方案。')
  }
}

export const progressSteps = ['理解照片', '整理方案', '生成网页', '检查效果', '保存作品']
export function progressStep(stage?: string): number {
  if (stage === 'observing' || stage === 'localizing') return 0
  if (stage === 'planning' || stage === 'waiting') return 1
  if (stage === 'designing' || stage === 'generating' || stage === 'correcting') return 2
  if (['rendering', 'rendered', 'reviewing', 'reviewed', 'repairing'].includes(stage || '')) return 3
  if (stage === 'saving' || stage === 'completed') return 4
  return -1
}

export function progressText(progress: CreationProgress): { title: string; detail: string } {
  const round = progress.round || 1
  const phases: Record<string, { title: string; detail: string }> = {
    queued: { title: '任务已提交，正在排队', detail: '轮到后会自动开始，你可以留在这里等待。' },
    started: { title: '创作助手已开始处理', detail: '正在读取当前作品和你提供的信息。' },
    localizing: { title: '正在整理中文描述', detail: '把图片描述和方案转换为中文，保留原有事实。' },
    observing: { title: '正在理解照片', detail: progress.total ? '已理解 ' + (progress.completed || 0) + ' / ' + progress.total + ' 张，继续查看下一批。' : '逐批查看照片，整理画面中能够确认的内容。' },
    planning: { title: '正在整理你的想法', detail: '根据照片和刚才的回复，完善内容与风格。' },
    designing: { title: '正在设计版式与互动', detail: '结合照片挑选视觉语言和互动方式，让作品更有自己的风格。' },
    generating: { title: '正在生成网页', detail: '按照已确认的方案和选图，编排内容与交互。' },
    correcting: { title: '正在校对生成结果', detail: '校对照片和页面内容，让结果符合已确认的方案。' },
    rendering: { title: '正在检查页面效果', detail: '准备桌面和手机页面，查看图片与交互是否正常。' },
    rendered: { title: '桌面与手机截图已准备好', detail: '接下来会查看截图中的排版和可读性。' },
    reviewing: { title: '正在查看页面截图', detail: '第 ' + round + ' 轮检查：排版、图片、手机阅读与交互效果。' },
    reviewed: { title: '本轮效果检查已完成', detail: '正在整理检查结果，决定是否需要调整。' },
    repairing: { title: '正在调整页面效果', detail: '修复发现的问题，随后重新截图检查。' },
    saving: { title: '正在保存作品', detail: '保存版本和检查结果，马上可以预览。' },
    model: { title: '创作助手正在处理', detail: '正在整理结果，完成后会自动更新。' },
  }
  return phases[progress.stage || 'queued'] || phases.queued!
}

export function viewportText(value: string): string {
  return value === 'mobile' ? '手机' : '桌面'
}

export function regionText(value: string): string {
  const known: Record<string, string> = { first: '首屏', middle: '中段', last: '页尾', all: '页面整体', images: '图片区域', interaction: '交互区域', network: '资源加载', page: '页面截图' }
  if (known[value]) return known[value]
  if (value.startsWith('y=')) return '页面位置 ' + value.slice(2)
  return displayChinese(value, '已检查区域')
}

export function reportSummary(report: ReviewReport): string {
  return displayChinese(report.summary, '本次检查结果已保存，请查看下方问题与截图。')
}

export function snapshotProposal(proposal: Proposal): Proposal {
  return { structure: proposal.structure, style: proposal.style, used_photo_ids: [...proposal.used_photo_ids] }
}
