<template>
  <div class="p-4">
    <div class="flex items-center justify-between mb-4">
      <h1 class="text-base font-medium text-white">邀请码管理</h1>
      <button
        @click="doCreate"
        :disabled="creating"
        class="px-3 py-1.5 text-xs rounded-full bg-white text-black font-medium hover:bg-neutral-200 transition-colors disabled:opacity-50"
      >{{ creating ? '生成中...' : '+ 生成邀请码' }}</button>
    </div>

    <div v-if="newCode" class="mb-4 p-3 bg-amber-500/10 border border-amber-500/40 rounded-lg">
      <p class="text-xs text-amber-300 mb-1">新邀请码已生成（复制以下链接发送给访客）：</p>
      <p class="text-sm text-amber-100 font-mono break-all">{{ inviteLink }}</p>
      <button @click="copyLink" class="mt-2 text-xs text-amber-300 hover:text-amber-200">📋 {{ copied ? '已复制' : '复制链接' }}</button>
    </div>

    <div v-if="loading" class="text-center text-neutral-500 py-12 text-sm">Loading...</div>
    <div v-else class="space-y-2">
      <div v-for="inv in invites" :key="inv.id" class="flex items-center justify-between px-3 py-2 bg-neutral-800 rounded-lg border border-neutral-700">
        <div class="flex-1 min-w-0">
          <p class="text-sm text-white font-mono truncate">{{ inv.code }}</p>
          <p class="text-xs text-neutral-500">
            创建于 {{ formatDate(inv.created_at) }}
            <template v-if="inv.used_by"> · 已使用</template>
            <template v-else-if="isExpired(inv)"> · 已过期</template>
            <template v-else> · 有效期至 {{ formatDate(inv.expires_at) }}</template>
          </p>
        </div>
        <button v-if="!inv.used_by" @click="doDelete(inv.id)" class="px-2 py-1 text-xs text-red-400 hover:text-red-300">注销</button>
      </div>
      <p v-if="invites.length === 0" class="text-sm text-neutral-500 text-center py-8">暂无邀请码</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listInvites, createInvite, deleteInvite, type InviteCode } from '@/api/auth'

const invites = ref<InviteCode[]>([])
const loading = ref(true)
const creating = ref(false)
const newCode = ref<InviteCode | null>(null)
const copied = ref(false)

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

async function doDelete(id: number) {
  await deleteInvite(id)
  invites.value = invites.value.filter(i => i.id !== id)
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
