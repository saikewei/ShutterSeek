# ShutterSeek Web 相册 — 设计文档

**日期**: 2026-07-27  
**状态**: Draft

## 1. 概述

将 ShutterSeek 重构为 Web 相册应用，后端 Gin + 前端 Vue/TypeScript，部署于 NAS Docker 环境。此阶段仅搭建项目骨架（目录结构、构建流程、Dockerfile、CI/CD），不实现具体功能。

## 2. 部署拓扑

```
NAS (Docker)
├── postgres-main    (已存在, network: postgres-main_default)
├── redis            (已存在, 同网络)
└── shutterseek      (新增, 同网络)
    ├── /photos      → NAS bind mount READONLY
    └── /thumbnails  → NAS bind mount READWRITE
```

- ShutterSeek 加入已有 Docker 网络 `postgres-main_default`
- PG 和 Redis 容器不在本项目的 docker-compose 中管理
- 缩略图路径通过环境变量 `THUMBNAILS_DIR` 配置，开发/生产各自指定

## 3. 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 后端框架 | Gin (Go) | HTTP API + 静态文件服务 |
| 数据库 | PostgreSQL + pgvector | 已有 `photos` + `photo_embeddings` |
| 缓存 | Redis (go-redis/v9) | Session / 热点缩略图 / API 限流 |
| 前端 | Vue 3 + TypeScript + Vite | SPA, 生产构建产物由 Gin 静态托管 |
| 部署 | Docker Compose | 三容器栈 |
| CI/CD | GitHub Actions | 构建镜像 → 推送 → NAS 拉取部署 |

## 4. 开发环境

### 4.1 Dev Container (.devcontainer/)

当前 Dockerfile 仅含 Go + exiftool。需要新增：

- **Node.js 20 LTS** — Vue/Vite 编译需要
- **保留**: Go 1.24, exiftool, libvips-dev, air (热重载)
- 新增 `postCreateCommand`: `cd frontend && npm install`

`.devcontainer/devcontainer.json` 不需要改（mounts、network、shutdownAction 等不变）。

### 4.2 开发工作流

```
终端1: cd frontend && npm run dev     → Vite :5173 (HMR)
终端2: go run ./cmd/server/            → Gin :8080 (API)
                                     → Vite proxy /api → :8080
```

浏览器访问 `localhost:5173`，API 请求通过 Vite proxy 转发到 Gin。

## 5. 项目结构

```
ShutterSeek/
├── cmd/
│   └── server/           # 主应用入口 (新增)
│       └── main.go
├── internal/
│   ├── config/           # 配置 (复用)
│   ├── db/               # PG 连接池 (复用)
│   ├── models/           # 数据模型 (复用)
│   ├── handler/          # HTTP handler (新增, 每个资源一个文件)
│   ├── service/          # 业务逻辑 (新增)
│   └── middleware/       # 中间件 (新增)
├── frontend/             # Vue + TS (新增)
│   ├── src/
│   │   ├── main.ts
│   │   ├── App.vue
│   │   ├── router/
│   │   ├── views/
│   │   ├── components/
│   │   └── api/          # 后端 API 调用封装
│   ├── public/
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── tools/                # 工具代码 (从 cmd/ + internal/ 迁移)
│   ├── importer/         # 元数据导入
│   ├── thumbgen/         # 缩略图生成
│   └── embimport/        # 向量导入
├── Dockerfile            # 多阶段构建 (生产)
├── docker-compose.yml    # 生产部署 (仅 shutterseek, 加入已有网络)
├── .github/workflows/
│   └── deploy.yml        # CI/CD 流水线
├── config.yaml           # 应用配置
└── go.mod
```

## 5. 组件设计

### 5.1 后端 (Gin)

**入口点**: `cmd/server/main.go`

- 加载 config.yaml + 环境变量覆盖
- 初始化 PG 连接池 + Redis 客户端
- 注册路由（API + 静态文件）
- 启动 HTTP server (graceful shutdown)

**路由规划** (第一阶段只搭骨架):

```
GET  /api/health          → 健康检查
GET  /api/v1/photos       → 照片列表 (分页)
GET  /api/v1/photos/:id   → 照片详情 (含 EXIF)
GET  /api/v1/search       → 向量相似搜索 (?q=text / ?similar_to=id)
GET  /thumbnails/:id.jpg  → 缩略图静态文件 (Gin.Static)
/*                        → Vue SPA (index.html fallback)
```

**中间件**: CORS, 请求日志, 错误恢复, Rate Limiter (Redis)

**Handler 层**: 只做参数解析和响应序列化，业务逻辑委托给 Service 层。

**Service 层**: 封装 DB 查询、Redis 缓存逻辑。

### 5.2 前端 (Vue + TS)

- **构建工具**: Vite
- **路由**: vue-router (hash mode for SPA)
- **HTTP**: axios 封装
- **UI 框架**: 暂不引入，先用纯 CSS / CSS Modules。后续可加 Tailwind 或组件库。
- **构建产物**: `frontend/dist/` 由 Gin 的 `Static()` 托管
- **开发代理**: Vite dev server proxy `/api` → Gin backend

### 5.3 存储

| 数据 | 存储 | 路径/Key |
|------|------|----------|
| 照片元数据 | PostgreSQL `photos` | — |
| 搜索向量 | PostgreSQL `photo_embeddings` | — |
| 缩略图文件 | 文件系统 | `$THUMBNAILS_DIR/{id}.jpg` |
| Session | Redis | `session:{token}` |
| API 限流 | Redis | `ratelimit:{ip}:{endpoint}` |
| 热点缩略图缓存 | Redis | `thumb:{id}` (base64, TTL 1h) |

### 5.4 配置与密钥管理

**分层覆盖**（优先级从低到高）：

```
config.yaml (默认值, 不含密钥, 可提交 git)
  ← 开发: .env.local (gitignore, 本地密钥)
  ← 生产: 环境变量 (Docker Compose / K8s Secret)
```

**config.yaml** — 仅含非敏感默认值，可安全提交：

```yaml
server:
  port: 8080
  mode: release
database:
  host: postgres-main
  port: 5432
  sslmode: disable
  user: ""           # ← 空, 由环境变量注入
  password: ""       # ← 空, 不提交
  dbname: ""         # ← 空, 不提交
redis:
  host: redis
  port: 6379
  password: ""       # ← 空, 不提交
thumbnail:
  output_dir: /thumbnails/.thumbnails_1080
```

**开发环境** (`.env.local`, gitignore):

```env
SHUTTERSEEK_DB_USER=photo_user
SHUTTERSEEK_DB_PASSWORD=PhotoHyc65319436
SHUTTERSEEK_DB_NAME=photo_search
SHUTTERSEEK_REDIS_PASSWORD=
```

**生产环境** (Docker Compose `environment:`):

```yaml
services:
  shutterseek:
    environment:
      SHUTTERSEEK_DB_USER: ${DB_USER}
      SHUTTERSEEK_DB_PASSWORD: ${DB_PASSWORD}
      SHUTTERSEEK_DB_NAME: ${DB_NAME}
      SHUTTERSEEK_REDIS_PASSWORD: ${REDIS_PASSWORD}
      SHUTTERSEEK_SERVER_MODE: release
```

**GitHub Actions** — 通过 Secrets 注入构建时变量（非运行时密钥）：

| Secret | 用途 |
|--------|------|
| `GHCR_USERNAME` | 推送 Docker 镜像 |
| `GHCR_TOKEN` | GitHub Container Registry |
| `NAS_SSH_KEY` | SSH 到 NAS 部署 |
| `NAS_HOST` | NAS 地址 |

运行时密钥（DB 密码等）通过 NAS 上的 `.env` 文件挂载，不经过 GitHub。

**硬规则**:
- `config.yaml` 中的 `password`/`user`/`dbname` 永远留空，不提交真实值
- 所有密钥只通过环境变量注入
- `.env.local` 在 `.gitignore` 中

`config.yaml` 扩展（新增 server 和 redis 段）:

```yaml
server:
  port: 8080
  mode: debug          # debug | release

database: (已有, 不变)

redis:
  host: redis
  port: 6379
  password: ""
  db: 0

thumbnail:
  output_dir: /thumbnails/.thumbnails_1080   # 可被 THUMBNAILS_DIR 环境变量覆盖
```

环境变量统一前缀 `SHUTTERSEEK_`:
- `SHUTTERSEEK_DB_HOST`
- `SHUTTERSEEK_REDIS_HOST`
- `SHUTTERSEEK_THUMBNAILS_DIR`
- `SHUTTERSEEK_SERVER_PORT`
- etc.

### 5.5 Docker 构建

**多阶段 Dockerfile**:

```
Stage 1: node:20-alpine → 构建 Vue (npm ci && npm run build)
Stage 2: golang:1.24 → 编译 Go (CGO_ENABLED=0)
Stage 3: alpine:3.21 → 运行 (复制 Go binary + Vue dist + 配置文件)
```

最终镜像 <30MB，静态链接，无运行时依赖。

### 5.6 CI/CD

**触发**: push main 分支

**流水线**:
1. Checkout
2. Build Docker image (`shutterseek:latest`, `shutterseek:$GITHUB_SHA`)
3. Push to GitHub Container Registry (`ghcr.io/<user>/shutterseek`)
4. SSH to NAS → `docker compose pull && docker compose up -d`

## 6. 数据流

```
浏览器                     Gin 后端                   PostgreSQL/Redis
  │                          │                           │
  ├─ GET /api/v1/photos ────→│                           │
  │                          ├─ SELECT * FROM photos ───→│
  │                          │←──── rows ────────────────┤
  │←── JSON [] ─────────────┤                           │
  │                          │                           │
  ├─ GET /thumbnails/42.jpg →│                           │
  │                          ├─ Redis GET thumb:42 ─────→│
  │                          │←──── (miss) ─────────────┤
  │                          ├─ os.ReadFile(thumbdir/42.jpg)
  │                          ├─ Redis SET thumb:42 ─────→│
  │←── image/jpeg ──────────┤                           │
```

## 7. 待实现功能（第二阶段+）

- 缩略图网格浏览（虚拟滚动）
- 时间线/文件夹导航
- 向量相似搜索
- 照片详情页（EXIF 展示 + 原始文件下载）
- 管理员界面（重新扫描、缩略图生成触发）

## 8. 规格自审

- [x] 无 TBD/TODO 占位
- [x] 各组件边界清晰
- [x] 环境变量覆盖规则明确
- [x] 开发/生产路径差异已处理
- [x] 工具代码迁移路径已规划
