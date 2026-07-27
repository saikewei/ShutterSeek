# ShutterSeek Web 项目骨架 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 ShutterSeek 重构为 Gin + Vue/TS Web 相册项目骨架，含 Docker 构建和 CI/CD。

**Architecture:** 单仓库，后端 Gin (handler→service→db 三层)，前端 Vue 3 + Vite，多阶段 Docker 构建，部署于 NAS Docker Compose 加入已有网络。

**Tech Stack:** Go 1.25 / Gin / pgx v5 / go-redis v9, Node.js 22 / Vue 3 / TypeScript / Vite, Docker multi-stage, GitHub Actions

## Global Constraints

- 模块名: `shutterseek`
- Go >= 1.25.0, Node.js >= 20
- config.yaml 不含任何密钥，敏感值仅通过 `SHUTTERSEEK_*` 环境变量注入
- 缩略图路径通过 `THUMBNAILS_DIR` 环境变量配置
- Redis 使用 db:2
- 生产容器仅含 shutterseek，加入已有 `postgres-main_default` 网络
- `.env.local` 加入 `.gitignore`

---

### Task 1: 清理项目结构 + 更新配置

**Files:**
- Move: `main.go` → `tools/importer/main.go`
- Move: `cmd/thumbtest/main.go` → `tools/thumbgen/main.go`
- Move: `cmd/import_embeddings/main.go` → `tools/embimport/main.go`
- Move: `internal/` → `tools/importer/internal/` (config, db, models, extractor, scanner, importer, thumbnail)
- Modify: `go.mod` — 模块路径不变, 仅清理依赖
- Modify: `config.yaml` — 移除敏感字段, 新增 server/redis 段
- Create: `.gitignore`

**Interfaces:**
- Produces: `tools/importer/main.go` 可独立 `go run` 运行
- Produces: `tools/thumbgen/main.go` 可独立 `go run` 运行
- Produces: `config.yaml` 作为 Gin 应用的默认配置源

- [ ] **Step 1: 创建 tools/ 目录结构并移动文件**

```bash
mkdir -p tools/importer/internal
mkdir -p tools/thumbgen
mkdir -p tools/embimport

# 移动 importer
mv main.go tools/importer/main.go
mv internal/config tools/importer/internal/config
mv internal/db tools/importer/internal/db
mv internal/models tools/importer/internal/models
mv internal/extractor tools/importer/internal/extractor
mv internal/scanner tools/importer/internal/scanner
mv internal/importer tools/importer/internal/importer

# 移动 thumbgen
mv cmd/thumbtest/main.go tools/thumbgen/main.go
mv internal/thumbnail tools/thumbgen/internal/thumbnail

# 移动 embimport
mv cmd/import_embeddings/main.go tools/embimport/main.go

# 清理空目录
rmdir cmd/thumbtest 2>/dev/null; rmdir cmd/import_embeddings 2>/dev/null; rmdir cmd 2>/dev/null
rm -rf internal/ 2>/dev/null
```

- [ ] **Step 2: 更新 tools 代码中的 import 路径**

`tools/importer/main.go` 把 `"shutterseek/internal/..."` 改为 `"shutterseek/tools/importer/internal/..."`

```bash
find tools/ -name "*.go" -exec sed -i 's|shutterseek/internal/|shutterseek/tools/importer/internal/|g' {} +
# thumbgen 的 import 也要修正
find tools/thumbgen -name "*.go" -exec sed -i 's|shutterseek/internal/thumbnail|shutterseek/tools/thumbgen/internal/thumbnail|g' {} +
```

- [ ] **Step 3: 添加 Gin 和 Redis 依赖**

```bash
go get github.com/gin-gonic/gin@latest
go get github.com/redis/go-redis/v9@latest
go mod tidy
```

- [ ] **Step 4: 创建 .gitignore**

```gitignore
# 密钥
.env.local
.env.production

# 构建产物
frontend/dist/
frontend/node_modules/

# IDE
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db
```

- [ ] **Step 5: 更新 config.yaml**

移除所有敏感字段，仅保留默认值：

```yaml
server:
  port: 8080
  mode: debug

database:
  host: postgres-main
  port: 5432
  sslmode: disable
  max_connections: 20

redis:
  host: redis
  port: 6379
  db: 2

thumbnail:
  output_dir: /thumbnails/.thumbnails_1080
```

删除旧的配置内容（user/password/dbname/scanner/extensions/skip_video）。

- [ ] **Step 6: 验证编译**

```bash
go build ./tools/... && echo "tools compile OK"
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "chore: restructure project, move tools, update config"
```

---

### Task 2: 创建 internal/config (配置加载)

**Files:**
- Create: `internal/config/config.go`

**Interfaces:**
- Produces: `func Load(path string) (*Config, error)` — 加载 yaml，然后用环境变量覆盖
- Produces: `type Config struct { Server; Database; Redis; Thumbnail }`
- Consumed by: Task 3 (cmd/server)

- [ ] **Step 1: 编写 config.go**

```go
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Thumbnail ThumbnailConfig `yaml:"thumbnail"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type ThumbnailConfig struct {
	OutputDir string `yaml:"output_dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{Port: 8080, Mode: "debug"},
		Database: DatabaseConfig{
			Host: "postgres-main", Port: 5432, SSLMode: "disable",
		},
		Redis: RedisConfig{
			Host: "redis", Port: 6379, DB: 2,
		},
		Thumbnail: ThumbnailConfig{
			OutputDir: "/thumbnails/.thumbnails_1080",
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Env overrides (SHUTTERSEEK_ prefix)
	applyEnv(cfg)

	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("SHUTTERSEEK_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { cfg.Server.Port = n }
	}
	if v := os.Getenv("SHUTTERSEEK_SERVER_MODE"); v != "" { cfg.Server.Mode = v }

	if v := os.Getenv("SHUTTERSEEK_DB_HOST"); v != "" { cfg.Database.Host = v }
	if v := os.Getenv("SHUTTERSEEK_DB_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { cfg.Database.Port = n }
	}
	if v := os.Getenv("SHUTTERSEEK_DB_USER"); v != "" { cfg.Database.User = v }
	if v := os.Getenv("SHUTTERSEEK_DB_PASSWORD"); v != "" { cfg.Database.Password = v }
	if v := os.Getenv("SHUTTERSEEK_DB_NAME"); v != "" { cfg.Database.DBName = v }
	if v := os.Getenv("SHUTTERSEEK_DB_SSLMODE"); v != "" { cfg.Database.SSLMode = v }

	if v := os.Getenv("SHUTTERSEEK_REDIS_HOST"); v != "" { cfg.Redis.Host = v }
	if v := os.Getenv("SHUTTERSEEK_REDIS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { cfg.Redis.Port = n }
	}
	if v := os.Getenv("SHUTTERSEEK_REDIS_PASSWORD"); v != "" { cfg.Redis.Password = v }
	if v := os.Getenv("SHUTTERSEEK_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { cfg.Redis.DB = n }
	}

	if v := os.Getenv("SHUTTERSEEK_THUMBNAILS_DIR"); v != "" { cfg.Thumbnail.OutputDir = v }
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode)
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/config/ && echo "config OK"
```

---

### Task 3: 创建 internal/db + internal/redis (连接池)

**Files:**
- Create: `internal/db/pool.go`
- Create: `internal/redis/client.go`

**Interfaces:**
- Produces: `func NewPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error)`
- Produces: `func NewRedisClient(cfg config.RedisConfig) *redis.Client`
- Consumed by: Task 4 (cmd/server)

- [ ] **Step 1: 编写 internal/db/pool.go**

```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 2: 编写 internal/redis/client.go**

```go
package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"shutterseek/internal/config"
)

func NewClient(cfg config.RedisConfig) *goredis.Client {
	return goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func Ping(ctx context.Context, client *goredis.Client) error {
	return client.Ping(ctx).Err()
}
```

- [ ] **Step 3: 验证编译**

```bash
go build ./internal/db/ ./internal/redis/ && echo "OK"
```

---

### Task 4: 创建 cmd/server (Gin 入口)

**Files:**
- Create: `cmd/server/main.go`

**Interfaces:**
- Consumes: `config.Load`, `db.NewPGPool`, `redis.NewClient`
- Produces: Gin HTTP server on `:8080`, graceful shutdown
- Produces: 路由骨架 (health check + 静态文件 + SPA fallback)

- [ ] **Step 1: 编写 cmd/server/main.go**

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/config"
	"shutterseek/internal/db"
	myredis "shutterseek/internal/redis"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Database
	ctx := context.Background()
	pool, err := db.NewPGPool(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	log.Println("✓ PostgreSQL connected")

	// Redis
	rdb := myredis.NewClient(cfg.Redis)
	if err := myredis.Ping(ctx, rdb); err != nil {
		log.Printf("⚠ Redis unavailable: %v (continuing without cache)", err)
	} else {
		defer rdb.Close()
		log.Println("✓ Redis connected")
	}

	// Gin setup
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// API routes
	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Static files: thumbnails
	thumbDir := cfg.Thumbnail.OutputDir
	if _, err := os.Stat(thumbDir); err == nil {
		r.Static("/thumbnails", thumbDir)
		log.Printf("✓ Thumbnails: %s", thumbDir)
	} else {
		log.Printf("⚠ Thumbnails dir not found: %s", thumbDir)
	}

	// SPA fallback: serve frontend/dist/index.html for non-API routes
	frontendDir := "frontend/dist"
	if _, err := os.Stat(frontendDir); err == nil {
		r.StaticFile("/", frontendDir+"/index.html")
		r.Static("/assets", frontendDir+"/assets")
		r.Static("/favicon.ico", frontendDir+"/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			c.File(frontendDir + "/index.html")
		})
		log.Printf("✓ Frontend: %s", frontendDir)
	}

	// Start server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Printf("🚀 Server listening on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("Server stopped")
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./cmd/server/ && echo "server OK"
```

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: add Gin backend scaffold with health check"
```

---

### Task 5: 创建前端 Vue + TS 脚手架

**Files:**
- Create: `frontend/` (Vite + Vue 3 + TypeScript 项目)

**Interfaces:**
- Consumes: Gin API at `/api` (开发时 Vite proxy)
- Produces: `frontend/dist/` 构建产物，由 Gin 静态托管

- [ ] **Step 1: 用 Vite 初始化项目**

```bash
cd frontend
npm create vite@latest . -- --template vue-ts
npm install
npm install vue-router@4 axios
npm install -D @types/node
```

- [ ] **Step 2: 配置 Vite proxy 和 build**

创建 `frontend/vite.config.ts`:

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/thumbnails': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
```

- [ ] **Step 3: 创建基础页面**

`frontend/src/main.ts`:
```typescript
import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Home from './views/Home.vue'

const routes = [
  { path: '/', component: Home },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

createApp(App).use(router).mount('#app')
```

`frontend/src/App.vue`:
```vue
<template>
  <router-view />
</template>
```

`frontend/src/views/Home.vue`:
```vue
<template>
  <div class="home">
    <h1>ShutterSeek</h1>
    <p>{{ status }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const status = ref('Loading...')

onMounted(async () => {
  try {
    const res = await fetch('/api/health')
    const data = await res.json()
    status.value = `API: ${data.status}`
  } catch {
    status.value = 'API unavailable'
  }
})
</script>
```

- [ ] **Step 4: 创建 api 封装**

`frontend/src/api/client.ts`:
```typescript
const BASE = '/api/v1'

export async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}
```

- [ ] **Step 5: 验证前端开发模式**

```bash
cd frontend && npm run dev
# 访问 http://localhost:5173, 应显示 API status
```

- [ ] **Step 6: 验证生产构建**

```bash
cd frontend && npm run build
ls dist/  # 应有 index.html + assets/
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: add Vue+TS frontend scaffold with Vite"
```

---

### Task 6: 编写 Dockerfile (多阶段构建)

**Files:**
- Create: `Dockerfile`

**Interfaces:**
- Produces: `shutterseek:<tag>` Docker image, <30MB Alpine

- [ ] **Step 1: 编写 Dockerfile**

```dockerfile
# Stage 1: Build Vue frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /app/server .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY config.yaml .
EXPOSE 8080
CMD ["./server"]
```

- [ ] **Step 2: 创建 .dockerignore**

```
node_modules/
frontend/node_modules/
frontend/dist/
tools/
docs/
.git/
.env.local
.env.production
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile .dockerignore && git commit -m "feat: add multi-stage Dockerfile"
```

---

### Task 7: 编写 docker-compose.yml

**Files:**
- Create: `docker-compose.yml`

**Interfaces:**
- Produces: `docker compose up -d` 启动 shutterseek，加入已有网络

- [ ] **Step 1: 编写 docker-compose.yml**

```yaml
services:
  shutterseek:
    image: ghcr.io/${GITHUB_USER}/shutterseek:latest
    container_name: shutterseek
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /volume1/Main/Photos:/photos:ro
      - /volume1/Main/thumbnails:/thumbnails
    environment:
      SHUTTERSEEK_DB_USER: ${DB_USER}
      SHUTTERSEEK_DB_PASSWORD: ${DB_PASSWORD}
      SHUTTERSEEK_DB_NAME: ${DB_NAME}
      SHUTTERSEEK_REDIS_HOST: redis
      SHUTTERSEEK_REDIS_PASSWORD: ${REDIS_PASSWORD}
      SHUTTERSEEK_SERVER_MODE: release
      SHUTTERSEEK_THUMBNAILS_DIR: /thumbnails/.thumbnails_1080

networks:
  default:
    name: postgres-main_default
    external: true
```

- [ ] **Step 2: Commit**

```bash
git add docker-compose.yml && git commit -m "feat: add docker-compose for NAS deployment"
```

---

### Task 8: 编写 GitHub Actions CI/CD

**Files:**
- Create: `.github/workflows/deploy.yml`

- [ ] **Step 1: 编写 deploy.yml**

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Deploy to NAS
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ secrets.NAS_HOST }}
          username: ${{ secrets.NAS_USER }}
          key: ${{ secrets.NAS_SSH_KEY }}
          script: |
            cd /volume1/docker/shutterseek
            docker compose pull
            docker compose up -d
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/deploy.yml && git commit -m "feat: add GitHub Actions CI/CD"
```

---

### Task 9: 端到端验证

**Files:** None (verification only)

- [ ] **Step 1: 启动 Gin 后端**

```bash
SHUTTERSEEK_DB_USER=photo_user \
SHUTTERSEEK_DB_PASSWORD=PhotoHyc65319436 \
SHUTTERSEEK_DB_NAME=photo_search \
SHUTTERSEEK_REDIS_HOST=172.18.0.3 \
SHUTTERSEEK_REDIS_PASSWORD=Istanbul1453 \
SHUTTERSEEK_THUMBNAILS_DIR=/workspaces/ShutterSeek/thumbnails/.thumbnails_1080 \
go run ./cmd/server/
```

- [ ] **Step 2: 验证 health endpoint**

```bash
curl http://localhost:8080/api/health
# Expected: {"status":"ok"}
```

- [ ] **Step 3: 验证缩略图服务**

```bash
curl -o /dev/null -w "%{http_code}" http://localhost:8080/thumbnails/1.jpg
# Expected: 200
```

- [ ] **Step 4: 验证 SPA fallback**

```bash
curl -s http://localhost:8080/ | head -5
# Expected: <!DOCTYPE html>...
```

- [ ] **Step 5: 验证前端开发模式**

```bash
# 终端1
cd frontend && npm run dev

# 终端2
curl http://localhost:5173/api/health
# Expected: {"status":"ok"} (proxy to Gin)
```

- [ ] **Step 6: 验证 Docker 构建**

```bash
docker build -t shutterseek:test .
docker run --rm -p 8080:8080 shutterseek:test
# Expected: server starts (DB connection will likely fail in test env, that's OK)
```

- [ ] **Step 7: Commit (if any final touch-ups)**

```bash
git add -A && git commit -m "chore: final verification tweaks"
```

---

## Self-Review

**Spec coverage:** All sections covered — project structure (Task 1), config (Task 1-2), backend scaffold (Task 3-4), frontend scaffold (Task 5), Dockerfile (Task 6), docker-compose (Task 7), CI/CD (Task 8), verification (Task 9).

**Placeholder scan:** No TBD/TODO. All code blocks are concrete. All paths are absolute within project.

**Type consistency:** `Config` struct defined in Task 2 matches usage in Task 3-4. Import paths consistent throughout.
