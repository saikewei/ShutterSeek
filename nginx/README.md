# nginx 边缘（TLS + acme.sh）

这里是一个独立的 nginx 容器，负责：

- 在 **8443** 终止 TLS（宿主机 80/443 已被其他服务占用）
- 用 **acme.sh + 阿里云 DNS-01** 申请/续期证书，不依赖应用内置的 certmagic
- 反向代理 ShutterSeek 的内部 `shutterseek:8080`
- 为未来其他 web 服务提供共享的 TLS 入口

拓扑：

```text
浏览器 :8443
   │ HTTPS
   ▼
shutterseek-nginx (TLS 终止 + 证书)
   │ HTTP (shutterseek-proxy 网络)
   ▼
shutterseek:8080
```

## 持久化目录

宿主机默认 `/volume1/docker/ShutterSeek/nginx/`（`SHUTTERSEEK_NGINX_DIR`）：

| 目录 | 用途 |
|---|---|
| `acme/` | acme.sh 账户、域名配置、续期状态；含阿里云凭证，权限 700 |
| `certs/` | `fullchain.pem` + `privkey.pem`，供 nginx 读取 |
| `locations/` | 按路径挂到主站点的 `location` 片段（`*.conf`） |
| `servers/` | 额外的 `server {}` 虚拟主机（`*.conf`） |

`locations/` 和 `servers/` 是只读挂载；修改后执行：

```bash
docker exec shutterseek-nginx nginx -s reload
```

## 增加其他服务

### 按路径复用同一个域名和证书（推荐）

在 `/volume1/docker/ShutterSeek/nginx/locations/other.conf` 写入：

```nginx
location /other/ {
    set $other_upstream http://other-service:8080;
    proxy_pass $other_upstream;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

其他服务需要加入 `shutterseek-proxy` 网络，或者使用
`http://host.docker.internal:<宿主机端口>`（compose 已加 `host-gateway`）。
`proxy_pass` 使用变量是为了让 Docker DNS 每次请求重新解析，容器重建后不需要重启 nginx。

### 额外子域名

在 `servers/other.conf` 增加 `server { listen 8443 ssl; server_name ...; ... }`。
当前证书默认只签 `SHUTTERSEEK_DOMAIN`；新增子域名需要让证书覆盖它（例如改成通配符
`*.saikewei.tech`，仍然用 DNS-01），否则浏览器会报证书不匹配。按路径代理则不需要新证书。

## 证书

首次启动：

1. 如果 `certs/fullchain.pem` 和 `certs/privkey.pem` 不存在，entrypoint 会先同步执行
   `acme.sh --issue --dns dns_ali`，失败则容器不启动（避免用无效证书对外服务）。
2. 如果已有证书（例如从旧 certmagic 目录迁移），nginx 先启动；renew 脚本随后在后台
   重新签发/接管，成功后会 `nginx -s reload`。

手动检查/续期：

```bash
docker exec shutterseek-nginx /usr/local/bin/renew-certs.sh
docker exec shutterseek-nginx tail -f /var/log/nginx/error.log
```

默认每 12 小时检查一次（`ACME_RENEW_INTERVAL=43200`）。acme.sh 会根据自己的续期窗口
决定是否真正续期，不会每次检查都重新签发。

联调时把 `ACME_SERVER` 设为 `letsencrypt_test`，并**使用独立的 `SHUTTERSEEK_NGINX_DIR`**，
避免 staging 证书/账户状态污染生产目录。

## 从旧 certmagic 证书迁移

旧证书在 `/volume1/docker/ShutterSeek/certs/certificates/...`，可以在首次启动 nginx 前复制：

```bash
mkdir -p /volume1/docker/ShutterSeek/nginx/certs
cp <旧目录>/nas.saikewei.tech.crt /volume1/docker/ShutterSeek/nginx/certs/fullchain.pem
cp <旧目录>/nas.saikewei.tech.key /volume1/docker/ShutterSeek/nginx/certs/privkey.pem
chmod 600 /volume1/docker/ShutterSeek/nginx/certs/*
```

这样切换时 nginx 立即有可用证书；后台 acme.sh 接管成功后会替换它。旧目录不要删除，
等新证书续期成功并验证后再清理。

## 注意事项

- 阿里云凭证通过 `Ali_Key` / `Ali_Secret` 注入；acme.sh 会写入 `acme/account.conf`，
  因此该目录必须保持 700，不要提交或打包。
- nginx 的 `/nginx-health` 只返回 `ok`，不经过应用；compose 用它做健康检查。
- 生产环境 `SHUTTERSEEK_COOKIE_SECURE=true`；应用通过 `X-Forwarded-Proto` 之外的显式环境变量
  决定 cookie 的 `Secure` 标志，避免 TLS 终止在边缘时退化成非 Secure。
- 关闭面板/外部访问时，只需要停止 `shutterseek-nginx`；应用内部 8080 不再直接暴露。
