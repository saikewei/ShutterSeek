<template>
  <div class="min-h-screen flex items-center justify-center bg-neutral-950">
    <div class="w-80 bg-neutral-900 rounded-xl p-6 border border-neutral-800">
      <h1 class="text-lg font-semibold text-white mb-6 text-center">ShutterSeek</h1>
      <form @submit.prevent="doLogin" class="space-y-4">
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
          placeholder="密码"
          autocomplete="current-password"
        />
        <p v-if="error" class="text-xs text-red-400">{{ error }}</p>
        <button
          type="submit"
          :disabled="loading"
          class="w-full py-2 text-sm rounded-lg bg-white text-black font-medium hover:bg-neutral-200 transition-colors disabled:opacity-50"
        >{{ loading ? '登录中...' : '登录' }}</button>
      </form>
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
