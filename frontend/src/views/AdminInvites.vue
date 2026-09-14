<template>
  <div class="p-3 sm:p-4 max-w-3xl mx-auto">
    <div class="flex items-center justify-between gap-3 mb-3">
      <p class="text-xs text-ink-3">未使用 {{ pendingCount }} / 共 {{ invites.length }}</p>
      <button
        @click="doCreate"
        :disabled="creating"
        class="btn-primary px-3.5 py-2 text-xs"
      >{{ creating ? '生成中...' : '+ 生成邀请码' }}</button>
    </div>

    <div v-if="newCode" class="mb-4 p-3.5 bg-accent-soft border border-accent/40 rounded-xl">
      <p class="text-xs text-accent-strong mb-1.5">新邀请码已生成，把链接发给对方即可：</p>
      <p class="text-sm text-ink font-mono break-all leading-relaxed">{{ inviteLink }}</p>
      <div class="mt-3 flex gap-2">
        <button @click="copyLink" class="btn-ghost px-3.5 py-2 text-xs">{{ copied ? '已复制' : '复制链接' }}</button>
        <button v-if="canShare" @click="shareLink" class="btn-ghost px-3.5 py-2 text-xs">分享…</button>
        <button @click="newCode = null" class="ml-auto px-3 py-2 text-xs text-ink-3 hover:text-ink">收起</button>
      </div>
    </div>

    <div v-if="loading" class="text-center text-ink-3 py-12 text-sm">加载中...</div>
    <div v-else class="space-y-2">
      <div
        v-for="inv in invites"
        :key="inv.id"
        class="flex items-center gap-3 px-3.5 py-3 bg-surface rounded-xl border border-line"
      >
        <div class="flex-1 min-w-0">
          <p class="text-sm text-ink font-mono truncate">{{ inv.code }}</p>
          <p class="text-[11px] text-ink-3 mt-0.5">
            创建于 {{ formatDate(inv.created_at) }}
            <template v-if="inv.used_by"> · 已使用</template>
            <template v-else-if="isExpired(inv)"> · 已过期</template>
            <template v-else> · 有效期至 {{ formatDate(inv.expires_at) }}</template>
          </p>
        </div>
        <button
          v-if="!inv.used_by"
          @click="confirmTarget = inv"
          class="shrink-0 px-3.5 py-2 text-xs rounded-lg text-danger-ink hover:bg-danger-soft transition-colors"
        >注销</button>
      </div>
      <p v-if="invites.length === 0" class="text-sm text-ink-3 text-center py-8">暂无邀请码</p>
    </div>

    <AppModal :open="confirmTarget !== null" title="注销邀请码" @close="confirmTarget = null">
      <p class="text-xs text-ink-2 mb-4">
        注销后该链接立即失效，确定要注销 <span class="font-mono text-ink">{{ confirmTarget?.code }}</span> 吗？
      </p>
      <div class="flex justify-end gap-2">
        <button @click="confirmTarget = null" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink">取消</button>
        <button @click="doDelete" class="btn-danger px-4 py-1.5 text-xs">注销</button>
      </div>
    </AppModal>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { listInvites, createInvite, deleteInvite, type InviteCode } from '@/api/auth'
import AppModal from '@/components/AppModal.vue'

const invites = ref<InviteCode[]>([])
const loading = ref(true)
const creating = ref(false)
const newCode = ref<InviteCode | null>(null)
const copied = ref(false)
const confirmTarget = ref<InviteCode | null>(null)

const canShare = typeof navigator !== 'undefined' && typeof navigator.share === 'function'

const pendingCount = computed(
  () => invites.value.filter((i) => !i.used_by && !isExpired(i)).length,
)

const inviteLink = computed(() => {
  if (!newCode.value) return ''
  return `${window.location.origin}${window.location.pathname}#/invite/${newCode.value.code}`
})

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString()
}

function isExpired(inv: InviteCode): boolean {
  return new Date(inv.expires_at) < new Date()
}

onMounted(async () => {
  try { invites.value = (await listInvites()).items } catch { /* */ }
  finally { loading.value = false }
})

async function doCreate() {
  creating.value = true
  try {
    newCode.value = await createInvite()
    invites.value.unshift(newCode.value)
    copied.value = false
  } finally {
    creating.value = false
  }
}

async function doDelete() {
  const target = confirmTarget.value
  confirmTarget.value = null
  if (!target) return
  try {
    await deleteInvite(target.id)
    invites.value = invites.value.filter(i => i.id !== target.id)
  } catch { /* keep the row so the user can retry */ }
}

// Share sheet first on mobile; falls back to the clipboard, and to a hidden
// textarea when the origin is not a secure context.
async function shareLink() {
  if (canShare) {
    try {
      await navigator.share({ url: inviteLink.value })
      return
    } catch {
      // User dismissed the sheet, or sharing is unavailable: fall through.
    }
  }
  await copyLink()
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(inviteLink.value)
    copied.value = true
  } catch {
    const el = document.createElement('textarea')
    el.value = inviteLink.value
    document.body.appendChild(el)
    el.select()
    document.execCommand('copy')
    document.body.removeChild(el)
    copied.value = true
  }
}
</script>
