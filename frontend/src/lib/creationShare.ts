const sharedPath = /^\/creations\/shared\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?:\?[^#]*)?$/i
export function sharedLoginTarget(value: unknown): string {
  return typeof value === 'string' && sharedPath.test(value) ? value : '/'
}
export function creationShareURL(id: string): string {
  return location.origin + location.pathname + '#/creations/shared/' + encodeURIComponent(id)
}
