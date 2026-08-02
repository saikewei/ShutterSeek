<template>
  <!-- Mobile guest shell: top bar + bottom tabs, no sidebar -->
  <div v-if="isGuestMobile" class="flex flex-col h-screen supports-[height:100dvh]:h-dvh bg-neutral-950 text-white">
    <header class="shrink-0 px-4 py-3 border-b border-neutral-800 bg-neutral-900 flex items-center justify-between">
      <h1 class="text-sm font-semibold tracking-wide text-white">ShutterSeek</h1>
      <button @click="doLogout" class="text-xs text-neutral-400 hover:text-neutral-200 transition-colors">退出</button>
    </header>

    <main class="flex-1 overflow-auto overflow-x-hidden">
      <router-view />
    </main>

    <nav class="shrink-0 flex border-t border-neutral-800 bg-neutral-900 pb-[env(safe-area-inset-bottom)]">
      <router-link
        to="/"
        class="flex-1 py-3 text-xs text-center transition-colors"
        :class="$route.path === '/' ? 'text-white bg-neutral-800' : 'text-neutral-400'"
      >全部照片</router-link>
      <router-link
        to="/albums"
        class="flex-1 py-3 text-xs text-center transition-colors"
        :class="$route.path.startsWith('/albums') ? 'text-white bg-neutral-800' : 'text-neutral-400'"
      >相册</router-link>
      <router-link
        to="/search"
        class="flex-1 py-3 text-xs text-center transition-colors"
        :class="$route.path === '/search' ? 'text-white bg-neutral-800' : 'text-neutral-400'"
      >搜索</router-link>
    </nav>
  </div>

  <!-- Desktop shell -->
  <div v-else class="flex h-screen supports-[height:100dvh]:h-dvh bg-neutral-950 text-white">
    <!-- Sidebar -->
    <nav v-if="!hideSidebar" class="w-48 shrink-0 bg-neutral-900 border-r border-neutral-800 flex flex-col">
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
          to="/search"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path === '/search' ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >搜索</router-link>
        <router-link
          v-if="isAdmin"
          to="/admin/invites"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path === '/admin/invites' ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >邀请</router-link>
        <router-link
          v-if="isAdmin"
          to="/admin/logs"
          class="block px-4 py-2 text-sm transition-colors"
          :class="$route.path === '/admin/logs' ? 'text-white bg-neutral-800' : 'text-neutral-400 hover:text-white hover:bg-neutral-800/50'"
        >日志</router-link>
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
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { authState, isAdmin, clearUser } from '@/stores/auth'
import { isGuestMobile } from '@/stores/device'
import { logout } from '@/api/auth'

const route = useRoute()
const router = useRouter()
const hideSidebar = computed(() => route.meta.hideSidebar)

async function doLogout() {
  try { await logout() } catch { /* ignore */ }
  clearUser()
  router.push('/login')
}
</script>
