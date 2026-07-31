import { ref, computed } from 'vue'
import { authState } from './auth'

const MOBILE_RE = /Android|iPhone|iPad|iPod|Mobile/i

// UA-based detection — mobile shell applies only to guests.
export const isMobile = ref(MOBILE_RE.test(navigator.userAgent))

export const isGuestMobile = computed(() => isMobile.value && authState.user?.role === 'guest')
