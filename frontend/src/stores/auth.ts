import { reactive, computed } from 'vue'
import { getMe, type User } from '@/api/auth'

interface AuthState {
  user: User | null
  loading: boolean
  checked: boolean // true after the initial me() call resolves
}

// Reactive singleton — no Pinia needed for a single global auth state.
export const authState = reactive<AuthState>({
  user: null,
  loading: false,
  checked: false,
})

export const isAdmin = computed(() => authState.user?.role === 'admin')
export const isGuest = computed(() => authState.user?.role === 'guest')
export const isLoggedIn = computed(() => authState.user !== null)

export async function checkAuth(): Promise<void> {
  if (authState.checked) return
  authState.loading = true
  try {
    authState.user = await getMe()
  } catch {
    authState.user = null
  } finally {
    authState.loading = false
    authState.checked = true
  }
}

export function setUser(user: User | null): void {
  authState.user = user
  authState.checked = true
}

export function clearUser(): void {
  authState.user = null
}
