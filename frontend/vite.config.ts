import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'url'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import type { Plugin } from 'vite'

const ortDir = fileURLToPath(new URL('./public/ort', import.meta.url))

// onnxruntime-web 在 WebGPU (jsep) 路径下会动态 import('/ort/*.mjs')，
// Vite 默认禁止把 public 文件当模块导入（只在构建时原样拷贝）。
// 这里在 transform 管线之前直接静态返回 /ort/* 文件。
function ortDevStatic(): Plugin {
  return {
    name: 'ort-dev-static',
    configureServer(server) {
      server.middlewares.use('/ort', async (req, res, next) => {
        try {
          const name = decodeURIComponent((req.url || '').split('?')[0]).replace(/^\/+/, '')
          if (!name || name.includes('..')) return next()
          const data = await readFile(join(ortDir, name))
          res.setHeader('Content-Type', name.endsWith('.wasm') ? 'application/wasm' : 'text/javascript')
          res.setHeader('Cache-Control', 'no-cache')
          res.end(data)
        } catch {
          next()
        }
      })
    },
  }
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), ortDevStatic()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        // 批量上传：一次请求可能带几百 MB，dev 代理别提前掐断
        timeout: 300000,
        proxyTimeout: 300000,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq, req) => {
            req.headers['connection'] = 'keep-alive'
          })
          proxy.on('error', (err, req, res) => {
            console.error('[vite proxy error]', err.message)
          })
        },
      },
      // Thumbnails are served under /api/v1/thumbnails, so the '/api' rule
      // above covers them; the old public '/thumbnails' alias is gone.
      '/models': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
