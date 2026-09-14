// Sticky chrome geometry.
//
// Sticky offsets used to be hard-coded numbers spread over PhotoGrid (37, 46)
// and AlbumDetail (53). They are all "how much sticky chrome is above me in the
// current scroll container", which differs per shell, so they are now computed
// from measured element heights and shared through provide/inject.

import { onMounted, onUnmounted, ref, type InjectionKey, type Ref } from 'vue'

/**
 * Height of the sticky chrome pinned above the content in the current scroll
 * container: the mobile top bar, AlbumDetail's header, or 0 when nothing is
 * pinned (Home, Search, desktop shell).
 *
 * A getter rather than a ref so pages can stack their own header on top of
 * whatever the shell already pinned.
 */
export const topOffsetKey: InjectionKey<() => number> = Symbol('ss-top-offset')

/**
 * Height of a rendered element, kept in sync through a ResizeObserver.
 * Returns 0 until the element is mounted.
 */
export function useElementHeight(el: Ref<HTMLElement | null>): Ref<number> {
  const height = ref(0)
  let observer: ResizeObserver | null = null

  onMounted(() => {
    const node = el.value
    if (!node) return
    const update = () => {
      height.value = node.offsetHeight
    }
    update()
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(update)
      observer.observe(node)
    }
  })

  onUnmounted(() => {
    observer?.disconnect()
    observer = null
  })

  return height
}
