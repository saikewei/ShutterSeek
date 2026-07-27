# ShutterSeek

智能照片相册，支持元数据浏览、向量相似搜索。

## 技术栈

| 层 | 技术 |
|---|------|
| 后端 | Go + Gin |
| 数据库 | PostgreSQL + pgvector |
| 缓存 | Redis |
| 前端 | Vue 3 + TypeScript + Vite |
| 部署 | Docker Compose (NAS) |

## 快速开始（开发）

```bash
# 1. 安装依赖
go mod tidy
cd frontend && npm install && cd ..

# 2. 配置密钥
cp .env.local.example .env.local
# 编辑 .env.local 填入数据库和 Redis 连接信息

# 3. 启动后端 (终端1)
go run ./cmd/server/

# 4. 启动前端 (终端2)
cd frontend && npm run dev

# 5. 浏览器打开 http://localhost:5173
#    Vite 会将 /api 请求代理到后端 :8080
```

## 项目结构

```
ShutterSeek/
├── cmd/server/             # Gin 应用入口
├── internal/
│   ├── config/             # 配置（yaml + env）
│   ├── db/                 # PostgreSQL 连接池
│   └── redis/              # Redis 客户端
├── frontend/               # Vue 3 + TypeScript
│   └── src/
│       ├── views/          # 页面组件
│       ├── components/     # 可复用组件
│       └── api/            # 后端 API 封装
├── tools/                  # 工具脚本
│   ├── importer/           # 照片元数据导入
│   ├── thumbgen/           # 缩略图生成
│   └── embimport/          # 向量导入
├── Dockerfile              # 多阶段构建
├── docker-compose.yml      # 生产部署
└── config.yaml             # 应用配置（不含密钥）
```

## 配置

`config.yaml` 存放非敏感默认值，密钥通过环境变量注入：

| 环境变量 | 说明 |
|----------|------|
| `SHUTTERSEEK_DB_USER` | 数据库用户名 |
| `SHUTTERSEEK_DB_PASSWORD` | 数据库密码 |
| `SHUTTERSEEK_DB_NAME` | 数据库名 |
| `SHUTTERSEEK_REDIS_HOST` | Redis 地址 |
| `SHUTTERSEEK_REDIS_PASSWORD` | Redis 密码 |
| `SHUTTERSEEK_THUMBNAILS_DIR` | 缩略图目录 |

开发环境将变量写入 `.env.local`（已 gitignore），生产环境通过 Docker Compose 注入。

## 部署

```bash
# 构建并推送镜像 (GitHub Actions 自动执行)
docker build -t ghcr.io/shutterseek/shutterseek:latest .

# NAS 上启动
docker compose up -d
```

## 数据库

- `photos` — 照片元数据（75,400 条）
- `photo_embeddings` — 搜索向量 1024 维（74,577 条）

## 工具

```bash
# 元数据导入
go run ./tools/importer/ --config config.yaml

# 缩略图生成
go run ./tools/thumbgen/ --limit 5000 --workers 2

# 向量导入
go run ./tools/embimport/
```
