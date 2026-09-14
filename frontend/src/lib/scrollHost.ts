// Scroll host abstraction.
//
// The desktop shell scrolls an inner element (<main data-scroll-host>); the
// mobile shell scrolls the document itself, because iOS Safari only collapses
// its bottom toolbar when the *document* scrolls -- a nested scroll container
// never triggers that. Everything that needs "the current scroller" goes
// through this module instead of a hard-coded querySelector.

const HOST_SELECTOR = '[data-scroll-host]'

export function hostEl(): HTMLElement | null {
  if (typeof document === 'undefined') return null
  return document.querySelector<HTMLElement>(HOST_SELECTOR)
}

function docScroller(): HTMLElement {
  return (document.scrollingElement as HTMLElement | null) ?? document.documentElement
}

/** Current scroll offset of the active host (0 when not scrollable). */
export function getScrollTop(): number {
  const el = hostEl()
  return el ? el.scrollTop : docScroller().scrollTop
}

export function setScrollTop(top: number): void {
  const el = hostEl()
  if (el) {
    el.scrollTop = top
    return
  }
  docScroller().scrollTop = top
}

/** Full scrollable height of the active host. */
export function hostScrollHeight(): number {
  const el = hostEl()
  return el ? el.scrollHeight : docScroller().scrollHeight
}

/**
 * Viewport-relative top edge of the host. Zero when the document is the host,
 * since the document's top edge *is* the viewport top.
 */
export function hostViewportTop(): number {
  const el = hostEl()
  return el ? el.getBoundingClientRect().top : 0
}

export function scrollHostToTop(smooth = false): void {
  const behavior: ScrollBehavior = smooth ? 'smooth' : 'auto'
  const el = hostEl()
  if (el) {
    el.scrollTo({ top: 0, behavior })
    return
  }
  window.scrollTo({ top: 0, behavior })
}

/** Shift the scroll offset by `delta` px (used to keep content anchored). */
export function scrollHostBy(delta: number): void {
  if (!delta) return
  setScrollTop(getScrollTop() + delta)
}

/**
 * Subscribe to scroll events on the active host. Resolved once at subscribe
 * time, so call it from onMounted (after the shell has rendered).
 */
export function onHostScroll(cb: () => void): () => void {
  const el = hostEl()
  const target: EventTarget = el ?? window
  target.addEventListener('scroll', cb, { passive: true })
  return () => target.removeEventListener('scroll', cb)
}

/**
 * Hide the scrollable content while measuring or jumping, so an intermediate
 * scroll position is never painted. The document host hides #app, because
 * hiding <body> would take the fixed chrome down with it.
 */
export function setHostVisibility(visible: boolean): void {
  const el = hostEl()
  const target = el ?? document.getElementById('app')
  if (target) target.style.visibility = visible ? '' : 'hidden'
}
