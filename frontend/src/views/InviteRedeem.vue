<template>
  <div class="min-h-screen flex items-center justify-center bg-neutral-950">
    <div class="w-80 bg-neutral-900 rounded-xl p-6 border border-neutral-800">
      <h1 class="text-lg font-semibold text-white mb-2 text-center">创建访客账号</h1>
      <p class="text-xs text-neutral-500 mb-6 text-center">使用邀请码注册</p>
      <form @submit.prevent="doRedeem" class="space-y-4">
        <input
          v-model="username"
          class="w-full px-3 py-2 text-sm rounded-lg bg-neutral-800 text-white border border-neutral-700 outline-none focus:border-neutral-500"
          placeholder="用户名"
          autocomplete="username"
        />
        <input
          v-model="password"
          type="password"
          class="w-full px-3 py-2 text-sm rounded-lg bg-neutral-800 text-white border border-neutral-700 outline-none focus:border-neutral-500"
          placeholder="密码（至少6位）"
          autocomplete="new-password"
        />
        <p v-if="error" class="text-xs text-red-400">{{ error }}</p>
        <button
          type="submit"
          :disabled="loading"
          class="w-full py-2 text-sm rounded-lg bg-white text-black font-medium hover:bg-neutral-200 transition-colors disabled:opacity-50"
        >{{ loading ? '创建中...' : '创建账号' }}</button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { redeemInvite } from '@/api/auth'
import { setUser } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const code = route.params.code as string
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function doRedeem() {
  error.value = ''
  if (password.value.length < 6) {
    error.value = '密码至少需要6个字符'
    return
  }
  loading.value = true
  try {
    const user = await redeemInvite(code, username.value, password.value)
    setUser(user)
    router.push('/')
  } catch (e: any) {
    error.value = e?.response?.data?.error || '创建失败'
  } finally {
    loading.value = false
  }
}
</script>
