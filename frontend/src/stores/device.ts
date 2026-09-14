// Layout mode detection.
//
// The shell is chosen by viewport + pointer type rather than user agent:
// iPadOS reports a desktop UA ("Macintosh"), and a narrow desktop window
// deserves the same compact shell as a phone.

import { computed, ref } from 'vue'

export const MOBILE_SHELL_QUERIES = [
  '(max-width: 767px)',
  '(max-width: 1024px) and (pointer: coarse)',
]

/** Pure predicate so the rule can be unit-tested without a browser. */
export function matchesMobileShell(matches: (query: string) => boolean): boolean {
  return MOBILE_SHELL_QUERIES.some((q) => matches(q))
}

const supported = typeof window !== 'undefined' && typeof window.matchMedia === 'function'

const queryMatches = ref(
  supported ? matchesMobileShell((q) => window.matchMedia(q).matches) : false,
)

if (supported) {
  const lists = MOBILE_SHELL_QUERIES.map((q) => window.matchMedia(q))
  const sync = () => {
    queryMatches.value = lists.some((l) => l.matches)
  }
  for (const list of lists) list.addEventListener('change', sync)
}

/**
 * True when the compact shell should be used: bottom tab bar, document-level
 * scrolling, no sidebar. Reactive to resize and rotation.
 */
export const isMobileShell = computed(() => queryMatches.value)
