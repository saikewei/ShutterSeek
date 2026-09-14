import axios from 'axios'

const API_BASE = '/api/v1'

// Thumbnails live under the authenticated API now, so they must go through the
// same origin (and carry the session cookie) as everything else.
//
// In dev the absolute URL skips the Vite proxy. It only works when the browser
// really runs on this machine: opening the dev server from a phone would
// resolve `localhost` to the phone itself and every thumbnail would break, so
// use the proxied path there.
const onLocalhost =
  typeof location !== 'undefined' &&
  (location.hostname === 'localhost' || location.hostname === '127.0.0.1')

export const THUMB_BASE =
  import.meta.env.DEV && onLocalhost
    ? 'http://localhost:8080/api/v1/thumbnails'
    : '/api/v1/thumbnails'

export const api = axios.create({
  baseURL: API_BASE,
  timeout: 60000,
})
