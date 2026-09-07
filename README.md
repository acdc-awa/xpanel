# XrayPanel

<div align="center">

**主控 - 节点 - 用户三层架构的代理节点管理面板**
*Go 1.26 + Vue 3.5 + Xray-core v26.6.27*

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Vue Version](https://img.shields.io/badge/Vue-3.5+-4FC08D?style=flat&logo=vuedotjs)](https://vuejs.org)
[![Xray-core](https://img.shields.io/badge/Xray--core-v26.6.27-red?style=flat)](https://github.com/XTLS/Xray-core)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](https://docker.com)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?style=flat&logo=typescript)](https://www.typescriptlang.org)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)

</div>

---

## 项目简介

XrayPanel 是一套自托管的代理节点管理系统，采用「主控（Master） - 节点（Agent） - 用户端」三层架构：主控负责 Web 管理界面、配置生成、订阅服务与节点编排；节点 Agent 托管 Xray-core 进程，经 WebSocket 长连接接收配置下发、上报心跳与流量；用户端通过订阅链接获取节点信息。系统支持 VLESS 协议下 TCP / XHTTP / WS 传输与 REALITY / TLS / vlessenc 安全层的组合配置，并提供可视化拓扑编排、权限组订阅模板、余额与礼品卡计费等运营能力。

> **仓库结构**：面板（本仓库）与节点 Agent（[XPanel-Node](https://github.com/acdc-awa/XPanel-Node)）为两个独立仓库。Agent 二进制经 GitHub Actions 发布到 XPanel-Node Releases（linux/amd64 + arm64，附 sha256 校验），节点安装与自升级均从 Releases 拉取；通信协议包（`pkg/protocol`）单源托管于 XPanel-Node，本仓库经 go.mod 引入。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│           用户自备反向代理 (Caddy/Nginx) —— TLS 终止 + 域名/路径分流           │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ 127.0.0.1:18080 / 18082 / 6000
┌───────────────────────────────────┴─────────────────────────────────────────┐
│                           Master 主控控制面 (Docker)                         │
│                                                                             │
│  ┌────────────────────────┐  ┌─────────────────────────┐  ┌──────────────┐  │
│  │ Vue3 + Element Plus    │  │ Gin REST API + JWT 鉴权 │  │ 节点 WS 网关  │  │
│  │ 管理端 & 用户端 SPA     │  │ 订阅服务 (Clash / VLESS) │  │ (WSS 长连接) │  │
│  └────────────────────────┘  └─────────────────────────┘  └──────────────┘  │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │ (双向长连接: 心跳 / 流量上报 / 配置下发)
             ┌─────────────────────────┴─────────────────────────┐
             ▼                                                   ▼
┌─────────────────────────┐                         ┌─────────────────────────┐
│    Agent 节点 1 (VPS)   │                         │     Agent 节点 2 (VPS)   │
│ ┌─────────────────────┐ │                         │ ┌─────────────────────┐ │
│ │ xray-agent 守护进程  │ │                         │ │ xray-agent 守护进程 │ │
│ ├─────────────────────┤ │                         | ├─────────────────────┤ │
│ │ Xray-core v26.6.27  │ │                         │ │ Xray-core v26.6.27  │ │
│ └─────────────────────┘ │                         | └─────────────────────┘ │
└─────────────────────────┘                         └─────────────────────────┘
```

---

## 核心特性

### 传输与安全
- **VLESS 多传输层**：TCP（推荐搭配 REALITY + Vision）、XHTTP（适配 CDN 与反代分流）、WS；
- **入站安全层可配置**：REALITY / TLS / vlessenc（Xray 内建加密，适用无 TLS 场景）/ none 四选一；
- **物理监听与对外暴露解耦**：节点本地仅监听 `127.0.0.1` 明文，TLS 终止由外部反向代理承担，订阅侧自动覆写分享地址（`ShareSecurity` / `ShareSNI` / `ShareHost` / `SharePath`）。

### 可视化拓扑编排
- 拖拽式画布管理节点连线与出站链（Outbound Chain），支持多级转发可视化；
- 直-弧-直动态避让连线与 DAG 自动排版，布局云端持久化，支持全屏模式。

### 安全机制
- JWT 密钥首次启动自动生成（`crypto/rand`，64 字符）并持久化，无默认弱密钥；
- 首次初始化自动生成 16 位随机管理员密码并在控制台高亮输出，首次登录强制改密；
- `reset-admin` CLI 子命令可在终端直接重置密码，重置同时递增 `token_version` 使全网旧会话立即失效；
- 支持 TOTP 两步验证、Cloudflare Turnstile 人机校验与登录失败防爆破锁定。

### 部署韧性
- 用户面板域名与节点通信端点物理分离，面板更换域名不影响节点在线；
- Agent 支持多候选端点池与自动回退，主端点不可达时自动切换备用地址。

### 计费与订阅
- 纯余额直付：礼品卡（卡密）批量生成、导出、核销与余额流水；
- 权限组模型：以入站开放权限组为访问控制来源，套餐自动绑定权限组；
- 订阅模板库与 Clash 模板引擎：支持 `$PROXIES$` 全量展开、`$FILTER_PROXIES(regex)` 正则分组。

### 监控与运营
- 仪表盘核心 KPI 卡片、3/7/30 天流量趋势图、节点时序性能监控（CPU / 内存 / 磁盘 / 带宽 / 连接数）；
- 公告系统（置顶与首页弹窗）、审计日志、定时备份。

---

## 快速部署（主控）

生产环境推荐 **Docker Compose + 压缩包挂载**形态部署主控：release 压缩包含编译好的二进制与前端产物，解压到宿主目录、完成配置后即可启动。TLS 终止与域名/路径分流由你自行部署的反向代理承担，本项目不随 compose 部署任何反代。

### 目录结构（安装后）

```
/opt/xray-panel/
├── master                  # 主控二进制（升级时替换）
├── web/dist/               # 前端产物（升级时替换）
├── configs/config.yaml     # 应用配置（唯一入口，编辑后重启生效）
├── data/                   # 数据目录（SQLite + JWT + 备份，持久化）
├── docker-compose.yml
├── .env                    # 仅 compose 编排参数（宿主端口映射 / BIND_ADDR，不进进程）
├── .env.example
├── Dockerfile.runtime      # 固定运行时镜像定义（compose 检测到镜像缺失时自动构建）
├── Caddyfile.reference     # 自备反代参考模板
├── install.sh              # 一键部署/升级脚本（release 包内自带）
└── deploy/master/entrypoint.sh
```

### 第一步：运行安装脚本

在目标目录执行（二选一）：

```bash
# 方式 A：在线一键（默认取最新 release，解压到当前目录）
curl -fsSL https://raw.githubusercontent.com/acdc-awa/xpanel/master/deploy/master/install.sh | bash

# 方式 B：下载 release 包到服务器后，使用包内脚本
bash install.sh                 # 解压到当前目录
bash install.sh --dir /opt/xray-panel   # 或指定目录
```

脚本自动完成：架构探测（amd64/arm64）→ 下载对应 release 包（直连 GitHub 超时自动切换内置镜像源）→ sha256 强制校验 → 解压二进制、前端产物与编排模板到目标目录 → 首次安装生成 `configs/config.yaml` 与 `.env` → 创建 `data/` 数据目录 → 以 root 运行时自动将安装目录属主调整为容器内用户（uid 1000）。

重复运行默认只更新二进制与前端产物，**保留已有配置与数据**（即升级语义；`--fresh` 强制全新覆盖）。

常用选项：

| 选项 | 说明 |
|---|---|
| `--dir <path>` | 安装目录（默认当前目录） |
| `--version <v>` | 钉版本安装（如 `v0.1.21`），缺省取最新 release |
| `--file <path>` | 本地 release 压缩包，完全离线部署 |
| `--mirror <url>` | GitHub 替代基址/代理前缀（如 `https://ghproxy.net/https://github.com`） |
| `--fresh` | 全新覆盖（默认保留已有配置与数据） |
| `--dry-run` | 只打印将执行的步骤 |

### 第二步：编辑配置

编辑 `configs/config.yaml`（应用配置唯一入口；`.env` 只承载 compose 编排参数，不参与应用配置）。至少填写面板公网地址，JWT 密钥与管理员账密留空即可（首次启动自动生成）：

```yaml
app:
  env: prod
  public_url: https://panel.yourdomain.com
  # ws_public_url: wss://ws.yourdomain.com/node/ws   # 可选；不填则使用面板域名 + /node/ws
```

**三端口模型**：面板由三个独立监听端口组成——

- **面板**（`app.port`，默认 18080）：SPA 前端与后端 API 合并监听，含 `/healthz` `/readyz` 探针；
- **节点 WS 网关**（`app.ws_port`，默认 18082）：对外路径 `/node/ws`，可用 `app.ws_public_url` 整体覆盖；
- **订阅**（`app.sub_port`，默认 6000）：独立订阅端口。

三个端口默认只绑定宿主机 `127.0.0.1`（改 `.env` 中 `BIND_ADDR=0.0.0.0` 可对全网卡开放），由你部署的反向代理按域名/路径分流（参考模板见 `Caddyfile.reference`）。

### 第三步：启动

```bash
docker compose up -d
```

首次启动会自动构建固定运行时镜像（仅运行时依赖，不含业务代码，不随版本变化），无需手动 `docker build`。

### 第四步：配置反向代理

TLS 终止与 443 端口由你的反代接管。以 Caddy 为例，使用包内 `Caddyfile.reference`（模板已按 `127.0.0.1` upstream 配好三端口分流规则）：

```bash
docker run -d --name caddy \
  -p 80:80 -p 443:443 \
  -v /opt/xray-panel/Caddyfile.reference:/etc/caddy/Caddyfile:ro \
  -v caddy-data:/data -v caddy-config:/config \
  -e SITE_ADDRESS=panel.yourdomain.com \
  -e SUB_SITE_ADDRESS=sub.yourdomain.com \
  caddy:2-alpine
```

- `SITE_ADDRESS` 为面板域名，Caddy 自动申请并续签 HTTPS 证书；`SUB_SITE_ADDRESS` 为订阅独立域名（可选，不用可删除模板中对应段落）；
- 使用 Nginx 等其他反代时，参照模板注释中的分流规则自行编写（`/node/ws` 规则必须先于默认反代匹配）；
- 反代与面板同机时保持 `BIND_ADDR=127.0.0.1`，反代容器通过 `host.docker.internal` 或宿主机网卡访问各端口。

> **安全提示（客户端 IP 识别）**：面板按 `CF-Connecting-IP` → `X-Real-IP` → `X-Forwarded-For` → `RemoteAddr` 的优先级识别客户端 IP，用于登录/订阅限流、审计日志与人机验证。请保持「面板端口仅绑定 127.0.0.1 + 反代前置」的部署形态——切勿将面板端口直接暴露公网，否则客户端 IP 可被伪造，基于 IP 的限流将失效。

### 第五步：获取初始管理员密码

```bash
docker compose logs master
```

首次初始化会输出如下卡片（随机密码仅显示一次，首次登录后强制修改）：

```text
==========================================================================
                   XrayPanel 主控系统首次初始化成功！
==========================================================================
   管理后台:       https://panel.yourdomain.com (或您的反代域名)
   管理员账号:     admin@panel.local
   初始管理员密码: Kd4%H&$sb67Bnk^@
--------------------------------------------------------------------------
   [安全提示] 初始随机密码仅在控制台显示一次，请妥善保存！
   [安全提示] 首次登录后系统将强制要求修改密码。
==========================================================================
```

---

## 接入节点（Agent）

1. 登录管理后台，进入 **「服务器」** 页面，点击 **「新增服务器」**；
2. 填写服务器名称与公网 IP，保存后点击对应服务器的 **「安装命令」** 按钮复制一键安装指令；
3. 登录节点 VPS，以 `root` 权限粘贴并执行：

```bash
bash <(curl -fsSL https://github.com/acdc-awa/XPanel-Node/releases/latest/download/install-agent.sh) \
  --master wss://panel.yourdomain.com/node/ws \
  --node-id 1 \
  --secret sec_xxxxxxxxxxxxxxxx
```

脚本自动完成：

- 从 [XPanel-Node Releases](https://github.com/acdc-awa/XPanel-Node/releases) 下载 `xray-agent` 二进制（自动匹配 amd64/arm64，按 release `checksums.txt` 强制 sha256 校验）；
- 下载并配置锁定的 Xray-core v26.6.27（官方 Releases + 校验）；
- 配置 systemd 守护进程并启动，与主控建立 WSS 长连接后自动上线。

节点侧同样支持离线与镜像参数：`--agent-file` / `--xray-file`（本地文件安装）、`--agent-mirror`（GitHub 代理前缀）、`--agent-version`（钉版本）、`--force-config`（重置节点配置）等，直连 GitHub 超时时脚本也会自动切换内置镜像源。

---

## 日常运维

### 重置管理员密码

无需登录数据库，在宿主机执行：

```bash
# 方式 A：自动生成全新 16 位随机强密码
docker compose exec master /app/master reset-admin

# 方式 B：指定新密码
docker compose exec master /app/master reset-admin -password "MyNewPass2026#!"

# 指定目标管理员（默认重置首个管理员）
docker compose exec master /app/master reset-admin -email admin@example.com
```

重置后系统自动递增 `token_version`，所有已签发的旧会话 Token 立即失效。

### 升级主控

三种方式任选：

```bash
# 方式 A（推荐）：管理后台 → 系统设置 → 「检查更新 / 应用更新」
# 自动下载 release 并强制校验 sha256，替换后进程退出由容器自动拉起新版本，
# 新版本启动失败时自动回滚上一版本。更新前请先手动备份。

# 方式 B：重新运行安装脚本（覆盖 master 与 web/dist，保留配置与数据）
cd /opt/xray-panel && bash install.sh

# 方式 C：离线手动升级
curl -fLO https://github.com/acdc-awa/xpanel/releases/latest/download/xpanel-master-<ver>-linux-amd64.tar.gz
sha256sum -c xpanel-master-<ver>-linux-amd64.tar.gz.sha256
tar -xzf xpanel-master-<ver>-linux-amd64.tar.gz -C /opt/xray-panel
docker compose restart master
```

升级只替换 `master` 二进制与 `web/dist`，`configs/config.yaml` 与 `data/` 不受影响；回滚 = 用上一版文件覆盖同名路径后 `docker compose restart master`。

### 节点维护命令

在节点 VPS 上执行：

```bash
xray-agent status       # 查看 Agent 与 Xray 进程运行状态
xray-agent restart      # 重启 Agent 及其托管的 Xray-core
xray-agent logs -n 100  # 查看最近 100 行运行日志（-f 持续跟踪）
xray-agent upgrade      # 检查并执行 Agent 自升级
xray-agent uninstall    # 卸载 Agent 及相关组件
```

---

## 本地开发

### 依赖环境

- Go 1.26+
- Node.js 22+ 与 npm（CI 同版本）
- Xray-core v26.6.27（配置验证用，建议使用[官方二进制](https://github.com/XTLS/Xray-core/releases)）

### 仓库与工作区

面板与节点两个仓库同级放置，通过根目录 `go.work` 聚合为 Go 工作区：

```bash
git clone https://github.com/acdc-awa/xpanel.git
git clone https://github.com/acdc-awa/XPanel-Node.git
# 目录结构：xpanel/ 与 XPanel-Node/ 同级，工作区文件位于父目录 go.work
```

面板经 `go.mod` 引入 XPanel-Node 的 `pkg/protocol`（`require github.com/acdc-awa/xpanel-node`）；工作区模式下本地修改 agent 仓库代码即时生效，发布时以 go.mod 钉定的版本为准。

### 后端

```bash
cd xpanel

# 本地运行主控
go run ./cmd/master -config configs/config.example.yaml

# 运行全量单元测试
go test ./...

# 编译
go build -o bin/master ./cmd/master
```

### 前端

```bash
cd xpanel/web

npm install          # 安装依赖
npm run dev          # Vite 开发服务器（端口 5173）
npm run typecheck    # vue-tsc 严格类型检查
npm run build        # 生产构建（含类型检查）
```

---

## 技术栈

| 层次 | 技术选型 | 说明 |
|---|---|---|
| 后端框架 | Go 1.26 + Gin | 单二进制分发，低资源占用 |
| 持久层 | GORM + SQLite（默认） | 开发与生产均默认 SQLite（WAL 模式），MySQL 驱动保留可选 |
| 协议引擎 | Xray-core v26.6.27（锁定版本） | 仅 VLESS 协议，传输层 TCP / XHTTP / WS |
| 前端框架 | Vue 3.5 + Vite + TypeScript | 组合式 API，Pinia 状态管理 |
| UI 组件库 | Element Plus + SCSS | 深色/浅色主题适配 |
| 拓扑画布 | @vue-flow/core | 自定义节点、动态避让连线、DAG 自动排版 |
| 反向代理 | 用户自备（Caddy 2 / Nginx 等） | TLS 终止与域名/路径分流由用户部署的反代承担，仓库提供参考模板 |
| 安全与认证 | JWT (HMAC-SHA256) + Argon2id + TOTP | 无状态鉴权、会话版本吊销、两步验证 |

---

## 许可证

本项目基于 [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE) 开源，Copyright (C) 2026 acdc-awa。

- 任何人都可以自由使用、修改和再分发本项目，但无论以二进制还是网络服务形式向他人提供，都必须以相同协议开放完整源代码。
- 捆绑/运行时下载的 [Xray-core](https://github.com/XTLS/Xray-core) 官方二进制遵循 [MPL-2.0](https://github.com/XTLS/Xray-core/blob/main/LICENSE)，不受本协议约束。
- 如需商业授权（闭源、豁免 AGPL 义务），请联系作者单独洽谈。
