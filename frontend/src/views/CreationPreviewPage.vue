<template>
  <div class="full-preview bg-canvas text-ink">
    <header class="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3 bg-raised">
      <RouterLink :to="'/creations/' + workId" class="btn-ghost px-3 py-2 text-xs">← 返回创作</RouterLink>
      <h1 class="text-sm font-medium flex-1 truncate">{{ detail?.creation.title || '作品完整预览' }}</h1>
      <select v-if="detail" v-model="version" class="input !w-auto max-w-48 !py-1.5 !text-xs" aria-label="完整预览版本">
        <option v-for="item in detail.versions" :key="item.id" :value="item.id">{{ date(item.created_at) }} · {{ item.verified ? '检查通过' : '候选版本' }}</option>
      </select>
      <div class="flex gap-1" aria-label="预览宽度">
        <button v-for="item in ['自适应', '宽屏', '手机']" :key="item" class="pill" :class="mode === item ? 'bg-accent-soft text-accent-strong' : 'text-ink-3'" :aria-pressed="mode === item" @click="mode = item">{{ item }}</button>
      </div>
    </header>
    <p v-if="error" role="alert" class="p-4 text-danger-ink text-sm">{{ error }}</p>
    <main class="preview-canvas">
      <iframe v-if="version" :key="version" :src="previewURL(workId, version)" sandbox="allow-scripts" referrerpolicy="no-referrer" title="完整作品预览" :style="{ maxWidth: mode === '手机' ? '390px' : mode === '宽屏' ? '1440px' : '100%' }" />
      <p v-else class="text-sm text-ink-3 p-8">{{ detail ? '这份作品还没有生成可预览的版本。' : '正在加载作品…' }}</p>
    </main>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getCreation, previewURL, type WorkDetail } from '@/api/creations'
const route = useRoute(), workId = computed(() => String(route.params.id))
const detail = ref<WorkDetail | null>(null), version = ref(''), mode = ref('自适应'), error = ref('')
function date(value: string) { return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) }
watch(() => workId.value, async id => {
  detail.value = null; error.value = ''; version.value = ''
  try {
    const data = await getCreation(id); if (id !== workId.value) return
    detail.value = data
    const requested = String(route.query.version || '')
    version.value = data.versions.some(item => item.id === requested) ? requested : data.creation.current_version
  } catch { error.value = '作品加载失败，可能已失效或无权访问。' }
}, { immediate: true })
</script>
<style scoped>
.full-preview { position: fixed; inset: 0; display: flex; flex-direction: column; height: 100dvh; overflow: hidden; }
.full-preview > header { flex-shrink: 0; }
.preview-canvas { flex: 1; min-height: 0; display: flex; justify-content: center; background: var(--color-surface); }
.preview-canvas iframe { width: 100%; height: 100%; border: 0; background: white; }
</style>
