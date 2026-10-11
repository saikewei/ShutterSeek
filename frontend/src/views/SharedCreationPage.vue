<template>
  <div class="shared-preview bg-canvas text-ink">
    <header class="flex items-center gap-3 border-b border-line px-4 py-3 bg-raised">
      <RouterLink to="/" class="btn-ghost shrink-0 px-3 py-2 text-xs">← 返回照片</RouterLink>
      <h1 class="text-sm font-medium flex-1 min-w-0 truncate">{{ detail?.title || '分享的作品' }}</h1>
      <span v-if="detail" class="text-xs text-ink-3 shrink-0">只读查看</span>
    </header>
    <p v-if="error" role="alert" class="p-5 text-sm text-danger-ink">{{ error }}</p>
    <iframe v-else-if="detail" :src="sharedPreviewURL(shareId)" sandbox="allow-scripts" referrerpolicy="no-referrer" title="分享作品预览" />
    <p v-else role="status" class="p-5 text-sm text-ink-3">正在加载分享作品…</p>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getSharedCreation, sharedPreviewURL, type SharedCreation } from '@/api/creations'
import { clearUser } from '@/stores/auth'
import { displayChinese } from '@/lib/creationChat'
const route = useRoute(), router = useRouter()
const shareId = computed(() => String(route.params.share))
const detail = ref<SharedCreation | null>(null), error = ref('')
watch(shareId, async id => {
  detail.value = null; error.value = ''
  try {
    const data = await getSharedCreation(id)
    if (id === shareId.value) detail.value = data
  } catch (e) {
    if (id !== shareId.value) return
    const value = e as { response?: { status?: number; data?: { error?: string } } }
    if (value.response?.status === 401) {
      clearUser()
      void router.replace({ path: '/login', query: { redirect: route.fullPath } })
      return
    }
    error.value = displayChinese(value.response?.data?.error, '分享已关闭、作品已还原，或你无权访问其中的照片。')
  }
}, { immediate: true })
</script>
<style scoped>
.shared-preview { position: fixed; inset: 0; height: 100dvh; display: flex; flex-direction: column; overflow: hidden; }
.shared-preview > header, .shared-preview > p { flex-shrink: 0; }
.shared-preview iframe { flex: 1; min-height: 0; width: 100%; border: 0; background: white; }
</style>
