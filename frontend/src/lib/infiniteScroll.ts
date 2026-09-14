// Infinite-scroll decision logic for the photo grid.
//
// IntersectionObserver only reports *transitions* of the intersection state.
// The old code turned that into `if (intersecting && hasMore && !loading)
// loadPage()`, which silently dropped any trigger that arrived while a page
// was already in flight: when the sentinel then stayed inside the preload
// margin, no further callback ever came and the list stuck at the bottom with
// no way to recover.
//
// Loading is therefore modelled as a condition that can be re-evaluated at any
// time, from the observer, from the scroll handler, and at the end of every
// completed page. Keeping the decision pure puts the regression under test.

export interface AutoLoadState {
  hasMore: boolean
  loading: boolean
  /** Sentinel top edge in viewport coordinates. */
  sentinelTop: number
  viewportHeight: number
  /** How far ahead of the viewport the next page starts loading. */
  aheadPx: number
  /** Current timestamp. */
  now: number
  /** Retries are suppressed until this timestamp after a failed request. */
  retryAfter: number
}

export function shouldAutoLoad(s: AutoLoadState): boolean {
  if (!s.hasMore || s.loading) return false
  if (s.now < s.retryAfter) return false
  return s.sentinelTop <= s.viewportHeight + s.aheadPx
}

/** How far ahead of the viewport a page starts loading, in pixels. */
export function loadAheadPx(viewportHeight: number): number {
  return viewportHeight * 2
}
