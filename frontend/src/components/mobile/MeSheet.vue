<template>
  <AppModal :open="open" title="账号" @close="close">
    <div class="space-y-5">
      <div class="flex items-center gap-3">
        <span
          class="w-10 h-10 shrink-0 rounded-full bg-accent-soft text-accent-strong flex items-center justify-center font-display text-base"
          aria-hidden="true"
        >{{ initial }}</span>
        <span class="min-w-0">
          <span class="block text-sm text-ink truncate">{{ username }}</span>
          <span class="block text-xs text-ink-3">{{ roleLabel }}</span>
        </span>
      </div>

      <button
        type="button"
        class="btn-danger-soft w-full py-2.5 text-sm"
        @click="doLogout"
      >退出登录</button>
    </div>
  </AppModal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import AppModal from '@/components/AppModal.vue'
import { authState, clearUser, isAdmin } from '@/stores/auth'
import { logout } from '@/api/auth'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const router = useRouter()

const username = computed(() => authState.user?.username ?? '未登录')
const roleLabel = computed(() => (isAdmin.value ? '管理员' : '访客'))
const initial = computed(() => username.value.trim().charAt(0).toUpperCase() || '?')

function close() {
  emit('close')
}

async function doLogout() {
  close()
  try {
    await logout()
  } catch {
    // Clearing local state is what matters; the cookie is short-lived anyway.
  }
  clearUser()
  router.push('/login')
}
</script>
