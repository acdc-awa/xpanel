# XrayPanel

<div align="center">

**主控 - 节点 - 用户三层架构的代理节点管理面板**
*Go 1.27 + Vue 3.5 + Xray-core v26.10.31*

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Vue Version](https://img.shields.io/badge/Vue-3.5+-4FC08D?style=flat&logo=vuedotjs)](https://vuejs.org)
[![Xray-core](https://img.shields.io/badge/Xray--core-v26.10.31-red?style=flat)](https://github.com/XTLS/Xray-core)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)

</div>

---

## 项目简介

XrayPanel 是一套自托管的代理节点管理系统：主控负责 Web 管理端、Xray 配置生成、订阅服务与节点编排；节点 Agent 托管 Xray-core 进程，经 WSS 长连接接收配置下发、上报心跳与流量；用户凭订阅连接接入点。支持 VLESS（TCP / XHTTP）、Hysteria 2 与四层直通，并提供可视化拓扑编排、权限组订阅模板与余额计费。

> 节点 Agent 是独立仓库 [XPanel-Node](https://github.com/acdc-awa/XPanel-Node)：二进制与安装脚本从其 Releases 发布，跨仓库协议包 `pkg/protocol` 单源托管在那边、本仓库经 go.mod 引入。

```
用户 ──TLS──▶ 自备反代（Caddy / Nginx：TLS 终止 + 域名/路径分流）
                   │ 127.0.0.1:18080 面板 / 18082 节点 WS / 6000 订阅
                   ▼
        ┌──────────────────────────────────────┐
        │  Master 主控（Docker）               │
        │  SPA + API │ 订阅服务 │ 节点 WS 网关 │
        └──────────────────┬───────────────────┘
                           │ WSS：心跳 / 流量上报 / 配置下发
                           ▼
        ┌──────────────────────────────────────┐
        │  Agent 节点（每台 VPS 一个）         │
        │  xray-agent + Xray-core v26.10.31    │
        └──────────────────────────────────────┘
```

---

## 核心特性

- **协议**：VLESS（TCP + REALITY/Vision；XHTTP 适配 CDN 与反代分流）、Hysteria 2（QUIC），以及承接任意 TCP/UDP 转发的四层直通（tunnel）；
- **安全层四选一**：REALITY / TLS / VLESS Encryption / none；
- **拓扑编排**：拖拽画布管理出站链，直-弧-直动态避让、DAG 自动排版，布局持久化；
- **订阅**：VLESS 链接与 Clash 配置，支持模板库与 `$PROXIES$` / `$FILTER_PROXIES(regex)`；
- **计费运营**：礼品卡余额直付、权限组与套餐、订阅到期自动退订、审计日志、定时备份；
- **安全**：JWT 密钥首启自动生成，初始管理员随机密码仅输出一次且首登强制改密，支持 TOTP、Turnstile 与登录防爆破；
- **部署韧性**：面板域名与节点通信端点分离，Agent 多候选端点自动回退；出站可开 PROXY Protocol v2 透传真实客户端 IP。

---

## 快速部署（主控）

生产推荐 Docker Compose 形态：release 包含二进制与前端产物，解压改配置即可启动，数据落在安装目录的 `data/`（SQLite + 备份）。TLS 终止与域名分流由你自备的反代承担（参考包内 `Caddyfile.reference`）。

### 1. 安装

```bash
# 在线一键（取最新 release，解压到当前目录）
curl -fsSL https://raw.githubusercontent.com/acdc-awa/xpanel/master/deploy/master/install.sh | bash

# 或下载 release 包后
bash install.sh --dir /opt/xray-panel
```

常用参数：`--dir` 安装目录、`--version` 钉版本、`--file` 本地包离线安装、`--mirror` GitHub 代理前缀、`--fresh` 全新覆盖（默认保留已有配置与数据）；完整列表见 `bash install.sh --help`。

重复运行即升级：替换二进制与前端产物（编排模板同步刷新），已有的 `configs/`、`.env` 与 `data/` 保留。

### 2. 配置

编辑 `configs/config.yaml`（应用配置唯一入口），至少填写面板公网地址；JWT 密钥与管理员账密留空即可，首次启动自动生成：

```yaml
app:
  env: prod
  public_url: https://panel.yourdomain.com
  # ws_public_url: wss://ws.yourdomain.com/node/ws   # 可选；缺省用面板域名 + /node/ws
```

三端口：`18080` 面板（SPA + API，含 `/healthz` `/readyz`）、`18082` 节点 WS 网关（`/node/ws`）、`6000` 订阅。默认只绑 `127.0.0.1`，改 `.env` 的 `BIND_ADDR=0.0.0.0` 才对全网卡开放。

### 3. 启动

```bash
docker compose up -d
```

首次启动自动构建运行时镜像（仅运行时依赖，不随版本变化）。控制台会输出**仅显示一次**的随机管理员密码，首次登录强制改密。

### 4. 反向代理

用包内 `Caddyfile.reference` 按域名/路径分流三个端口（`/node/ws` 规则须先于默认反代匹配）：

```bash
docker run -d --name caddy -p 80:80 -p 443:443 \
  -v /opt/xray-panel/Caddyfile.reference:/etc/caddy/Caddyfile:ro \
  -v caddy-data:/data -v caddy-config:/config \
  -e SITE_ADDRESS=panel.yourdomain.com \
  -e SUB_SITE_ADDRESS=sub.yourdomain.com \
  caddy:2-alpine
```

面板只采信**来源命中 `app.trusted_proxies` 的** `X-Forwarded-For`（`CF-Connecting-IP` / `X-Real-IP` 不再直接信任），默认仅回环。Docker 部署时来源是网桥网关（常见 `172.18.0.1`），必须加入 `trusted_proxies`；反代侧还需用真实对端**覆盖**而非追加 XFF，否则限流与审计会把所有人记成同一 IP。

---

## 接入节点

1. 管理后台 →「服务器」→「新增服务器」，填写名称与公网 IP；
2. 点击该服务器的「安装命令」复制指令；
3. 在节点 VPS 以 `root` 执行：

```bash
bash <(curl -fsSL https://github.com/acdc-awa/XPanel-Node/releases/latest/download/install-agent.sh) \
  --master wss://panel.yourdomain.com/node/ws \
  --node-id 1 \
  --secret sec_xxxxxxxxxxxxxxxx
```

脚本按 release `checksums.txt` 强制校验 sha256，安装 `xray-agent` 与锁定的 Xray-core v26.10.31，写入 systemd 后自动上线。离线与镜像场景用 `--agent-file` / `--agent-mirror` / `--agent-version`。

---

## 日常运维

```bash
# 重置管理员密码（默认首个管理员，-email 可指定）
docker compose exec master /app/master reset-admin
docker compose exec master /app/master reset-admin -password "MyNewPass2026#!"

# 升级面板：管理后台 →「系统设置 → 检查更新」（sha256 校验 + 自检 + 失败自动回滚）
#         或直接重跑安装脚本：cd /opt/xray-panel && bash install.sh
```

升级只替换二进制与前端产物，配置与数据不动。**数据库 schema 是有版本的**：跨 `DBMinCompatibleVersion` 升级后不能只回退二进制，必须同时换回升级前的数据库备份（并删除 `panel.db-wal` / `panel.db-shm`）；面板内「备份恢复」只回数据、不回版本。**更新前先手动备份。**

节点侧常用命令：`xray-agent status`、`restart`、`logs -n 100`、`upgrade`、`uninstall`。

---

## 本地开发

依赖 Go 1.27+、Node.js 22+。两个仓库同级放置，并用 `go work init` 并入同一工作区，本地改 agent 的 `pkg/protocol` 才会立即生效（只克隆本仓库也可，go.mod 会拉取已发布的协议包）：

```bash
git clone https://github.com/acdc-awa/xpanel.git
git clone https://github.com/acdc-awa/XPanel-Node.git
go work init ./xpanel ./XPanel-Node
```

```bash
# 后端
go run ./cmd/master -config configs/config.example.yaml
go test ./...
go build -o bin/master ./cmd/master

# 前端（端口 5173）
cd web && npm install && npm run dev
npm run typecheck
npm run build
```

---

## 技术栈

| 层次 | 技术 |
|---|---|
| 后端 | Go 1.27 + Gin，单二进制分发 |
| 持久层 | GORM + SQLite（WAL；开发与生产同构） |
| 协议引擎 | Xray-core v26.10.31（锁定）：VLESS / Hysteria 2 / 四层直通 |
| 前端 | Vue 3.5 + Vite + TypeScript + Pinia |
| UI | Cloudscape 移植组件库（`Cs*`） |
| 拓扑画布 | @vue-flow/core |
| 认证 | JWT（HMAC-SHA256）+ Argon2id + TOTP |

---

## 许可证

本项目基于 [AGPL-3.0](LICENSE) 开源，Copyright (C) 2026 acdc-awa：无论以二进制还是网络服务形式提供给他人，都必须以相同协议开放完整源码。捆绑的 Xray-core 遵循 MPL-2.0。如需商业授权，请联系作者。
