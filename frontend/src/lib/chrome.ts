// Sticky chrome geometry.
//
// Sticky offsets used to be hard-coded numbers spread over PhotoGrid (37, 46)
// and AlbumDetail (53). They are all "how much sticky chrome is above me in the
// current scroll container", which differs per shell, so they are now computed
// from measured element heights and shared through provide/inject.

import { onUnmounted, ref, watch, type InjectionKey, type Ref } from 'vue'

/**
 * Height of the sticky chrome pinned above the content in the current scroll
 * container: the mobile top bar, AlbumDetail's header, or 0 when nothing is
 * pinned (Home, Search, desktop shell).
 *
 * A getter rather than a ref so a page can stack its own header on top of
 * whatever the shell already pinned.
 */
export const topOffsetKey: InjectionKey<() => number> = Symbol('ss-top-offset')

/**
 * Height of a rendered element, kept in sync through a ResizeObserver.
 * The getter is re-evaluated after every render, so elements behind a v-if
 * (the mobile shell, the filter bar) are picked up when they appear and
 * reported as 0 while they are gone.
 */
export function useElementHeight(getEl: () => HTMLElement | null | undefined): Ref<number> {
  const height = ref(0)
  let observer: ResizeObserver | null = null

  const detach = () => {
    observer?.disconnect()
    observer = null
  }

  const attach = (node: HTMLElement | null | undefined) => {
    detach()
    height.value = node ? node.offsetHeight : 0
    if (!node || typeof ResizeObserver === 'undefined') return
    observer = new ResizeObserver(() => {
      height.value = node.offsetHeight
    })
    observer.observe(node)
  }

  watch(getEl, attach, { immediate: true, flush: 'post' })
  onUnmounted(detach)

  return height
}
