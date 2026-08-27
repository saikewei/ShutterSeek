<template>
  <div class="min-h-screen flex items-center justify-center relative overflow-hidden bg-base">
    <!-- 背景氛围 -->
    <div class="absolute inset-0 bg-gradient-to-br from-base via-raised to-black" />
    <div class="absolute -top-40 -left-40 w-[28rem] h-[28rem] rounded-full bg-accent/10 blur-3xl" />
    <div class="absolute -bottom-40 -right-40 w-[28rem] h-[28rem] rounded-full bg-accent/5 blur-3xl" />

    <!-- 登录卡片 -->
    <div class="relative w-full max-w-sm px-6">
      <div class="bg-raised/60 backdrop-blur-xl border border-line rounded-2xl p-8 shadow-2xl shadow-black/50">
        <!-- 品牌 -->
        <div class="flex flex-col items-center mb-8">
          <div class="w-14 h-14 rounded-2xl bg-gradient-to-br from-accent-strong to-accent flex items-center justify-center shadow-lg shadow-accent/25">
            <svg class="w-7 h-7 text-[#1C1208]" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6.827 6.175A2.31 2.31 0 0 1 5.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 0 0-1.134-.175 2.31 2.31 0 0 1-1.64-1.055l-.822-1.316a2.192 2.192 0 0 0-1.736-1.039 48.774 48.774 0 0 0-5.232 0 2.192 2.192 0 0 0-1.736 1.039l-.821 1.316Z" />
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 12.75a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Z" />
            </svg>
          </div>
          <h1 class="text-xl font-semibold text-ink mt-4 tracking-tight">ShutterSeek</h1>
          <p class="text-sm text-ink-3 mt-1">登录以浏览你的照片</p>
        </div>

        <form @submit.prevent="doLogin" class="space-y-4">
          <!-- 用户名 -->
          <div class="relative">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-3" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />
            </svg>
            <input
              v-model="username"
              class="w-full pl-10 pr-3 py-2.5 text-sm rounded-lg bg-surface/70 text-ink border border-line-strong outline-none placeholder:text-ink-3 focus:border-accent/60 focus:ring-1 focus:ring-accent/20 transition-all"
              placeholder="用户名"
              autocomplete="username"
            />
          </div>

          <!-- 密码 -->
          <div class="relative">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-3" fill="none" viewBox="0 0 24 24" stroke-width="1.8" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
            </svg>
            <input
              v-model="password"
              type="password"
              class="w-full pl-10 pr-3 py-2.5 text-sm rounded-lg bg-surface/70 text-ink border border-line-strong outline-none placeholder:text-ink-3 focus:border-accent/60 focus:ring-1 focus:ring-accent/20 transition-all"
              placeholder="密码"
              autocomplete="current-password"
            />
          </div>

          <p v-if="error" class="text-xs text-danger-ink">{{ error }}</p>

          <button
            type="submit"
            :disabled="loading"
            class="w-full py-2.5 text-sm rounded-lg bg-gradient-to-r from-accent-strong to-accent text-[#1C1208] font-semibold hover:from-[#E5B397] hover:to-accent-strong transition-all shadow-lg shadow-accent/20 disabled:opacity-50 disabled:cursor-not-allowed"
          >{{ loading ? '登录中...' : '登 录' }}</button>
        </form>
      </div>

      <p class="text-center text-xs text-ink-3 mt-6">Private photo archive · 私密照片档案</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login } from '@/api/auth'
import { setUser } from '@/stores/auth'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function doLogin() {
  error.value = ''
  loading.value = true
  try {
    const user = await login(username.value, password.value)
    setUser(user)
    router.push('/')
  } catch (e: any) {
    error.value = e?.response?.data?.error || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>
