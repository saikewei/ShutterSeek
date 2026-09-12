# AGENTS.md — ShutterSeek 项目约定

> 给所有 AI 编码助手的**单一事实源**。本文件只写「从代码里看不出来、写错代价高」的约定；
> 技术栈细节、目录结构、快速开始见 `README.md`，此处不重复。
> 保持简短：每多一行，每个会话都要为它付出上下文成本。

## 1. 项目速览

部署在 NAS 上的个人照片管理应用：时间轴浏览 + 相册分享 + 语义搜索（文字搜图、上传图搜图）。
后端 Go + Gin + GORM + pgx + go-redis（`handler → service → model`）；前端 Vue 3 + TS + Vite + Tailwind v4；
PostgreSQL + pgvector（1024 维）存元数据与向量；FastAPI + ONNX Runtime sidecar 产出文本向量，
浏览器 WebGPU fp16 产出图片向量；Docker Compose 部署，GitHub Actions 自动构建与发布。

## 2. 安全边界（最高优先级）

- **生产库默认只读**。`photo_search` 是真实生产库（约 6.7 万张照片、6.7 万条向量）。
  任何 `INSERT / UPDATE / DELETE` 都必须先说明意图、取得用户确认后再执行。
- **DDL 永远只出 SQL**，由用户手动执行。不要自己 `CREATE / ALTER / DROP`。
- **禁止** `TRUNCATE`、`DROP`、无条件批量 `DELETE`。
  注意：权限实际是开着的（`photo_user` 对这些表有完整 DML，含部分表的 `TRUNCATE`）——靠的是约定，不是权限。
- 需要写数据时**优先走 API**（有鉴权、业务校验、缓存失效）；直连 SQL 是最后手段。
- **integration 测试直连生产库**：跑之前 `set -a; source .env.local; set +a`，且测试**必须自清理**数据。
- **密钥**只通过 `SHUTTERSEEK_*` 环境变量 / `.env.local`（已 gitignore）注入：绝不写进代码、`config.yaml`、日志或提交。
- **Git 红线**：**提交只落在 `dev`**；`main` 只接受合并、**禁止直接提交**（由 `scripts/git-hooks/pre-commit` 强制）；
  不推送、不合并 `main`（合并需用户确认）、不改写历史。
- 以下目录**不进 git**：`docs/`、`tmp/`、`models/`、`thumbnails/`、`uploads/`、`certs/`、`.claude/`、`.env*`。
- 不要在 NAS 宿主机上执行 `docker compose` 等改变共享状态的命令——部署由 CI 完成。

## 3. 工作流与提交

- **分支纪律：所有提交都只落在 `dev`；`main` 只接受合并，任何情况下都不要直接往 `main` 提交**
  （合并进 `main` 前需用户确认）。
- 该纪律由仓库内钩子**强制**：`scripts/git-hooks/pre-commit`（靠 `git config core.hooksPath scripts/git-hooks` 生效）
  拒绝 `main` 上的直接 `commit` / `cherry-pick` / `revert` / `amend`，放行合并提交。
  新克隆需执行一次 `git config core.hooksPath scripts/git-hooks`；`--no-verify` 可绕过，但规则不允许。
  本仓库 `core.fileMode=false`：改动该脚本后若 git 把模式记回 `100644`，用
  `git update-index --chmod=+x scripts/git-hooks/pre-commit` 修正，否则克隆方的钩子不生效。
- 一小步一提交，消息前缀：`feat | fix | perf | test | ci | style | docs | chore`。
- 完成一个小功能/修复后即可提交，**但不要推送**（push `main` 会触发 CI 部署）。
- CI（`.github/workflows/deploy.yml`）：push `main` → 跑单测 → 构建并推送主镜像与 sidecar 镜像到 GHCR
  → Tailscale → NAS 上 `docker compose pull && docker compose up -d`。

## 4. 验收命令（改完必须跑）

| 场景 | 命令 |
|---|---|
| 后端编译 | `go build ./...` |
| 后端单测（与 CI 等价） | `go test -count=1 ./cmd/... ./internal/...` |
| 前端单测 + 构建 | `cd frontend && npm test && npm run build`（`build` 已含 `vue-tsc -b`） |
| integration（连生产库、自清理） | `set -a; source .env.local; set +a; go test -tags integration ./internal/...` |

## 5. 架构不变量（改代码时不能破）

- 分层 `handler → service → model`；**handler 不得直接访问 DB/Redis**（无测试守护，靠自觉）。
  前端对应 `views/`（页面）、`components/`（复用组件）、`api/`（后端封装）。
- 复杂或性能敏感查询用**裸 SQL 写在 service 层**（pgvector、日期聚合、range、`SET LOCAL`）；常规 CRUD 用 GORM。
- 角色 `admin` / `guest`：guest 的可见性在 **SQL 层**限定 `is_public`（不是 UI 层）；公开路由仅
  `POST /auth/login`、`POST /invites/redeem`、`GET /invites/validate/:code`，其余经 `AdminOnly()` 守卫。
- JWT 存 HttpOnly cookie `shutterseek_token`（`path=/api`、30 天）；撤销靠比较 `iat` 与 `users.updated_at`（1s 容差）。
  启动必需 `SHUTTERSEEK_JWT_SECRET`；库里没有 admin 时才用 `SHUTTERSEEK_ADMIN_PASSWORD` 播种初始管理员。
- HTTPS 可选：`SHUTTERSEEK_TLS_ENABLED=true` → certmagic + Let's Encrypt **DNS-01**（阿里云 DNS，因 80/443 不可用）；
  开启后 cookie 置 `Secure`，未开启走明文 HTTP。
- 缓存 key / TTL / 失效（**只有第一页进缓存**）：`cache:first_page:`(60s)、`cache:total_photos`(5m)、`cache:photo_dates:`(5m)、
  `cache:albums:`(60s)、`cache:album_dates:`、`cache:album_photos:`、`cache:qvec:v1:`(7d)。
  guest 的数据**一律加 `guest:` 前缀**，绝不跨角色共享。
  **改查询参数或响应格式后必须清缓存**；切换相册 `is_public` 必须走 `AlbumService.InvalidateCaches()`。
- 时区固定 **+08**（`cstZone = time.FixedZone("CST", 8*3600)`）：日期解析与格式化统一走它，不要用 `time.Local`。
- 分页是游标式：`(taken_at, id) < (?, ?)` 配 `taken_at DESC, id DESC`；相册列表按 `sort_order, id`。

## 6. 开发环境（dev 容器内的事实）

- 依赖服务：`postgres-main:5432`（库 `photo_search`、用户 `photo_user`）、Redis `172.18.0.3:6379` DB 2
  （compose 网络内为 `redis:6379`）；凭据在 `.env.local`。
- 三个进程与端口：后端 `:8080`（`air` 热重载）、文本向量 sidecar `:8000`（`./embed/run_dev.sh`）、
  Vite `:5173`（把 `/api`、`/thumbnails`、`/models` 代理到 8080）。
- 模型文件：`models/model.onnx`（BGE-M3 INT8，文本）、`models/vision_encoder/model.onnx`（fp16 1.2GB，浏览器下载）。
- 容器内**没有** `psql`、`redis-cli`、`rg`、`docker` CLI、`cwebp`；其中 `cwebp` 是上传缩略图的依赖，
  在 dev 容器里是手工装的、重建后会丢失（**生产镜像**已含 `libwebp-tools`，不受影响）。
- 因此查数据库用 **Go 探针**：在 `tmp/` 写小 `main.go`，复用 `internal/config` + `internal/db` 与 `.env.local`
  （已有样例：`tmp/envcheck`、`tmp/dbcheck`）。

## 7. 已知问题（尚未修复；修好一条就删一条）

- PNG 上传**必然生成不出缩略图**：`internal/service/upload.go` 只注册了 `image/jpeg`，`image.Decode` 对 PNG
  返回 `unknown format`，exiftool 兜底又返回 0 字节。
- 静态文件缺失会被 SPA fallback 成 **200 + index.html**（缺缩略图时不返回 404，掩盖问题）。
- GORM debug 模式会把**整条 1024 维向量**打进慢 SQL 日志（>200ms 触发）。
- `GET /api/v1/photos/:id/original` 对 PNG 返回 `Content-Type: image/jpeg`（`internal/handler/handler.go:157` 硬编码）。
- 文本向量（BGE-M3）与图片向量（CLIP）**不在同一向量空间**，是已知的工程取舍。

## 8. 文档地图（`docs/` 不进 git，仅本地参考）

- `docs/handoff-2026-08-27.md` — 交接文档与红线（最新）
- `docs/project-notes-2026-08-01.md` — 核心架构约定与历史排查结论
- `docs/superpowers/specs/`（16 篇设计文档）、`docs/superpowers/plans/`（8 篇实现计划）
