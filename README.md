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

### AI 网页创作（默认关闭）

管理员可从相册或选图进入，单个作品 1–200 张照片。先回答或跳过最多三个问题，再确认结构、风格和选图；
生成与每次修改都在隔离浏览器中执行桌面、手机截图审查。首次审查后最多修复两轮；
未通过时仍保存候选版本，页面显示问题、截图和实际检查范围。

数据库迁移见 [scripts/sql/creations-v1.sql](scripts/sql/creations-v1.sql)，必须由用户手动执行。
应用不会自动建表。迁移包含 photo_user 的表和事件序列授权；已执行旧版 SQL 的环境需另外手动执行 [creations-v1-grants.sql](scripts/sql/creations-v1-grants.sql)。生产数据库与文件仍遵循 AGENTS.md 的只读约定。

开发环境在开发容器中使用：
```sh
python3 -m venv /tmp/shutterseek-agent-venv
/tmp/shutterseek-agent-venv/bin/pip install -r agent/requirements.txt
/tmp/shutterseek-agent-venv/bin/playwright install --with-deps chromium
/tmp/shutterseek-agent-venv/bin/python -m unittest discover -s agent/tests -v
```

配置仅通过环境变量注入；密钥不要放在 config.yaml 或提交里：

| 变量 | 默认 / 用途 |
|---|---|
| SHUTTERSEEK_CREATIONS_ENABLED | false；Go 功能开关 |
| SHUTTERSEEK_AGENT_ADDRESS | 127.0.0.1:50051；Go 到 Agent 的 gRPC 地址 |
| SHUTTERSEEK_CREATIONS_RPC_LISTEN | 127.0.0.1:8081；Go 回调监听 |
| SHUTTERSEEK_CREATIONS_WORKSPACE_ADDRESS | shutterseek:8081；Agent 到 Go 的地址，开发设为 127.0.0.1:8081 |
| SHUTTERSEEK_CREATIONS_MODEL_KEY | 必需；模型密钥 |
| SHUTTERSEEK_CREATIONS_MODEL | deepseek-flash |
| SHUTTERSEEK_CREATIONS_MODEL_URL | https://api.deepseek.com |
| COMPOSE_PROFILES | 启用生产第三容器时设为 creations |

开发启动 Agent：`cd agent && /tmp/shutterseek-agent-venv/bin/python server.py`。
启用 Go 前，先检查手动迁移和模型配置；开发与生产共库，不要为测试创建生产作品。
真实模型离线验收可运行 `python agent/tests/live_review.py`，不连接数据库，不创建作品；
它会产生受控模型调用费用，报告与截图存放在被忽略的 tmp/ 下。

部署仍由 main 的 CI 完成，新增独立 Agent 镜像缓存和 ACR 同步。不要手动改变 NAS 线上容器。
Agent 无数据库凭据、无照片目录挂载、无应用 Cookie；模型并发 5，浏览器公平队列并发 1。
运行凭证只存 SHA-256 摘要，绑定运行、用户、作品与最多 200 张照片，31 分钟失效；
重启会重新发放凭证。检查点和 pending writes 由 Go 存储，仅含文本和图片/截图引用。

预览接口使用 CSP `sandbox allow-scripts`，页面内只允许内联脚本/样式、data 图片/字体；
可信外层用 frame-src about: 约束生成页自身的导航；不允许外部网络、同源权限、表单、弹窗或顶层导航。SSE 只读取已持久化事件，
断线不取消；重新进入作品继续订阅。恢复与提交校验基础版本，运行提交按 run_id 幂等。
