<template>
  <div class="flex h-screen bg-neutral-950 text-white">
    <!-- Sidebar -->
    <nav class="w-48 shrink-0 bg-neutral-900 border-r border-neutral-800 flex flex-col">
      <div class="px-4 py-4 border-b border-neutral-800">
        <h1 class="text-sm font-semibold tracking-wide text-white">ShutterSeek</h1>
      </div>
      <div class="flex-1 py-2">
        <router-link
          to="/"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path === '/' ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >全部照片</router-link>
        <router-link
          to="/albums"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path.startsWith('/albums') ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >相册</router-link>
        <router-link
          v-if="isAdmin"
          to="/admin/invites"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path === '/admin/invites' ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >邀请</router-link>
      </div>
      <div class="px-4 py-3 border-t border-neutral-800 flex items-center justify-between">
        <span class="text-xs text-neutral-500">{{ authState.user?.username }}</span>
        <button @click="doLogout" class="text-xs text-neutral-400 hover:text-neutral-200 transition-colors">退出</button>
      </div>
    </nav>

    <!-- Main content -->
    <main class="flex-1 overflow-auto">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { authState, isAdmin, clearUser } from '@/stores/auth'
import { logout } from '@/api/auth'

const router = useRouter()

async function doLogout() {
  try { await logout() } catch { /* ignore */ }
  clearUser()
  router.push('/login')
}
</script>
