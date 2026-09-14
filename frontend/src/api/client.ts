import axios from 'axios'

const API_BASE = '/api/v1'

// In dev, serve thumbnails directly from the backend
// to avoid Vite proxy Content-Length issues.
//
// The absolute URL only works when the browser really runs on this machine:
// opening the dev server from a phone would resolve `localhost` to the phone
// itself and every thumbnail would break, so use the proxied path there.
const onLocalhost =
  typeof location !== 'undefined' &&
  (location.hostname === 'localhost' || location.hostname === '127.0.0.1')

export const THUMB_BASE =
  import.meta.env.DEV && onLocalhost
    ? 'http://localhost:8080/api/thumbnails'
    : '/api/thumbnails'

export const api = axios.create({
  baseURL: API_BASE,
  timeout: 60000,
})
