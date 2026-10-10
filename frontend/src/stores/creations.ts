import { ref, watch } from 'vue'
import { isAdmin } from './auth'
import { api } from '@/api/client'

export const creationsEnabled = ref(false)
watch(isAdmin, async (admin) => {
  creationsEnabled.value = false
  if (!admin) return
  try { creationsEnabled.value = (await api.get('/creations/capabilities')).data.enabled === true }
  catch { creationsEnabled.value = false }
}, { immediate: true })
