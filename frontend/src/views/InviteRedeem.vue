<template>
  <div class="min-h-screen flex items-center justify-center relative overflow-hidden bg-neutral-950">
    <!-- 背景氛围 -->
    <div class="absolute inset-0 bg-gradient-to-br from-neutral-950 via-neutral-900 to-black" />
    <div class="absolute -top-40 -left-40 w-[28rem] h-[28rem] rounded-full bg-sky-500/10 blur-3xl" />
    <div class="absolute -bottom-40 -right-40 w-[28rem] h-[28rem] rounded-full bg-amber-500/10 blur-3xl" />

    <!-- 邀请注册卡片 -->
    <div v-if="!checking" class="relative w-full max-w-sm px-6">
      <div class="bg-neutral-900/60 backdrop-blur-xl border border-neutral-800 rounded-2xl p-8 shadow-2xl shadow-black/50">
        <div class="flex flex-col items-center mb-8">
          <div class="w-14 h-14 rounded-2xl bg-gradient-to-br from-sky-400 to-sky-600 flex items-center justify-center shadow-lg shadow-sky-500/25">
            <svg class="w-7 h-7 text-neutral-950" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 12a2.25 2.25 0 0 0-2.25-2.25H15a3 3 0 1 1-6 0H5.25A2.25 2.25 0 0 0 3 12m18 0v6a2.25 2.25 0 0 1-2.25 2.25H5.25A2.25 2.25 0 0 1 3 18v-6m18 0V9M3 12V9m18 0a2.25 2.25 0 0 0-2.25-2.25H5.25A2.25 2.25 0 0 0 3 9m18 0V6a2.25 2.25 0 0 0-2.25-2.25H5.25A2.25 2.25 0 0 0 3 6v3" />
            </svg>
          </div>
          <h1 class="text-xl font-semibold text-white mt-4 tracking-tight">创建访客账号</h1>
          <p class="text-sm text-neutral-500 mt-1">通过邀请码加入</p>
        </div>

        <form @submit.prevent="doRedeem" class="space-y-4">
          <div class="relative">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-500" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />
            </svg>
            <input
              v-model="username"
              class="w-full pl-10 pr-3 py-2.5 text-sm rounded-lg bg-neutral-800/70 text-white border border-neutral-700/80 outline-none placeholder:text-neutral-500 focus:border-sky-500/50 focus:ring-1 focus:ring-sky-500/20 transition-all"
              placeholder="用户名"
              autocomplete="username"
            />
          </div>

          <div class="relative">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-500" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
            </svg>
            <input
              v-model="password"
              type="password"
              class="w-full pl-10 pr-3 py-2.5 text-sm rounded-lg bg-neutral-800/70 text-white border border-neutral-700/80 outline-none placeholder:text-neutral-500 focus:border-sky-500/50 focus:ring-1 focus:ring-sky-500/20 transition-all"
              placeholder="密码（至少6位）"
              autocomplete="new-password"
            />
          </div>

          <div class="relative">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-500" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
            </svg>
            <input
              v-model="confirmPassword"
              type="password"
              class="w-full pl-10 pr-3 py-2.5 text-sm rounded-lg bg-neutral-800/70 text-white border border-neutral-700/80 outline-none placeholder:text-neutral-500 focus:border-sky-500/50 focus:ring-1 focus:ring-sky-500/20 transition-all"
              placeholder="确认密码"
              autocomplete="new-password"
              @keyup.enter="doRedeem"
            />
          </div>

          <p v-if="error" class="text-xs text-red-400">{{ error }}</p>

          <button
            type="submit"
            :disabled="loading"
            class="w-full py-2.5 text-sm rounded-lg bg-gradient-to-r from-sky-500 to-sky-600 text-neutral-950 font-semibold hover:from-sky-400 hover:to-sky-500 transition-all shadow-lg shadow-sky-500/20 disabled:opacity-50 disabled:cursor-not-allowed"
          >{{ loading ? '创建中...' : '创建账号' }}</button>
        </form>
      </div>

      <p class="text-center text-xs text-neutral-600 mt-6">Invited by the archive owner · 由相册主人邀请</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { redeemInvite, validateInvite } from '@/api/auth'
import { setUser } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const code = route.params.code as string
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)
const checking = ref(true)

// Validate the invite code on entry — redirect to login if expired/invalid
onMounted(async () => {
  try {
    const res = await validateInvite(code)
    if (!res.valid) {
      router.replace('/login')
      return
    }
  } catch {
    router.replace('/login')
    return
  } finally {
    checking.value = false
  }
})

async function doRedeem() {
  error.value = ''
  if (password.value.length < 6) {
    error.value = '密码至少需要6个字符'
    return
  }
  if (password.value !== confirmPassword.value) {
    error.value = '两次输入的密码不一致'
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
