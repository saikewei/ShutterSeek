<template>
  <!-- Mobile guest shell: top bar + bottom tabs, no sidebar -->
  <div v-if="isGuestMobile" class="flex flex-col h-screen supports-[height:100dvh]:h-dvh bg-base text-ink">
    <header class="shrink-0 px-4 py-3 border-b border-line bg-raised flex items-center justify-between">
      <h1 class="font-display text-sm font-semibold tracking-wide text-ink">ShutterSeek</h1>
      <button @click="doLogout" class="text-xs text-ink-3 hover:text-ink-2 transition-colors duration-150">退出</button>
    </header>

    <main class="flex-1 overflow-auto overflow-x-hidden">
      <router-view />
    </main>

    <nav class="shrink-0 flex border-t border-line bg-raised pb-[env(safe-area-inset-bottom)]">
      <router-link
        to="/"
        class="flex-1 py-3 text-xs text-center transition-colors duration-150"
        :class="$route.path === '/' ? 'text-accent-strong font-semibold' : 'text-ink-3'"
      >全部照片</router-link>
      <router-link
        to="/albums"
        class="flex-1 py-3 text-xs text-center transition-colors duration-150"
        :class="$route.path.startsWith('/albums') ? 'text-accent-strong font-semibold' : 'text-ink-3'"
      >相册</router-link>
      <router-link
        to="/search"
        class="flex-1 py-3 text-xs text-center transition-colors duration-150"
        :class="$route.path === '/search' ? 'text-accent-strong font-semibold' : 'text-ink-3'"
      >搜索</router-link>
    </nav>
  </div>

  <!-- Desktop shell -->
  <div v-else class="flex h-screen supports-[height:100dvh]:h-dvh bg-base text-ink">
    <!-- Sidebar -->
    <nav v-if="!hideSidebar" class="w-48 shrink-0 bg-raised border-r border-line flex flex-col">
      <div class="px-4 pt-5 pb-4">
        <h1 class="font-display text-base font-medium tracking-wide text-ink">ShutterSeek</h1>
        <p class="text-[9px] tracking-[0.2em] uppercase text-ink-3 mt-1">Private Archive</p>
      </div>
      <div class="flex-1 py-2">
        <router-link
          to="/"
          class="relative block px-4 py-2 text-sm transition-colors duration-150 rounded-r-md"
          :class="$route.path === '/' ? 'text-ink font-semibold bg-accent-soft' : 'text-ink-2 hover:text-ink hover:bg-white/5'"
        >
          <span v-if="$route.path === '/'" class="absolute left-0 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent"></span>
          全部照片
        </router-link>
        <router-link
          to="/albums"
          class="relative block px-4 py-2 text-sm transition-colors duration-150 rounded-r-md"
          :class="$route.path.startsWith('/albums') ? 'text-ink font-semibold bg-accent-soft' : 'text-ink-2 hover:text-ink hover:bg-white/5'"
        >
          <span v-if="$route.path.startsWith('/albums')" class="absolute left-0 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent"></span>
          相册
        </router-link>
        <router-link
          to="/search"
          class="relative block px-4 py-2 text-sm transition-colors duration-150 rounded-r-md"
          :class="$route.path === '/search' ? 'text-ink font-semibold bg-accent-soft' : 'text-ink-2 hover:text-ink hover:bg-white/5'"
        >
          <span v-if="$route.path === '/search'" class="absolute left-0 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent"></span>
          搜索
        </router-link>
        <template v-if="isAdmin">
          <div class="border-t border-line my-2 mx-4"></div>
          <router-link
            to="/admin/invites"
            class="relative block px-4 py-2 text-sm transition-colors duration-150 rounded-r-md"
            :class="$route.path === '/admin/invites' ? 'text-ink font-semibold bg-accent-soft' : 'text-ink-2 hover:text-ink hover:bg-white/5'"
          >
            <span v-if="$route.path === '/admin/invites'" class="absolute left-0 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent"></span>
            邀请
          </router-link>
          <router-link
            to="/admin/logs"
            class="relative block px-4 py-2 text-sm transition-colors duration-150 rounded-r-md"
            :class="$route.path === '/admin/logs' ? 'text-ink font-semibold bg-accent-soft' : 'text-ink-2 hover:text-ink hover:bg-white/5'"
          >
            <span v-if="$route.path === '/admin/logs'" class="absolute left-0 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent"></span>
            日志
          </router-link>
        </template>
      </div>
      <div class="px-4 py-3 border-t border-line flex items-center justify-between">
        <span class="text-xs text-ink-3">{{ authState.user?.username }}</span>
        <button @click="doLogout" class="text-xs text-ink-3 hover:text-ink-2 transition-colors duration-150">退出</button>
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
