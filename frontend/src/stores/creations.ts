import { ref, watch } from 'vue'
import { authState } from './auth'
import { api } from '@/api/client'

export const creationsEnabled = ref(false)
export const creationSharingEnabled = ref(false)
watch(() => authState.user, async (user, _old, onCleanup) => {
  creationsEnabled.value = false
  creationSharingEnabled.value = false
  if (!user || !['admin', 'guest'].includes(user.role)) return
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  try {
    const { data } = await api.get('/creations/capabilities', { signal: controller.signal })
    if (!controller.signal.aborted && authState.user?.id === user.id) {
      creationsEnabled.value = data.enabled === true
      creationSharingEnabled.value = data.sharing_enabled === true
    }
  } catch { /* A failed capability request leaves creation disabled. */ }
}, { immediate: true })
