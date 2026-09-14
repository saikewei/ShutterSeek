// Thumbnail preload scheduler.
//
// Native `loading="lazy"` only starts a fetch once a cell is roughly two
// viewports away, and then every cell in that band starts at once. Against a
// cold NAS that burst takes long enough that cells entering the viewport are
// still blank -- which reads as "the thumbnail never loads".
//
// This scheduler warms the browser cache further ahead and at a bounded rate,
// so the <img> usually finds its bytes already cached and paints immediately.
// Native lazy loading stays in place as the safety net.
//
// Two priorities:
//   'visible' -- the cell is on screen right now, jump the queue
//   'ahead'   -- the cell is approaching, fill the cache in the background
//
// Kept free of DOM access so the queueing rules can be unit-tested.

export type PreloadPriority = 'visible' | 'ahead'

export interface PreloadScheduler {
  /** True when the id is queued, in flight, or already loaded. */
  has(id: number): boolean
  /** Queue an id. Re-scheduling a known id at a higher priority promotes it. */
  schedule(id: number, priority: PreloadPriority): void
  /** Number of ids not yet finished. */
  pending(): number
  /** Drop everything that has not started yet. */
  clear(): void
}

/** First value of a Set, i.e. the oldest insertion. */
function firstOf(set: Set<number>): number | undefined {
  for (const v of set) return v
  return undefined
}

/**
 * @param load     starts one fetch; must call `done` exactly once
 * @param workers  maximum concurrent fetches
 */
export function createPreloadScheduler(
  load: (id: number, done: () => void) => void,
  workers = 6,
): PreloadScheduler {
  const visible = new Set<number>()
  const ahead = new Set<number>()
  const known = new Set<number>()
  let active = 0

  function pump() {
    while (active < workers) {
      const next = firstOf(visible) ?? firstOf(ahead)
      if (next === undefined) return
      visible.delete(next)
      ahead.delete(next)
      active++
      let finished = false
      load(next, () => {
        // A loader that calls back twice must not free two slots.
        if (finished) return
        finished = true
        active--
        pump()
      })
    }
  }

  return {
    has: (id) => known.has(id),
    pending: () => visible.size + ahead.size + active,

    schedule(id, priority) {
      if (known.has(id)) {
        // Already waiting in the background but now on screen: promote it.
        if (priority === 'visible' && ahead.has(id)) {
          ahead.delete(id)
          visible.add(id)
          pump()
        }
        return
      }
      known.add(id)
      if (priority === 'visible') visible.add(id)
      else ahead.add(id)
      pump()
    },

    clear() {
      visible.clear()
      ahead.clear()
    },
  }
}
