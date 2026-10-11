<template>
  <AppModal :open="open" title="分享作品" @close="close">
    <p class="text-sm text-ink-2 leading-relaxed">分享正在预览的版本。接收者须登录访客或管理员账号后查看，访客还需有权访问作品中的照片。</p>
    <p class="text-xs text-ink-3 mt-2">链接仅展示作品；对话、编辑和版本管理仍由你使用。生成新版本后，可在这里更新分享。</p>
    <p v-if="loading" role="status" class="text-sm text-ink-3 mt-4">正在读取分享设置…</p>
    <p v-if="error" role="alert" class="text-sm text-danger-ink mt-3">{{ error }}</p>
    <template v-if="share?.enabled">
      <label class="block text-xs text-ink-2 mt-4">分享链接
        <input ref="linkInput" :value="link" readonly class="input mt-2" @click="linkInput?.select()" />
      </label>
      <div class="flex gap-2 mt-3 flex-wrap">
        <button class="btn-primary px-4 py-2 text-sm" :disabled="busy" @click="copy">{{ copied ? '已复制' : '复制链接' }}</button>
        <button class="btn-ghost px-4 py-2 text-sm" :disabled="busy" @click="revoke">关闭分享</button>
        <button v-if="share.version_id !== version" class="btn-ghost px-4 py-2 text-sm" :disabled="busy" @click="enable">更新为预览版本</button>
      </div>
      <p class="text-xs text-ink-3 mt-3">{{ share.version_id === version ? '当前预览版本已分享。' : '链接仍展示先前选择的版本。' }}关闭分享或还原作品后，链接将失效。</p>
    </template>
    <button v-else-if="!loading" class="btn-primary px-4 py-2 text-sm mt-4" :disabled="busy" @click="enable">{{ busy ? '开启中…' : '开启登录后分享' }}</button>
  </AppModal>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AppModal from '@/components/AppModal.vue'
import { getCreationShare, enableCreationShare, revokeCreationShare, type CreationShare } from '@/api/creations'
import { creationShareURL } from '@/lib/creationShare'
import { displayChinese } from '@/lib/creationChat'
const props = defineProps<{ open: boolean; workId: string; version: string; baseVersion: string }>()
const emit = defineEmits<{ close: [] }>()
const share = ref<CreationShare | null>(null), loading = ref(false), busy = ref(false), error = ref(''), copied = ref(false)
const linkInput = ref<HTMLInputElement>()
const link = computed(() => share.value ? creationShareURL(share.value.id) : '')
let loadId = 0
function close() { if (!busy.value) emit('close') }
function failure(e: unknown) {
  const value = e as { response?: { data?: { error?: string } } }
  error.value = displayChinese(value.response?.data?.error, '分享操作失败，请重试。')
}
watch(() => props.open, async open => {
  const current = ++loadId
  if (!open) return
  share.value = null; error.value = ''; copied.value = false; loading.value = true
  try { const data = await getCreationShare(props.workId); if (current === loadId) share.value = data }
  catch (e) { if (current === loadId) failure(e) }
  finally { if (current === loadId) loading.value = false }
})
async function enable() {
  if (busy.value) return
  busy.value = true; error.value = ''; copied.value = false
  try { share.value = await enableCreationShare(props.workId, props.version, props.baseVersion) }
  catch (e) { failure(e) } finally { busy.value = false }
}
async function revoke() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { await revokeCreationShare(props.workId); if (share.value) share.value.enabled = false }
  catch (e) { failure(e) } finally { busy.value = false }
}
async function copy() {
  copied.value = false
  try { await navigator.clipboard.writeText(link.value); copied.value = true }
  catch {
    linkInput.value?.focus(); linkInput.value?.select()
    copied.value = document.execCommand('copy')
    if (!copied.value) error.value = '自动复制失败，请选中链接手动复制。'
  }
}
</script>
