/** Float16Array 在 TS 5.6 / ES2020 lib 下不可用，运行时统一走 globalThis。 */
export function float16Array(length: number): any {
  return new (globalThis as any).Float16Array(length)
}
