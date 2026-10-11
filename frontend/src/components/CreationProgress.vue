<template>
  <div class="creation-progress rounded-xl border border-accent/25 bg-raised p-4" :class="{ 'progress-compact': compact }" role="status" aria-live="polite">
    <div class="progress-summary flex items-start gap-3">
      <span class="progress-spinner mt-1 shrink-0" aria-hidden="true"></span>
      <div class="flex-1 min-w-0">
        <div class="progress-heading">
          <button v-if="compact" class="flex-1 text-left text-xs font-medium text-ink flex items-center gap-1" :aria-expanded="expanded" :aria-label="expanded ? '收起生成进度' : '展开生成进度'" @click="expanded = !expanded">
            <span>{{ text.title }}</span><span aria-hidden="true">{{ expanded ? '⌃' : '⌄' }}</span>
          </button>
          <p v-else class="text-sm font-medium text-ink">{{ text.title }}</p>
          <span v-if="compact" class="text-[10px] text-ink-3 shrink-0">{{ elapsedLabel }}</span>
        </div>
        <template v-if="!compact || expanded">
          <p class="text-xs text-ink-2 mt-1 leading-relaxed">{{ text.detail }}</p>
          <p class="text-xs text-ink-3 mt-2">{{ elapsedLabel }} · 关闭页面不会取消任务</p>
        </template>
        <p v-if="reconnecting" class="text-xs text-ink-2 mt-1">连接暂时中断，正在恢复进度…</p>
      </div>
      <button class="btn-ghost px-3 py-1.5 text-xs shrink-0" @click="$emit('cancel')">取消任务</button>
    </div>
    <ol class="flex gap-1 mt-4" aria-label="创作进度">
      <li v-for="(item, index) in progressSteps" :key="item" class="flex-1 min-w-0" :aria-current="index === step ? 'step' : undefined">
        <div class="h-1 rounded-full" :class="[index <= step ? 'bg-accent' : 'bg-line', { 'mb-1.5': !compact || expanded }]"></div>
        <span v-if="!compact || expanded" class="text-[10px] sm:text-xs" :class="index === step ? 'text-ink font-medium' : 'text-ink-3'">{{ item }}</span>
      </li>
    </ol>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { progressSteps, progressStep, progressText, type CreationProgress } from '@/lib/creationChat'
const props = defineProps<{ progress: CreationProgress; startedAt: number; reconnecting: boolean; compact?: boolean }>()
defineEmits<{ cancel: [] }>()
const expanded = ref(false)
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => clearInterval(timer))
const text = computed(() => progressText(props.progress))
const step = computed(() => progressStep(props.progress.stage))
const elapsedLabel = computed(() => {
  const seconds = Math.max(0, Math.floor((now.value - props.startedAt) / 1000))
  return seconds < 60 ? '已等待 ' + seconds + ' 秒' : '已等待 ' + Math.floor(seconds / 60) + ' 分 ' + seconds % 60 + ' 秒'
})
</script>
<style scoped>
.progress-compact { padding: .5rem .75rem; }
.progress-compact .progress-spinner { margin-top: 0; }
.progress-compact .progress-summary { align-items: center; }
.progress-compact .progress-heading { display: flex; align-items: center; gap: .5rem; }
.progress-compact .progress-heading > button { min-width: 0; }
.progress-compact .progress-heading > button > span:first-child { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.progress-compact ol { margin-top: .5rem; }
.progress-spinner { width: 14px; height: 14px; border: 2px solid var(--color-line); border-top-color: var(--color-accent); border-radius: 50%; animation: progress-spin 1s linear infinite; }
@keyframes progress-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .progress-spinner { animation: none; } }
</style>
