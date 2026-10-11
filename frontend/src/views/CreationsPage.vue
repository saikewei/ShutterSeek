<template>
  <div class="creation-page" :class="{ 'creation-editor': !!workId }">
    <header class="flex items-center gap-3 flex-wrap">
      <RouterLink v-if="workId" to="/creations" class="btn-ghost px-2.5 py-1.5 text-xs" aria-label="返回作品列表">← 作品</RouterLink>
      <div class="flex-1 min-w-0">
        <h1 class="font-display text-xl truncate">{{ detail?.creation.title || 'AI 网页创作' }}</h1>
        <p v-if="detail" class="text-xs text-ink-3 mt-1">{{ detail.creation.current_version ? '继续对话，让作品更贴近你的想法' : '聊聊照片背后的故事，再一起确认方案' }}</p>
      </div>
      <template v-if="detail">
        <button class="btn-ghost px-3 py-2 text-xs" @click="photosOpen = true">照片 · {{ detail.photo_ids.length }}</button>
        <button v-if="detail.versions.length" class="btn-ghost px-3 py-2 text-xs" @click="versionsOpen = true">版本记录</button>
      </template>
    </header>
    <p v-if="error || runFailure" role="alert" class="rounded-xl border border-danger-ink/30 bg-raised p-3 text-sm text-danger-ink">{{ error || runFailure }}</p>
    <p v-if="!creationsEnabled" class="text-ink-2 text-sm">AI 创作尚未启用。</p>

    <template v-else-if="!workId">
      <section class="creation-intro rounded-2xl border border-line bg-raised p-6 sm:p-8">
        <p class="text-xs text-accent-strong mb-3">从照片开始，一起创作</p>
        <h2 class="font-display text-2xl sm:text-3xl text-ink">把照片，讲成故事。</h2>
        <p class="text-sm text-ink-2 leading-relaxed mt-3 max-w-lg">选一个相册，聊聊你想表达的故事。助手会一步步和你完善想法，制作可以浏览和互动的网页。</p>
        <div class="flex gap-2 flex-wrap mt-5 text-xs text-ink-3"><span>旅行游记</span><span aria-hidden="true">·</span><span>生活故事</span><span aria-hidden="true">·</span><span>照片画廊</span></div>
        <button class="btn-primary px-5 py-2.5 mt-6 text-sm" @click="albumPickerOpen = true">选择相册</button>
        <p class="text-xs text-ink-3 mt-3">每份作品可使用 1–200 张照片，补充的信息仅用于当前作品。</p>
      </section>
      <section v-if="works.length" class="space-y-3">
        <h2 class="text-sm font-medium text-ink-2">我的作品</h2>
        <div class="grid sm:grid-cols-2 xl:grid-cols-3 gap-3">
          <RouterLink v-for="work in works" :key="work.id" :to="'/creations/' + work.id" class="p-5 border border-line rounded-xl bg-raised hover:border-accent/50 transition-colors">
            <h3 class="font-medium text-sm truncate">{{ work.title }}</h3>
            <p class="text-xs text-ink-3 mt-2">{{ work.current_version ? '已保存 · 继续完善' : '草稿 · 继续创作' }}</p>
          </RouterLink>
        </div>
      </section>
    </template>

    <template v-else-if="detail">
      <CreationProgress v-if="activeRun" class="creation-live-progress" :progress="progress" :started-at="startedAt" :reconnecting="reconnecting" @cancel="cancel" />
      <p v-else-if="busy" role="status" class="text-sm text-ink-2">正在提交，请稍候…</p>
      <div v-if="isMobileShell" class="panel-switch flex gap-1 p-1 rounded-xl bg-raised border border-line" aria-label="创作面板">
        <button v-for="item in ['对话', '预览']" :key="item" class="flex-1 py-2 rounded-lg text-sm transition-colors" :class="panel === item ? 'bg-accent-soft text-ink font-medium' : 'text-ink-3'" :aria-pressed="panel === item" @click="panel = item">{{ item }}</button>
      </div>

      <div class="creation-workspace" :class="{ 'creation-mobile': isMobileShell }">
        <section v-show="!isMobileShell || panel === '对话'" class="creation-conversation min-w-0 rounded-2xl border border-line bg-raised" aria-label="创作对话">
          <div class="conversation-heading px-4 sm:px-5 py-4 border-b border-line">
            <h2 class="text-sm font-medium">一起创作</h2>
            <p class="text-xs text-ink-3 mt-1">说说想法，也可以随时调整内容和风格。</p>
          </div>
          <div ref="chatScroll" class="conversation-scroll space-y-5 p-4 sm:p-5" role="log" aria-label="聊天内容" tabindex="0" @scroll="rememberChatPosition">
            <div v-if="!detail.messages.length" class="chat-row">
              <span class="chat-avatar" aria-hidden="true">✦</span>
              <div class="chat-bubble chat-assistant">
                <p class="text-xs text-ink-3 mb-2">创作助手</p>
                <p class="text-sm leading-relaxed">想用这组照片讲一个什么样的故事？可以先告诉我一个简单的想法，我们慢慢完善。</p>
              </div>
            </div>
            <div v-for="item in detail.messages.filter(m => m.content.trim())" :key="item.id" class="chat-row" :class="{ 'chat-user-row': item.role === 'user' }">
              <span v-if="item.role !== 'user'" class="chat-avatar" aria-hidden="true">✦</span>
              <div class="chat-bubble" :class="item.role === 'user' ? 'chat-user' : 'chat-assistant'">
                <p class="text-xs text-ink-3 mb-2">{{ item.role === 'user' ? '你' : '创作助手' }}</p>
                <p class="whitespace-pre-wrap break-words text-sm leading-relaxed">{{ item.role === 'user' ? item.content : assistantText(item.content) }}</p>
              </div>
            </div>
            <div v-if="currentQuestion && !activeRun" class="text-xs text-ink-3">直接在下方回复即可，这个问题也可以跳过。</div>

            <div v-if="proposal && !activeRun && !detail.creation.current_version" class="proposal-card rounded-xl border border-accent/30 p-4 space-y-3">
              <div class="flex items-center justify-between gap-2">
                <h3 class="text-sm font-medium">当前方案</h3>
                <span class="text-xs text-ink-3">{{ proposal.used_photo_ids.length }} 张照片</span>
              </div>
              <p class="text-sm text-ink-2 leading-relaxed whitespace-pre-wrap">{{ displayChinese(proposal.structure, '请先更新中文方案，查看具体内容后再确认。') }}</p>
              <p class="text-xs text-ink-3">风格 · {{ displayChinese(proposal.style, '等待中文更新') }}</p>
              <div class="flex flex-wrap gap-2">
                <button v-if="proposalNeedsChinese" class="btn-primary px-4 py-2 text-sm" :disabled="busy" @click="localize">更新中文方案</button>
                <button v-else class="btn-primary px-4 py-2 text-sm" :disabled="busy || !canConfirm" @click="run('confirm')">确认并生成</button>
                <button v-if="!proposalNeedsChinese" class="btn-ghost px-3 py-2 text-xs" :disabled="busy" @click="editOpen = true">调整方案</button>
                <button class="btn-ghost px-3 py-2 text-xs" @click="photosOpen = true">调整选图</button>
              </div>
            </div>

          </div>
          <div class="conversation-footer">
            <form class="composer rounded-xl border border-line p-3" @submit.prevent="send">
              <label for="creation-message" class="sr-only">给创作助手的消息</label>
              <textarea id="creation-message" v-model="message" :rows="isMobileShell ? 2 : 3" class="w-full resize-none bg-transparent text-sm text-ink placeholder:text-ink-3 focus:outline-none" :placeholder="detail.creation.current_version ? '告诉我想修改哪里…' : currentQuestion ? '回复助手，或聊聊你想调整的地方…' : '说说你想做什么样的作品…'" :disabled="!!activeRun || busy" @keydown.enter="composerEnter" />
              <div class="flex items-center gap-2 mt-2">
                <span class="text-xs text-ink-3 flex-1">回车发送 · ⇧ + 回车换行</span>
                <button v-if="currentQuestion && !activeRun" type="button" class="btn-ghost px-3 py-2 text-xs" :disabled="busy" @click="run('skip')">先跳过</button>
                <button type="submit" class="btn-primary px-4 py-2 text-sm" :disabled="busy || !!activeRun || !message.trim()">发送</button>
              </div>
            </form>
          </div>
        </section>

        <section v-show="!isMobileShell || panel === '预览'" class="creation-preview-panel min-w-0 space-y-3" aria-label="作品预览">
          <div class="flex items-center gap-2 px-1">
            <h2 class="text-sm font-medium flex-1">作品预览</h2>
            <button v-if="report.rounds" class="btn-ghost px-3 py-1.5 text-xs" @click="reportOpen = true">效果检查</button>
            <span v-if="selectedVersion" class="text-xs" :class="selectedVersion.verified ? 'text-accent-strong' : 'text-ink-3'">{{ selectedVersion.verified ? '效果检查通过' : '待完善的候选版本' }}</span>
            <button v-if="version && version !== detail.creation.current_version" class="btn-ghost px-3 py-1.5 text-xs" :disabled="busy || !!activeRun" @click="restore">恢复此版本</button>
          </div>
          <iframe v-if="version" :key="version" :src="previewURL(workId, version)" sandbox="allow-scripts" referrerpolicy="no-referrer" title="作品沙箱预览" class="creation-preview" />
          <div v-else class="preview-empty rounded-2xl border border-dashed border-line bg-raised flex flex-col items-center justify-center text-center p-6">
            <span class="text-3xl text-accent-strong mb-5" aria-hidden="true">✦</span>
            <h3 class="font-display text-xl text-ink">{{ activeRun ? '你的作品正在成形' : '故事会在这里展开' }}</h3>
            <p class="text-sm text-ink-3 mt-3 max-w-xs leading-relaxed">{{ activeRun ? '当前阶段会在上方实时更新，完成后自动显示预览。' : '先聊聊想法，确认内容、风格和照片后，就可以生成作品。' }}</p>
          </div>
          <p v-if="version" class="text-xs text-ink-3 px-1">已自动保存。继续在对话中提出修改，会保留当前版本。</p>

        </section>
      </div>

      <AppModal :open="reportOpen" title="效果检查" @close="reportOpen = false">
        <p class="text-xs text-ink-3 mb-3">{{ report.rounds }} 轮检查 · {{ report.issues?.length || 0 }} 项提示</p>
        <div class="space-y-3">
              <p class="text-sm text-ink-2">{{ reportSummary(report) }}</p>
              <div v-for="(issue, index) in report.issues" :key="index" class="text-xs leading-relaxed border-t border-line pt-2">
                <p class="text-ink-3 mb-1">{{ issue.severity === 'blocking' ? '需要调整' : '提示' }} · {{ viewportText(issue.viewport) }} · {{ regionText(issue.region) }}</p>
                <p>{{ displayChinese(issue.description, '此区域有待检查的问题。') }}</p>
                <p class="text-ink-3 mt-1">{{ displayChinese(issue.suggestion, '请在对话中让助手继续调整这个区域。') }}</p>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <a v-for="shot in report.screenshots" :key="shot.id" :href="screenshotURL(shot.id)" target="_blank" rel="noopener noreferrer" class="text-xs text-ink-3">
                  <img :src="screenshotURL(shot.id)" :alt="viewportText(shot.viewport) + ' · ' + regionText(shot.kind)" loading="lazy" class="w-full rounded-lg border border-line mb-1" />
                  {{ viewportText(shot.viewport) }} · {{ regionText(shot.kind) }} · 页面位置 {{ shot.y ?? 0 }}
                </a>
              </div>
              <p class="text-[11px] text-ink-3">仅检查报告列出的区域和交互；模型仍可能遗漏问题。</p>
            </div>
      </AppModal>
      <AppModal :open="photosOpen" title="创作照片" @close="photosOpen = false">
        <div class="flex gap-2 mb-4">
          <button v-for="item in ['选图', '照片信息']" :key="item" class="btn-ghost px-3 py-2 text-xs" :aria-pressed="photoPanel === item" :class="{ 'bg-accent-soft': photoPanel === item }" @click="photoPanel = item">{{ item }}</button>
        </div>
        <template v-if="photoPanel === '选图'">
          <p class="text-sm text-ink-2 mb-3">使用 {{ usedIDs.length }} 张 · 未使用 {{ detail.photo_ids.length - usedIDs.length }} 张</p>
          <p v-if="!detail.creation.current_version" class="text-xs text-ink-3 mb-3">{{ proposal ? '勾选要放进作品的照片，确认后会保留你的选择。' : '先聊聊想法，助手整理方案后即可调整选图。' }}</p>
          <div class="grid grid-cols-3 sm:grid-cols-4 gap-3">
            <label v-for="id in detail.photo_ids" :key="id" class="relative text-xs">
              <img :src="THUMB_BASE + '/' + id + '.webp'" :alt="'照片 ' + id" loading="lazy" class="w-full aspect-square object-cover rounded-lg" :class="{ 'opacity-40': !usedIDs.includes(id) }" />
              <input v-if="proposal && !activeRun && !detail.creation.current_version" v-model="proposal.used_photo_ids" type="checkbox" :value="id" class="absolute top-2 left-2" :aria-label="'使用照片 ' + id" />
              <span class="block text-ink-3 mt-1">{{ usedIDs.includes(id) ? '已选择' : '未使用' }} · {{ id }}</span>
            </label>
          </div>
        </template>
        <template v-else>
          <div v-if="hasEnglish" class="rounded-lg border border-line p-3 mb-4">
            <p class="text-xs text-ink-3 mb-2">旧作品中的描述可以更新为中文。</p>
            <button class="btn-primary px-3 py-2 text-xs" :disabled="busy || !!activeRun" @click="localize">更新中文描述</button>
          </div>
          <h3 class="text-xs font-medium mb-3">图片观察 · 仅描述画面，可能不准确</h3>
          <p v-if="!detail.state.observations?.length" class="text-xs text-ink-3">助手尚未查看照片，发送想法后会逐批整理。</p>
          <div v-for="observation in detail.state.observations" :key="observation.photo_id" class="flex gap-3 mb-4">
            <img :src="THUMB_BASE + '/' + observation.photo_id + '.webp'" :alt="'照片 ' + observation.photo_id" loading="lazy" class="w-12 h-12 object-cover rounded-lg shrink-0" />
            <p class="text-xs text-ink-2 leading-relaxed">{{ displayChinese(observation.observation, '此描述需要更新为中文。') }}</p>
          </div>
          <h3 class="text-xs font-medium mt-5 mb-2">你提供的信息</h3>
          <p v-for="fact in detail.state.user_facts" :key="fact.message_id" class="text-xs text-ink-2 mb-2 break-words">{{ displayChinese(fact.display_text || fact.text, '此条信息保留了用户原文，可更新为中文查看。') }}</p>
          <h3 class="text-xs font-medium mt-5 mb-2">尚未确认的信息</h3>
          <p v-for="unknown in detail.state.unknowns" :key="unknown" class="text-xs text-ink-3 mb-1">{{ displayChinese(unknown, '尚未确认') }}</p>
        </template>
        <template #footer><button class="btn-primary w-full py-2 text-sm" @click="photosOpen = false">完成</button></template>
      </AppModal>

      <AppModal :open="editOpen" title="调整创作方案" @close="editOpen = false">
        <div v-if="proposal" class="space-y-4">
          <label class="block text-sm">内容结构<textarea v-model="proposal.structure" class="creation-input mt-2" rows="5" /></label>
          <label class="block text-sm">视觉风格<input v-model="proposal.style" class="creation-input mt-2" /></label>
          <p class="text-xs text-ink-3">也可以直接在聊天中描述想要的调整，让助手帮你完善。</p>
        </div>
        <template #footer><button class="btn-primary w-full py-2 text-sm" :disabled="!canConfirm" @click="editOpen = false">保存调整</button></template>
      </AppModal>

      <AppModal :open="versionsOpen" title="版本记录" @close="versionsOpen = false">
        <label class="block text-sm">查看版本
          <select v-model="version" class="creation-input mt-2">
            <option v-for="item in detail.versions" :key="item.id" :value="item.id">{{ versionDate(item.created_at) }} · {{ item.verified ? '检查通过' : '待完善' }}</option>
          </select>
        </label>
        <p class="text-xs text-ink-3 mt-3">每次生成和修改都会保存版本，你可以查看或恢复。</p>
        <template #footer>
          <button v-if="version !== detail.creation.current_version" class="btn-primary w-full py-2 text-sm" :disabled="busy || !!activeRun" @click="restore">恢复此版本</button>
          <button v-else class="btn-ghost w-full py-2 text-sm" @click="versionsOpen = false">返回预览</button>
        </template>
      </AppModal>
    </template>
    <CreationAlbumPicker :open="albumPickerOpen" @close="albumPickerOpen = false" @created="onAlbumCreated" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppModal from '@/components/AppModal.vue'
import CreationAlbumPicker from '@/components/CreationAlbumPicker.vue'
import CreationProgress from '@/components/CreationProgress.vue'
import { THUMB_BASE } from '@/api/client'
import { creationsEnabled } from '@/stores/creations'
import { isMobileShell } from '@/stores/device'
import { isScrollHostNearEnd, scrollHostToEnd } from '@/lib/scrollHost'
import { snapshotProposal, assistantText, displayChinese, needsChinese, reportSummary, viewportText, regionText, type CreationProgress as Progress } from '@/lib/creationChat'
import { listCreations, getCreation, startCreationRun, cancelCreationRun, restoreCreationVersion, previewURL, parseReport, type Creation, type WorkDetail, type Proposal } from '@/api/creations'

const route = useRoute()
const router = useRouter()
const workId = computed(() => String(route.params.id || ''))
const works = ref<Creation[]>([])
const detail = ref<WorkDetail | null>(null)
const proposal = ref<Proposal | null>(null)
const message = ref('')
const version = ref('')
const error = ref('')
const busy = ref(false)
const panel = ref('对话')
const albumPickerOpen = ref(false)
const photosOpen = ref(false)
const photoPanel = ref('选图')
const editOpen = ref(false)
const versionsOpen = ref(false)
const chatScroll = ref<HTMLElement>()
const followChat = ref(true)
const reportOpen = ref(false)
const progress = ref<Progress>({ stage: 'queued' })
const startedAt = ref(Date.now())
const reconnecting = ref(false)
let source: EventSource | null = null
let subscribedRun = ''
const activeRun = computed(() => detail.value?.runs.find(r => r.status === 'queued' || r.status === 'running'))
const currentQuestion = computed(() => detail.value?.creation.current_version ? '' : detail.value?.state.questions?.[0] || '')
const selectedVersion = computed(() => detail.value?.versions.find(v => v.id === version.value))
const usedIDs = computed(() => {
  if (selectedVersion.value) {
    try { return JSON.parse(selectedVersion.value.used_photo_ids) as number[] } catch { return [] }
  }
  return proposal.value?.used_photo_ids || detail.value?.photo_ids || []
})
const canConfirm = computed(() => !!proposal.value?.structure.trim() && !!proposal.value?.style.trim() && !!proposal.value?.used_photo_ids.length)
const proposalNeedsChinese = computed(() => !!proposal.value && (needsChinese(proposal.value.structure) || needsChinese(proposal.value.style)))
const hasEnglish = computed(() => proposalNeedsChinese.value || detail.value?.state.observations?.some(o => needsChinese(o.observation)) || detail.value?.state.unknowns?.some(needsChinese) || detail.value?.state.user_facts?.some(f => needsChinese(f.display_text || f.text)))
const report = computed(() => parseReport(selectedVersion.value?.report || detail.value?.runs[0]?.report))
const runFailure = computed(() => {
  if (busy.value || activeRun.value || detail.value?.runs[0]?.status !== 'failed') return ''
  return displayChinese(parseReport(detail.value.runs[0].report).error, '创作任务失败，请重试；已保存的方案和作品会保留。')
})
function versionDate(value: string) { return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) }
function screenshotURL(id: string) { return '/api/v1/creations/' + workId.value + '/screenshots/' + id }
function failure(e: unknown) {
  const value = e as { response?: { data?: { error?: string } } }
  error.value = displayChinese(value.response?.data?.error, '操作失败，请稍后重试')
}
function closeStream() { source?.close(); source = null; subscribedRun = ''; reconnecting.value = false }
async function load() {
  if (!creationsEnabled.value) return
  const requestedId = workId.value
  try {
    if (!requestedId) { works.value = await listCreations(); return }
    const data = await getCreation(requestedId)
    if (requestedId !== workId.value) return
    detail.value = data
    const latest = data.runs[0]
    const draft = !data.creation.current_version && latest?.action === 'confirm' && latest.proposal ? latest.proposal : data.state.proposal
    proposal.value = draft ? structuredClone(draft) : null
    if (!version.value || !data.versions.some(v => v.id === version.value)) version.value = data.creation.current_version
    if (activeRun.value) {
      const createdAt = Date.parse(activeRun.value.created_at || '')
      if (Number.isFinite(createdAt)) startedAt.value = createdAt
      subscribe(activeRun.value.id)
    } else closeStream()
    await revealComposer()
  } catch (e) { failure(e) }
}
function subscribe(runId: string) {
  if (source && subscribedRun === runId) return
  closeStream()
  subscribedRun = runId
  const stream = new EventSource('/api/v1/creations/' + workId.value + '/runs/' + runId + '/events')
  source = stream
  stream.addEventListener('progress', (event) => {
    if (source !== stream) return
    try {
      const next = JSON.parse((event as MessageEvent).data) as Progress
      progress.value = progress.value.stage === next.stage ? { ...progress.value, ...next } : next
      reconnecting.value = false
    } catch { /* Ignore a malformed progress event. */ }
  })
  stream.addEventListener('open', () => { if (source === stream) reconnecting.value = false })
  stream.addEventListener('error', () => { if (source === stream) reconnecting.value = true })
  stream.addEventListener('done', async (event) => {
    if (source !== stream) return
    const id = workId.value
    let status = ''
    try { status = JSON.parse((event as MessageEvent).data).status } catch { /* Reload persisted state when an event has no status. */ }
    closeStream()
    version.value = ''
    await load()
    if (id !== workId.value) return
    if (status === 'completed' && detail.value?.creation.current_version && isMobileShell.value) panel.value = '预览'
    else await revealComposer()
  })
}
function rememberChatPosition() {
  if (chatScroll.value?.clientHeight) followChat.value = isScrollHostNearEnd(chatScroll.value)
}
async function revealComposer() {
  const follow = followChat.value
  await nextTick()
  if (follow && chatScroll.value && panel.value === '对话') scrollHostToEnd(chatScroll.value)
}

async function run(action: string, text = '') {
  if (!detail.value || busy.value || activeRun.value) return
  busy.value = true; error.value = ''; followChat.value = true; progress.value = { stage: 'queued' }; startedAt.value = Date.now()
  const requestedId = workId.value
  try {
    await startCreationRun(requestedId, {
      action, message: action === 'skip' ? '这个问题先跳过，按已提供的信息继续。' : text,
      base_version: detail.value.creation.current_version,
      ...(action === 'confirm' && proposal.value ? { proposal: snapshotProposal(proposal.value) } : {}),
    })
    if (requestedId !== workId.value) return
    if (text) message.value = ''
    await load()
  } catch (e) { failure(e) } finally { busy.value = false }
}
async function send() {
  if (!message.value.trim()) return
  await run(detail.value?.creation.current_version ? 'modify' : 'answer', message.value.trim())
}
function composerEnter(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229 || event.shiftKey || event.ctrlKey || event.metaKey || event.altKey) return
  event.preventDefault()
  void send()
}
async function localize() {
  photosOpen.value = false
  await run('localize')
}
async function cancel() {
  if (!activeRun.value) return
  try { await cancelCreationRun(workId.value, activeRun.value.id); closeStream(); await load() } catch (e) { failure(e) }
}
async function restore() {
  if (!detail.value) return
  busy.value = true; error.value = ''
  try { await restoreCreationVersion(workId.value, version.value, detail.value.creation.current_version); versionsOpen.value = false; await load() } catch (e) { failure(e) } finally { busy.value = false }
}
function onAlbumCreated(id: string) { albumPickerOpen.value = false; void router.push('/creations/' + id) }
watch([workId, creationsEnabled], () => {
  closeStream(); detail.value = null; proposal.value = null; version.value = ''; message.value = ''; error.value = ''
  progress.value = { stage: 'queued' }; followChat.value = true; reportOpen.value = false; panel.value = '对话'; photosOpen.value = false; editOpen.value = false; versionsOpen.value = false
  void load()
}, { immediate: true })
watch(panel, () => { if (panel.value === '对话') void revealComposer() })
onUnmounted(closeStream)
</script>

<style scoped>
.creation-page { padding: 1rem; display: flex; flex-direction: column; gap: 1rem; max-width: 1600px; margin: 0 auto; }
.creation-editor { width: 100%; height: 100%; min-height: 0; overflow: hidden; }
.creation-editor > header, .creation-live-progress, .creation-editor > [role="alert"], .panel-switch { flex-shrink: 0; }
.creation-workspace { display: grid; gap: 1.25rem; align-items: stretch; }
.creation-editor .creation-workspace { flex: 1; min-height: 0; grid-template-rows: minmax(0, 1fr); overflow: hidden; }
.creation-conversation { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
.conversation-heading { flex-shrink: 0; }
.conversation-scroll { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; scrollbar-width: thin; }
/* Selection is scoped to transcript and proposal content, including on Safari. */
.conversation-scroll, .conversation-scroll * { user-select: text; -webkit-user-select: text; }
.conversation-footer { flex-shrink: 0; padding: .75rem 1rem; border-top: 1px solid var(--color-line); }
.creation-input { width: 100%; border: 1px solid var(--color-line); border-radius: .65rem; padding: .7rem; background: transparent; font-size: .875rem; }
.chat-row { display: flex; align-items: flex-start; gap: .65rem; }
.chat-user-row { justify-content: flex-end; }
.chat-avatar { width: 1.6rem; height: 1.6rem; border-radius: 50%; flex-shrink: 0; display: grid; place-items: center; background: var(--color-accent-soft); color: var(--color-accent-strong); font-size: .8rem; }
.chat-bubble { min-width: 0; max-width: 92%; }
.chat-assistant { flex: 1; }
.chat-user { padding: .85rem 1rem; border-radius: 1rem 1rem .25rem 1rem; background: var(--color-accent-soft); }
.composer:focus-within { border-color: var(--color-accent); }
.composer textarea { display: block; max-height: 20dvh; min-height: 2.5rem; }
.creation-preview-panel { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
.creation-preview-panel > :first-child, .creation-preview-panel > p { flex-shrink: 0; }
.creation-preview { flex: 1; width: 100%; height: 0; min-height: 0; border: 1px solid var(--color-line); border-radius: 1rem; background: white; }
.preview-empty { flex: 1; min-height: 0; }
.creation-mobile .conversation-heading { padding: .75rem 1rem; }
.creation-mobile .conversation-heading > p { display: none; }
@media (min-width: 768px) {
  .creation-page { padding: 1.5rem; gap: 1.25rem; }
  .creation-workspace { grid-template-columns: minmax(320px, 0.85fr) minmax(0, 1.6fr); }
}
.creation-workspace.creation-mobile { grid-template-columns: minmax(0, 1fr); }
</style>
