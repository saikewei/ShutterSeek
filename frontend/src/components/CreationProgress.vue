<template>
  <div class="creation-progress rounded-xl border border-accent/25 bg-raised p-4" role="status" aria-live="polite">
    <div class="flex items-start gap-3">
      <span class="progress-spinner mt-1 shrink-0" aria-hidden="true"></span>
      <div class="flex-1 min-w-0">
        <p class="text-sm font-medium text-ink">{{ text.title }}</p>
        <p class="text-xs text-ink-2 mt-1 leading-relaxed">{{ text.detail }}</p>
        <p class="text-xs text-ink-3 mt-2">{{ elapsedLabel }} · 关闭页面不会取消任务</p>
        <p v-if="reconnecting" class="text-xs text-ink-2 mt-1">连接暂时中断，正在恢复进度…</p>
      </div>
      <button class="btn-ghost px-3 py-1.5 text-xs shrink-0" @click="$emit('cancel')">取消任务</button>
    </div>
    <ol class="flex gap-1 mt-4" aria-label="创作进度">
      <li v-for="(item, index) in progressSteps" :key="item" class="flex-1 min-w-0" :aria-current="index === step ? 'step' : undefined">
        <div class="h-1 rounded-full mb-1.5" :class="index <= step ? 'bg-accent' : 'bg-line'"></div>
        <span class="text-[10px] sm:text-xs" :class="index === step ? 'text-ink font-medium' : 'text-ink-3'">{{ item }}</span>
      </li>
    </ol>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { progressSteps, progressStep, progressText, type CreationProgress } from '@/lib/creationChat'
const props = defineProps<{ progress: CreationProgress; startedAt: number; reconnecting: boolean }>()
defineEmits<{ cancel: [] }>()
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
.progress-spinner { width: 14px; height: 14px; border: 2px solid var(--color-line); border-top-color: var(--color-accent); border-radius: 50%; animation: progress-spin 1s linear infinite; }
@keyframes progress-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .progress-spinner { animation: none; } }
</style>
