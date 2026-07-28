import axios from 'axios'

const API_BASE = '/api/v1'

// In dev, serve thumbnails directly from the backend
// to avoid Vite proxy Content-Length issues.
export const THUMB_BASE = import.meta.env.DEV
  ? 'http://localhost:8080/api/thumbnails'
  : '/api/thumbnails'

export const api = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
})
