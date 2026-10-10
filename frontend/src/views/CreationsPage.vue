<template>
  <div class="p-3 sm:p-5 space-y-4">
    <div class="flex items-center gap-3">
      <RouterLink v-if="workId" to="/creations" class="btn-ghost px-2 py-1">← 作品</RouterLink>
      <h1 class="text-xl font-display">{{ detail?.creation.title || 'AI 网页创作' }}</h1>
      <span v-if="activeRun" class="text-xs text-ink-3" role="status">{{ stageLabel }}</span>
    </div>
    <p v-if="error" role="alert" class="text-danger-ink">{{ error }}</p>
    <p v-if="!creationsEnabled">AI 创作尚未启用。</p>
    <template v-else-if="!workId">
      <p class="text-sm text-ink-2">从相册或照片网格选择 1–200 张照片，创作故事页、旅行展示、时间线或画廊。补充信息仅保存在当前作品。</p>
      <RouterLink to="/albums" class="btn-primary px-4 py-2 inline-flex">选择相册</RouterLink>
      <div class="grid sm:grid-cols-2 gap-3">
        <RouterLink v-for="work in works" :key="work.id" :to="'/creations/' + work.id" class="p-4 border border-line rounded-xl bg-raised">
          <h2>{{ work.title }}</h2><p class="text-xs text-ink-3 mt-2">{{ work.current_version ? '已保存作品' : '等待创作' }}</p>
        </RouterLink>
      </div>
    </template>
    <template v-else-if="detail">
      <div v-if="isMobileShell" class="flex gap-2" aria-label="创作面板">
        <button v-for="item in ['对话', '预览']" :key="item" class="btn-ghost px-4 py-2" :aria-pressed="panel === item" @click="panel = item">{{ item }}</button>
      </div>
      <div class="creation-panels" :class="{ 'creation-mobile': isMobileShell }">
        <section v-show="!isMobileShell || panel === '对话'" class="space-y-4 min-w-0">
          <div v-for="message in detail.messages" :key="message.id" class="rounded-xl p-3 border border-line bg-raised">
            <p class="text-xs text-ink-3 mb-1">{{ message.role === 'user' ? '你' : '创作助手' }}</p>
            <p class="whitespace-pre-wrap break-words text-sm">{{ readableMessage(message.content) }}</p>
          </div>
          <div v-if="detail.state.questions?.length && !activeRun && !detail.creation.current_version" class="space-y-3">
            <label v-for="(question, index) in detail.state.questions" :key="question" class="block text-sm">
              {{ question }}
              <input v-model="answers[index]" class="creation-input mt-1" placeholder="可留空跳过" />
            </label>
          </div>
          <div v-if="proposal && !activeRun && !detail.creation.current_version" class="space-y-3 border border-line rounded-xl p-3">
            <h2 class="font-medium">确认内容与风格</h2>
            <label class="block text-sm">内容结构<textarea v-model="proposal.structure" class="creation-input mt-1" rows="3" /></label>
            <label class="block text-sm">视觉风格<input v-model="proposal.style" class="creation-input mt-1" /></label>
            <p class="text-xs text-ink-3">下方勾选将使用的照片，再确认生成。</p>
            <button class="btn-primary px-4 py-2" :disabled="busy || !proposal.used_photo_ids.length" @click="run('confirm')">确认并生成</button>
          </div>
          <label class="block text-sm">创作要求或修改意见
            <textarea v-model="message" rows="3" class="creation-input mt-1" placeholder="想做什么样的作品？也可以补充照片背后的真实故事。" :disabled="!!activeRun" />
          </label>
          <div class="flex flex-wrap gap-2">
            <button class="btn-primary px-4 py-2" :disabled="busy || !!activeRun" @click="run(detail.creation.current_version ? 'modify' : 'answer')">
              {{ detail.creation.current_version ? '对话修改' : '发送 / 更新方案' }}
            </button>
            <button v-if="!detail.creation.current_version" class="btn-ghost px-4 py-2" :disabled="busy || !!activeRun" @click="run('skip')">跳过问题</button>
            <button v-if="activeRun" class="btn-ghost px-4 py-2" @click="cancel">取消任务</button>
          </div>
          <p class="text-xs text-ink-3">关闭页面不会取消任务，回来后可恢复进度。生成与修改后都会重新截图审查，最多修复两轮。</p>
          <details>
            <summary class="cursor-pointer text-sm">图片观察、你提供的事实与未知信息</summary>
            <h3 class="mt-2 text-sm">图片观察（可能不准确）</h3>
            <p v-for="observation in detail.state.observations" :key="observation.photo_id" class="text-xs mt-1">#{{ observation.photo_id }}：{{ observation.observation }}</p>
            <h3 class="mt-2 text-sm">你提供的信息</h3>
            <p v-for="fact in detail.state.user_facts" :key="fact.message_id" class="text-xs mt-1">{{ fact.text }}</p>
            <h3 class="mt-2 text-sm">未知信息</h3>
            <p v-for="unknown in detail.state.unknowns" :key="unknown" class="text-xs mt-1">{{ unknown }}</p>
          </details>
        </section>
        <section v-show="!isMobileShell || panel === '预览'" class="min-w-0 space-y-3">
          <div class="flex items-center gap-2 flex-wrap">
            <label class="text-sm">版本
              <select v-model="version" class="creation-input !w-auto ml-2">
                <option value="">尚无版本</option>
                <option v-for="item in detail.versions" :key="item.id" :value="item.id">{{ new Date(item.created_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) }} · {{ item.verified ? '检查通过' : '候选 / 未通过' }}</option>
              </select>
            </label>
            <button v-if="version && version !== detail.creation.current_version" :disabled="!!activeRun || busy" class="btn-ghost px-3 py-1" @click="restore">恢复此版本</button>
          </div>
          <iframe v-if="version" :key="version" :src="previewURL(workId, version)" sandbox="allow-scripts" referrerpolicy="no-referrer" title="作品沙箱预览" class="creation-preview" />
          <div v-else class="border border-dashed border-line rounded-xl p-10 text-ink-3 text-center">确认方案后，这里将显示可交互作品。</div>
          <div v-if="report.rounds || report.error" class="border border-line rounded-xl p-3 space-y-2">
            <h2 class="font-medium">视觉检查 · {{ report.rounds || 0 }} 轮</h2>
            <p class="text-sm">{{ report.summary || report.error }}</p>
            <p v-for="(issue, index) in report.issues" :key="index" class="text-sm">
              {{ issue.severity === 'blocking' ? '阻断' : '提示' }} · {{ issue.viewport }} · {{ issue.region }} · {{ issue.element }}：{{ issue.description }}；{{ issue.suggestion }}
            </p>
            <div class="grid grid-cols-2 gap-2">
              <a v-for="shot in report.screenshots" :key="shot.id" :href="screenshotURL(shot.id)" target="_blank" rel="noopener noreferrer" class="text-xs">
                <img :src="screenshotURL(shot.id)" :alt="shot.viewport + ' ' + shot.kind" loading="lazy" class="w-full border border-line" />
                {{ shot.viewport }} · {{ shot.kind }} · {{ shot.y ?? 0 }}
              </a>
            </div>
            <p class="text-xs text-ink-3">仅检查报告列出的区域和交互；模型可能遗漏问题。</p>
          </div>
        </section>
      </div>
      <details open class="border-t border-line pt-3">
        <summary class="cursor-pointer text-sm">选图：使用 {{ usedIDs.length }} 张 / 未使用 {{ detail.photo_ids.length - usedIDs.length }} 张</summary>
        <div class="grid grid-cols-4 sm:grid-cols-8 gap-2 mt-3">
          <label v-for="id in detail.photo_ids" :key="id" class="relative text-xs">
            <img :src="THUMB_BASE + '/' + id + '.webp'" :alt="'照片 ' + id" loading="lazy" class="w-full aspect-square object-cover rounded" :class="{ 'opacity-40': !usedIDs.includes(id) }" />
            <input v-if="proposal && !activeRun && !detail.creation.current_version" v-model="proposal.used_photo_ids" type="checkbox" :value="id" class="absolute top-1 left-1" :aria-label="'使用照片 ' + id" />
            <span>{{ usedIDs.includes(id) ? '使用' : '未使用' }} #{{ id }}</span>
          </label>
        </div>
      </details>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { THUMB_BASE } from '@/api/client'
import { creationsEnabled } from '@/stores/creations'
import { isMobileShell } from '@/stores/device'
import { listCreations, getCreation, startCreationRun, cancelCreationRun, restoreCreationVersion, previewURL, parseReport, type Creation, type WorkDetail, type Proposal } from '@/api/creations'

const route = useRoute()
const workId = computed(() => String(route.params.id || ''))
const works = ref<Creation[]>([])
const detail = ref<WorkDetail | null>(null)
const proposal = ref<Proposal | null>(null)
const message = ref('')
const answers = ref<string[]>([])
const version = ref('')
const error = ref('')
const busy = ref(false)
const stage = ref('')
const panel = ref('对话')
let source: EventSource | null = null
const activeRun = computed(() => detail.value?.runs.find(r => r.status === 'queued' || r.status === 'running'))
const usedIDs = computed(() => {
  const saved = detail.value?.versions.find(v => v.id === version.value)
  if (saved) { try { return JSON.parse(saved.used_photo_ids) as number[] } catch { return [] } }
  return proposal.value?.used_photo_ids || []
})
const report = computed(() => parseReport(detail.value?.versions.find(v => v.id === version.value)?.report || detail.value?.runs[0]?.report))
const stageLabel = computed(() => ({ observing: '理解照片', model: '模型处理中', rendering: '浏览器排队 / 渲染', rendered: '截图已保存', reviewing: '视觉审查', reviewed: '审查完成', repairing: '修复中' }[stage.value] || '任务排队 / 处理中'))

function readableMessage(text: string) {
  try { const data = JSON.parse(text); return data.proposal ? data.proposal.structure + '\n风格：' + data.proposal.style : text } catch { return text }
}
function screenshotURL(id: string) { return '/api/v1/creations/' + workId.value + '/screenshots/' + id }
function failure(e: unknown) {
  const value = e as { response?: { data?: { error?: string } } }
  error.value = value.response?.data?.error || '操作失败，请稍后重试'
}
async function load() {
  if (!creationsEnabled.value) return
  error.value = ''
  try {
    if (!workId.value) { works.value = await listCreations(); return }
    const requestedId = workId.value
    const data = await getCreation(requestedId)
    if (requestedId !== workId.value) return
    detail.value = data
    proposal.value = data.state.proposal ? structuredClone(data.state.proposal) : null
    if (!version.value || !data.versions.some(v => v.id === version.value)) version.value = data.creation.current_version
    if (activeRun.value) subscribe(activeRun.value.id)
  } catch (e) { failure(e) }
}
function subscribe(run: string) {
  source?.close()
  source = new EventSource('/api/v1/creations/' + workId.value + '/runs/' + run + '/events')
  source.addEventListener('progress', (event) => { try { stage.value = JSON.parse((event as MessageEvent).data).stage } catch { /* Ignore a malformed progress event. */ } })
  source.addEventListener('done', async () => { source?.close(); source = null; version.value = ''; await load() })
}
async function run(action: string) {
  if (!detail.value || busy.value || activeRun.value) return
  busy.value = true; error.value = ''
  try {
    const answerText = detail.value.state.questions?.map((q, i) => q + '：' + (answers.value[i]?.trim() || '跳过')).join('\n') || ''
    await startCreationRun(workId.value, {
      action, message: action === 'skip' ? '跳过本轮问题' : [message.value, answerText].filter(Boolean).join('\n'),
      base_version: detail.value.creation.current_version,
      ...(action === 'confirm' && proposal.value ? { proposal: proposal.value } : {}),
    })
    message.value = ''; answers.value = []
    await load()
  } catch (e) { failure(e) } finally { busy.value = false }
}
async function cancel() {
  if (!activeRun.value) return
  try { await cancelCreationRun(workId.value, activeRun.value.id); source?.close(); source = null; await load() } catch (e) { failure(e) }
}
async function restore() {
  if (!detail.value) return
  busy.value = true
  try { await restoreCreationVersion(workId.value, version.value, detail.value.creation.current_version); await load() } catch (e) { failure(e) } finally { busy.value = false }
}
watch([workId, creationsEnabled], () => { source?.close(); source = null; detail.value = null; version.value = ''; void load() }, { immediate: true })
onUnmounted(() => source?.close())
</script>

<style scoped>
.creation-input { width: 100%; border: 1px solid var(--color-line, #444); border-radius: .5rem; padding: .5rem; background: transparent; }
.creation-preview { width: 100%; height: 75svh; min-height: 420px; border: 1px solid var(--color-line, #444); border-radius: .75rem; background: white; }
.creation-panels { display: grid; gap: 1.5rem; }
@media (min-width: 768px) { .creation-panels { grid-template-columns: minmax(280px, 1fr) minmax(0, 2fr); } }
.creation-panels.creation-mobile { grid-template-columns: minmax(0, 1fr); }
</style>
