# AGENTS.md — ShutterSeek 项目约定

> 给所有 AI 编码助手的**单一事实源**。本文件只写「从代码里看不出来、写错代价高」的硬约束；
> 技术栈、目录结构、快速开始见 `README.md`。
>
> 历史事故、排查过程、缓存 TTL、前端滚动/弹窗细节、上传实现细节、已知问题的完整清单，
> 已移到 `docs/agent-details-2026-11-23.md`（本地，不入 git）。需要时再读，不要把所有细节
> 重新塞回本文件。
>
> 保持简短：每多一行，每个会话都要为它付出上下文成本。

## 1. 项目速览

部署在 NAS 上的个人照片管理应用：时间轴浏览 + 相册分享 + 语义搜索（文字搜图、上传图搜图）。
后端 Go + Gin + GORM + pgx + go-redis（`handler → service → model`）；前端 Vue 3 + TS + Vite + Tailwind v4；
PostgreSQL + pgvector（1024 维）；FastAPI + ONNX Runtime sidecar 产文本向量，浏览器 WebGPU fp16 产图片向量；
Docker Compose 部署，GitHub Actions 自动构建与发布。

## 2. 安全边界（最高优先级）

- **生产库默认只读**。`photo_search` 是真实生产库（约 6.7 万张照片、6.7 万条向量）。
  任何 `INSERT / UPDATE / DELETE` 都必须先说明意图、取得用户确认后再执行。
- **DDL 永远只出 SQL**，由用户手动执行。不要自己 `CREATE / ALTER / DROP`。
- **禁止** `TRUNCATE`、`DROP`、无条件批量 `DELETE`。
  权限实际是开着的（`photo_user` 有完整 DML，部分表有 `TRUNCATE`）——靠的是约定，不是权限。
- 需要写数据时**优先走 API**（有鉴权、业务校验、缓存失效）；直连 SQL 是最后手段。
- **integration 测试直连生产库**：跑之前 `set -a; source .env.local; set +a`，且测试**必须自清理**；
  跑之前先向用户说明并取得确认。
- **密钥**只通过环境变量 / `.env.local`（已 gitignore）注入：绝不写进代码、`config.yaml`、日志或提交。
- **删照片是「三处一起删」**：原图、`photos` 行、缩略图 `{id}.webp`。
  文件不在事务里，最容易漏；历史事故见 details。
- 以下目录**不进 git**：`docs/`、`tmp/`、`models/`、`thumbnails/`、`uploads/`、`certs/`、`.claude/`、`.env*`。
- 部署默认由 CI 完成；不要在 NAS 宿主机上手动 `docker compose` 改变线上状态。
  仅当 CI 不可用时作为例外，且事后说明改了什么、怎么验证。

## 3. 工作流与提交

- **提交只落在 `dev`**；`main` 只接受合并、**禁止直接提交**；**合并/推送 `main` 必须用户确认**；
  **不改写历史**。`dev` 自查无敏感信息后可直接推。新 clone 先执行：
  ```bash
  git config core.hooksPath scripts/git-hooks
  ```
- 一小步一提交，消息前缀：`feat | fix | perf | test | ci | style | docs | chore`。
- **注释性文字一律英文**：代码注释、提交信息（subject 与 body）都必须写英文。
  `scripts/git-hooks/commit-msg` 拒绝非 ASCII 提交信息；`pre-commit` 拒绝新增 `*.go` 注释行里的非 ASCII。
  Go 里的中文字符串字面量是 UI 文案，不受限；UI 文案、`docs/`、本文件仍用中文。
  历史提交是中文的**不追改**；改到哪就顺手译到哪。
- 提交身份固定 `saikewei <saikewei27@gmail.com>`。
- **推送**：开发容器到 `github.com:443` 受限，优先用宿主机代理
  `http://172.18.0.1:7890`；Mac bundle 中转、钩子实现、远端 ref 核对细节见 details。
- CI（`.github/workflows/deploy.yml`）：push `main` → Go/前端/Python/浏览器验收 →
  构建三镜像（app / embedding / agent）→ 运行时验收 → GHCR + 阿里云 ACR → Tailscale →
  上传冻结 Compose → NAS 拉同一 SHA 镜像并等待健康。

## 4. 验收命令（改完必须跑）

| 场景 | 命令 |
|---|---|
| 后端编译 | `go build ./...` |
| 后端单测（与 CI 等价） | `go test -count=1 ./cmd/... ./internal/...` |
| 前端单测 + 构建 | `cd frontend && npm test && npm run build`（`build` 已含 `vue-tsc -b`） |
| integration（连生产库、自清理） | `set -a; source .env.local; set +a; go test -tags integration ./internal/...`。包之间**并行且共享同一个库**：会临时造照片的测试（上传类）可能让别的包取到"刚被删的最新照片"，跨包断言要靠重试/多候选兜底 |

## 5. 架构不变量（改代码时不能破）

- 分层 `handler → service → model`；**handler 不得直接访问 DB/Redis**（无测试守护，靠自觉）。
  前端对应 `views/`（页面）、`components/`（复用组件）、`api/`（后端封装）。
  管理台入口 `GET /api/v1/admin/stats`（`AdminOnly()`，`StatsService.Snapshot`，**永不返回 error**）。
- 复杂或性能敏感查询用**裸 SQL 写在 service 层**（pgvector、日期聚合、range、`SET LOCAL`）；
  常规 CRUD 用 GORM。
- 角色 `admin` / `guest`：guest 可见性在 **SQL 层**限定 `is_public`（不是 UI 层）；公开路由仅
  `POST /auth/login`、`POST /invites/redeem`、`GET /invites/validate/:code`；
  照片浏览、创作和分享要求登录，管理和上传经 `AdminOnly()` 守卫。
  创作按 `user_id` 隔离；guest 仅使用 SQL 限定的公开相册照片；**禁止匿名分享**。
- **缩略图也走鉴权**：`GET /api/v1/thumbnails/<id>.webp`（`handler.Thumbnail`），URL 由
  `service.ThumbnailURL(id)` 统一产出，guest 再套 `PhotoInPublicAlbum`。
  **绝不要**把它改回 `r.Static` 或增加公开别名。
- JWT 存 HttpOnly cookie `shutterseek_token`（`path=/api`、30 天）；撤销靠比较 `iat` 与
  `users.updated_at`（1s 容差）。启动必需 `SHUTTERSEEK_JWT_SECRET`；
  库里没有 admin 时才用 `SHUTTERSEEK_ADMIN_PASSWORD` 播种初始管理员。
- **TLS 只在项目外的 nginx 边缘终止**：应用只监听 8080 明文，并发布到宿主机
  `127.0.0.1:${SHUTTERSEEK_APP_PORT:-8080}`；外部 nginx（不在本仓库/Compose 中）负责 8443、
  证书与续期，并反代到这个 loopback 端口。应用侧由 `SHUTTERSEEK_COOKIE_SECURE=true` 置
  cookie `Secure`，`SHUTTERSEEK_TRUSTED_PROXIES` 默认只信任 `127.0.0.1`。
  **不要**再把 certmagic/TLS 监听或 nginx 服务加回应用。
- 缓存：**只有第一页进缓存**；guest 数据一律加 `guest:` 前缀；改查询参数或响应格式后必须清缓存；
  切换相册 `is_public` 必须走 `AlbumService.InvalidateCaches()`；
  `cache:admin_stats:v1` 只缓存计数；健康检查与运行时长不进缓存。
  具体 key/TTL 见 details。
- 时区固定 **+08**（`cstZone = time.FixedZone("CST", 8*3600)`）：日期解析与格式化统一走它，
  不要用 `time.Local`。
- 前端两套 shell，靠媒体查询切换（`stores/device.ts` 的 `isMobileShell`，不看 UA）。
  移动端滚动、读取/写入滚动位置、贴顶高度**一律走 `lib/scrollHost.ts` / `lib/chrome.ts`**；
  不要再写 `document.querySelector('.overflow-auto')` 或 37/46/53 这类魔数。
  弹窗用 `components/AppModal.vue`，触屏面板用 `components/ActionSheet.vue`；
  照片网格**没有长按菜单**；触屏没有 hover/可靠 contextmenu。详细坑见 details。
- 分页是游标式：`(taken_at, id) < (?, ?)` 配 `taken_at DESC, id DESC`；相册列表按 `sort_order, id`。
- 上传：批量入口 `POST /photos/upload/batch`（multipart `file_N`/`vector_N`/`preview_N`，下标配对；
  `upload_handler.go` 用 `MultipartReader` 流式落盘，不走 `ParseMultipartForm`）。
  缩略图**长边 1080**，由 `upload_thumb.go` 三级降级生成；重活受**进程级**闸门
  `SHUTTERSEEK_UPLOAD_WORKERS`(2) 限流。查重靠 `photos.file_hash`：
  事务内 `pg_advisory_xact_lock` + 唯一索引。细节见 details。

## 6. 开发环境（dev 容器内的事实）

- 依赖服务：`postgres-main:5432`（库 `photo_search`、用户 `photo_user`）、Redis
  `172.18.0.3:6379` DB 2（compose 网络内 `redis:6379`）；凭据在 `.env.local`。
- 三个进程与端口：后端 `:8080`（`air` 热重载）、文本向量 sidecar `:8000`
  （`./embed/run_dev.sh`）、Vite `:5173`（把 `/api`、`/models` 代理到 8080）。
- **编译/测试在开发容器内做**，不要在 NAS 宿主机装 Go/Node：
  ```bash
  docker exec epic_mirzakhani bash -lc 'cd /workspaces/ShutterSeek && go build ./...'
  ```
  开发容器内没有 `docker`、`psql`、`redis-cli`、`rg`；查数据库用 `tmp/` 下的 Go 探针。
  `cwebp` 当前存在，但可能是手工装的，重建容器后会丢失。
- **dev 与生产共用同一个库、同一批文件**：缩略图在
  `/volume1/docker/ShutterSeek/thumbnails`；上传目录必须指 `/photos_uploads`
  （dev 容器把 `/volume1/Main/Photos/uploads` 挂在这里，**别写仓库内 `uploads/`**）。
  改完 `.env.local` 要**重启 air**。
- **dev 不运行外部 nginx 边缘**：8080/5173 是明文 HTTP，`SHUTTERSEEK_COOKIE_SECURE` 必须为 false；
  生产由项目外的 nginx 终止 TLS 并设为 true。
- 手机预览优先 8080；5173 的缩略图代理不可信。详细说明和不要写 TCP 中继的原因见 details。

## 7. 已知问题

完整清单已移到 `docs/agent-details-2026-11-23.md` §5；修好一条就删一条。

最容易误判的两条：

- 静态文件缺失或未注册的 `/api/...` 路由可能被 SPA fallback 成 **200 + index.html**，
  判断路由要看 `Content-Type` 或后端 `[GIN-debug]` 路由表。
- GORM debug 会把整条 1024 维向量打进慢 SQL 日志。

## 8. 文档地图

- `README.md` — 项目结构、快速开始、部署、外部 TLS 接口
- `docs/agent-details-2026-11-23.md` — 本文移出的历史、缓存 TTL、前端/上传细节、已知问题
- `docs/handoff-2026-11-23.md` — 当前环境、容器/SSH、生产拓扑、stash、交接清单
- `docs/environment-audit-2026-10-10.md` — 环境/API 实测快照（早于 nginx 拆分）
- `docs/handoff-2026-08-27.md` — 旧交接，已过时，只当历史
- `docs/project-notes-2026-08-01.md` — 历史架构与 user_logs 慢 SQL 排查
- `docs/superpowers/specs/`、`docs/superpowers/plans/` — 设计与实现计划
