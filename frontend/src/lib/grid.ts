// Grid column math.
//
// The grid and the page-size heuristic used to disagree: the CSS switched to
// 4 columns at 768px while calcLimit() still assumed 3, and below 768px the
// grid drew 3 columns while calcLimit() asked for 2. Both now call this.

/** Columns for a viewport. `mobileCols` overrides everything on the mobile shell. */
export function colsFor(width: number, mobileCols?: number | null): number {
  if (mobileCols) return mobileCols
  if (width >= 1280) return 6
  if (width >= 1024) return 5
  if (width >= 768) return 4
  return 3
}
