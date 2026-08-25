# 🚀 XrayPanel

<div align="center">

**现代化、轻量级、企业级「主控 - 节点 - 用户」三层代理管理面板**  
*基于 Go 1.26 + Vue 3.5 + Xray-core v26.6.27*

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Vue Version](https://img.shields.io/badge/Vue-3.5+-4FC08D?style=flat&logo=vuedotjs)](https://vuejs.org)
[![Xray-core](https://img.shields.io/badge/Xray--core-v26.6.27-red?style=flat)](https://github.com/XTLS/Xray-core)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](https://docker.com)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?style=flat&logo=typescript)](https://www.typescriptlang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

---

## 📖 项目简介

**XrayPanel** 是一套专为高性能、高可用与抗封锁场景设计的现代机场管理系统。系统采用 **「主控（Master） - 节点（Agent） - 用户端（Client）」** 三层架构，彻底解耦物理监听与外部反代，全面拥抱 **TCP (REALITY)** 与 **XHTTP (Splithttp)** 黄金双核传输协议，提供可视化拓扑路由编排、权限组订阅模板化、礼品卡与余额直付、以及全自动安全初始化等现代化特性。

> 📦 **仓库结构**：面板（本仓库，`XPanel`）与节点 Agent（[`XPanel-Node`](https://github.com/acdc-awa/XPanel-Node)）为两个独立仓库。Agent 二进制经 GitHub Actions 发布到 XPanel-Node Releases（linux/amd64 + arm64，附 sha256 校验），节点安装与自升级均从 Releases 拉取；通信协议包（`pkg/protocol`）单源托管于 XPanel-Node，本仓库经 go.mod 引入。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│           用户自备反向代理 (Caddy/Nginx) —— TLS 终止 + 域名/路径分流            │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ 127.0.0.1:18080 / 18082 / 6000
┌───────────────────────────────────┴─────────────────────────────────────────┐
│                           Master 主控控制面 (Docker)                          │
│                                                                             │
│  ┌────────────────────────┐  ┌─────────────────────────┐  ┌──────────────┐  │
│  │ Vue3 + Element Plus    │  │ Gin REST API + JWT 鉴权 │  │ 节点 WS 网关 │  │
│  │ 管理端 & 用户端 SPA    │  │ 订阅服务 (Clash / VLESS)│  │ (WSS 长连接) │  │
│  └────────────────────────┘  └─────────────────────────┘  └──────────────┘  │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │ (双向长连接: 心跳 / 流量上报 / 配置热推)
             ┌─────────────────────────┴─────────────────────────┐
             ▼                                                   ▼
┌─────────────────────────┐                         ┌─────────────────────────┐
│     Agent 节点 1 (VPS)   │                         │     Agent 节点 2 (VPS)   │
│ ┌─────────────────────┐ │                         │ ┌─────────────────────┐ │
│ │ xray-agent 守护进程 │ │                         │ │ xray-agent 守护进程 │ │
│ ├─────────────────────┤ │                         ├─────────────────────┤ │
│ │ Xray-core v26.6.27  │ │                         │ │ Xray-core v26.6.27  │ │
│ └─────────────────────┘ │                         └─────────────────────┘ │
└─────────────────────────┘                         └─────────────────────────┘
```

---

## ✨ 核心特性

### 1. 🌐 传输层架构纯粹化（黄金双核）
- 🚀 **TCP (Raw) + REALITY + Vision**：直连极限吞吐，免证书借壳伪装天花板；
- 🌊 **XHTTP (Splithttp) + TLS (Caddy/CDN)**：新一代原生 Web 请求切片模拟，彻底替代 WS 与 gRPC，天然适配 Caddy 反代与 CDN 穿透，抗断流能力极强；
- 🛡️ **物理监听与外部订阅彻底解耦**：节点本地仅监听 `127.0.0.1` 明文，外部通过 Caddy/Nginx 卸载 TLS，订阅自动覆写 `ShareSecurity` / `ShareSNI` / `ShareHost` / `SharePath`。

### 2. 🗺️ 可视化拓扑画布（Topology Canvas）
- 拖拽式节点连线、多 Handle 精确走线、出站链（Outbound Chain）与多级转发可视化；
- 盒间直-弧-直动态避让算法与 Detour 绕行，支持 DAG 拓扑一键自动排版；
- 全屏沉浸模式与云端布局持久化。

### 3. 🔐 生产级安全自闭环
- **JWT 密钥全自动生成**：系统首次建库自动通过 `crypto/rand` 生成 64 字符高强度密钥持久化于数据库，彻底告别 `.env` 硬编码弱密钥；
- **控制台首次初始化高亮卡片**：自动生成 16 位初始管理员随机密码并在控制台输出 ASCII 边框卡片，标记强制改密；
- **`reset-admin` CLI 一键救砖**：支持随时在终端执行子命令重置密码，并自动递增 `token_version` 瞬间吊销全网旧会话；
- **安全矩阵**：完整支持 TOTP 2FA 双因素认证、Cloudflare Turnstile 人机校验、密码防爆破锁定。

### 4. 🛰️ 控制面与业务面隔离（抗封锁多域容灾）
- 前端用户面板（随时换域名）与节点通信端点（隐蔽域名/海外裸 IP）物理分离；
- Agent 支持多候选端点池（Fallback Pool）与在线安全探测自动回退机制，彻底告别“面板换域名导致节点全失联”的运维灾难。

### 5. 💳 财务、权限组与模板化订阅
- **纯余额直付闭环**：支持卡密（礼品卡）批量生成、导出、核销与余额流水账本；
- **Xboard 权限组模型**：以节点入站开放权限组为访问控制权威来源，套餐自动绑定权限组；
- **基于权限组的 Clash 模板引擎**：支持 `$PROXIES$` 全量展开、`$FILTER_PROXIES(regex)` 地区/流媒体正则分组与行内数组展开。

### 6. 📊 真实时序监控与 100% 真实数据
- 仪表盘 6 大核心 KPI 运营卡片、30 天上下行流量面积图、节点时序性能监控抽屉（CPU/内存/磁盘/实时带宽/连接数）；
- 完整的公告系统（置顶 + 首页强弹窗提醒），全站零 Mock 数据残留。

---

## 🚀 快速部署指南

生产环境推荐使用 **Docker Compose + 压缩包挂载形态**部署主控（Master）：release 压缩包包含编译好的二进制与前端产物，解压到宿主目录、配好 config 即可启动；TLS 终止与域名/路径分流由**你自己部署的反向代理**（Caddy / Nginx 等）承担，本项目不随 compose 部署任何反代。

### 目录结构（解压后）

```
/opt/xray-panel/
├── master                  # 主控二进制（挂载进容器，升级时替换）
├── web/dist/               # 前端产物（挂载进容器，升级时替换）
├── configs/config.yaml     # 配置文件（挂载进容器，编辑后重启生效）
├── data/                   # 数据目录（SQLite + JWT + 备份，持久化）
├── docker-compose.yml
├── .env.example
└── Caddyfile.reference     # 自备反代参考模板
```

### 第一步：部署主控（Master）

#### 1. 下载并解压 release 包
从 [GitHub Releases](https://github.com/acdc-awa/xpanel/releases) 下载 `xpanel-master-<version>-linux-amd64.tar.gz`（含 `.sha256` 校验）：

```bash
cd /opt && mkdir -p xray-panel && cd xray-panel
# 下载后校验
sha256sum -c xpanel-master-*.tar.gz.sha256
tar -xzf xpanel-master-*.tar.gz
```

#### 2. 配置 `config.yaml`
编辑 `configs/config.yaml`，填入你的面板公网地址（JWT 密钥与管理员账密留空=首次启动自动生成，见文件内注释）：

```yaml
app:
  env: prod
  public_url: https://panel.yourdomain.com
  # ws_public_url: wss://ws.yourdomain.com/node/ws   # 可选；不填则用面板域名 + /node/ws
```

> 三端口模型：面板由三个独立监听端口组成——**面板**（`APP_PORT`，默认 18080，SPA 前端与
> **后端 API** 合并监听，含 `/healthz` `/readyz` 探针）、**节点 WS 网关**（`APP_WS_PORT`，默认 18082，
> 对外路径 `/node/ws`，可用 `APP_WS_PUBLIC_URL` 整体覆盖）、**订阅**（`APP_SUB_PORT`，默认 6000）。
> 三个端口默认只绑定宿主机 `127.0.0.1`（改 `.env` 里 `BIND_ADDR=0.0.0.0` 可对全网卡开放），
> 由你自己部署的反代按域名/路径分流（参考模板见 `Caddyfile.reference`）。

#### 3. 准备数据目录与运行时镜像
```bash
# data 目录需属主为容器内 app 用户（uid 1000）才能写入
sudo chown -R 1000:1000 data

# 构建固定运行时镜像（一次性；只装运行时依赖，不含业务代码，不随版本变）
docker build -f Dockerfile.runtime -t xpanel-master-runtime:latest .
```
> `Dockerfile.runtime` 在仓库根目录；也可以 `docker build -t xpanel-master-runtime:latest https://github.com/acdc-awa/xpanel.git#master` 在线构建（须含 Dockerfile.runtime 的 tag/分支）。

#### 4. 启动容器
```bash
docker compose up -d
```

#### 5. 配置反向代理（自备）
本项目不部署 Caddy，TLS 终止与 `443` 端口由你的反代接管。以 Caddy 为例，使用包内 `Caddyfile.reference`（模板已按 127.0.0.1 upstream 配好三端口分流规则）:

```bash
docker run -d --name caddy \
  -p 80:80 -p 443:443 \
  -v /opt/xray-panel/Caddyfile.reference:/etc/caddy/Caddyfile:ro \
  -v caddy-data:/data -v caddy-config:/config \
  -e SITE_ADDRESS=panel.yourdomain.com \
  -e SUB_SITE_ADDRESS=sub.yourdomain.com \
  caddy:2-alpine
```

- `SITE_ADDRESS` 为面板域名，Caddy 自动申请并续签 HTTPS 证书；`SUB_SITE_ADDRESS` 为订阅独立域名（可选，不用可删掉模板中对应段）；
- 使用 Nginx 等其他反代时，按模板注释中的分流规则自行编写即可（`/node/ws` 规则必须先于默认反代匹配）；
- 反代与面板同机时保持 `BIND_ADDR=127.0.0.1` 即可，反代容器通过 `host.docker.internal` 或宿主机网卡访问各端口。

> **安全提醒（IP 头与限流）**：面板按 `CF-Connecting-IP` → `X-Real-IP` → `X-Forwarded-For` → `RemoteAddr`
> 的优先级识别客户端 IP，用于登录/订阅限流、审计日志与人机验证。请务必保持
> **「面板端口仅绑定 127.0.0.1 + 反代前置」**的部署形态——反代会覆盖/追加可信的 IP 头，
> 限流与审计才能按真实 IP 生效。**切勿将面板端口直接暴露公网**（否则攻击者可伪造 IP 头绕过按 IP 的限流）。

#### 6. 获取初始管理员密码
查看控制台日志，复制系统生成的初始高强随机密码：
```bash
docker compose logs master
```
你将看到如下高亮卡片：
```text
==========================================================================
                🎉 XrayPanel 主控系统首次初始化成功！                     
==========================================================================
   管理后台:       https://panel.yourdomain.com
   管理员账号:     admin@panel.local
   初始管理员密码: Kd4%H&$sb67Bnk^@
--------------------------------------------------------------------------
   ⚠️  安全提示: 初始随机密码仅在控制台显示一次，请妥善保存！
   ⚠️  安全提示: 首次登录后系统将强制要求修改密码。
==========================================================================
```

打开浏览器访问 `https://panel.yourdomain.com`，使用上述账号密码登录即可！

---

### 第二步：安装与接入被控节点（Agent）

1. 登录管理后台，进入 **「服务器」** 页面，点击 **「新增服务器」**；
2. 填写服务器名称与公网 IP，保存后点击对应服务器的 **「安装命令」** 按钮复制一键安装指令；
3. 登录海外节点 VPS，以 `root` 权限粘贴并执行该命令：

```bash
bash <(curl -fsSL https://github.com/acdc-awa/XPanel-Node/releases/latest/download/install-agent.sh) \
  --master wss://panel.yourdomain.com/node/ws \
  --node-id 1 \
  --secret sec_xxxxxxxxxxxxxxxx
```

脚本将全自动完成：
- 从 [XPanel-Node Releases](https://github.com/acdc-awa/XPanel-Node/releases) 下载 `xray-agent` 二进制（自动匹配 amd64/arm64，release `checksums.txt` 强制 sha256 校验）
- 下载并配置锁定的 `Xray-core v26.6.27`（官方 Releases + `.dgst` 校验）
- 配置 systemd 守护进程
- 启动并建立与主控的 WSS 安全长连接，秒级自动上线！

---

## 🛠️ 常用运维命令与 CLI 工具

### 1. 重置管理员密码（救砖 / 忘记密码）
无需登录数据库，直接在宿主机执行：

```bash
# 方式 A：自动生成全新 16 位随机强密码
docker compose exec master /app/master reset-admin

# 方式 B：指定新密码
docker compose exec master /app/master reset-admin -password "MyNewPass2026#!"
```
> **安全机制**：执行重置后，系统将自动递增 `token_version`，**全网所有已签发的旧会话 Token 将被即刻强制失效**。

### 2. 主控升级（压缩包挂载形态）
```bash
cd /opt/xray-panel
# 下载新 release 包并校验
curl -fLO https://github.com/acdc-awa/xpanel/releases/latest/download/xpanel-master-<ver>-linux-amd64.tar.gz
sha256sum -c xpanel-master-<ver>-linux-amd64.tar.gz.sha256
# 解压并覆盖二进制与前端产物（config.yaml / data 保留不动）
tar -xzf xpanel-master-<ver>-linux-amd64.tar.gz -C /opt/xray-panel
# 重启容器完成升级
docker compose restart master
```
> 升级只替换 `master` 与 `web/dist`，`configs/config.yaml` 与 `data/`（SQLite + JWT + 备份）持久化不丢失；
> 回滚 = 用上一版文件覆盖同名路径再 `restart` 即可。

### 3. 节点 Agent 状态与维护（在节点 VPS 执行）
```bash
# 查看 Agent 与 Xray 进程运行状态
xray-agent status

# 重启 Agent 及其托管的 Xray-core
xray-agent restart

# 查看实时运行日志
xray-agent logs -n 100

# 节点自升级检查
xray-agent upgrade
```

---

## 💻 本地开发与代码构建

### 依赖环境
- Go 1.26+
- Node.js 22+ & npm
- Xray-core v26.6.27（测试验证用）

### 1. 后端开发
```bash
# 克隆代码库（面板 + 节点两个仓库，同级目录放置）
git clone https://github.com/acdc-awa/xpanel.git
git clone https://github.com/acdc-awa/XPanel-Node.git
cd xpanel

# 本地运行主控
go run ./cmd/master -config configs/config.example.yaml

# 运行全量单元测试
go test ./...

# 编译主控
go build -o bin/master ./cmd/master
```

> **协议包本地解析**：面板经 `go.mod` 引入 XPanel-Node 的 `pkg/protocol`。发布前 `go.mod` 以 `replace github.com/acdc-awa/xpanel-node => ../agent` 指向同级 agent 仓库目录（本地开发两仓库须同级放置）；XPanel-Node 发布后删除 replace 钉版本即可。

### 2. 前端开发
```bash
cd web

# 安装依赖
npm install

# 启动 Vite 开发热重载服务器 (端口 5173)
npm run dev

# 严格类型检查与生产打包
npm run typecheck
npm run build
```

### 3. E2E 全链路自动化测试
```bash
# 运行端到端冒烟与功能回归测试
bash tests/run_e2e.sh
```

---

## 🧰 技术栈一览

| 层次 | 技术选型 | 作用与特点 |
|---|---|---|
| **后端框架** | **Go 1.26 + Gin** | 高并发、极低内存占用、单二进制分发 |
| **持久层** | **GORM + SQLite (开发/标准) / MySQL 8.4 (生产可选)** | 自动迁移、事务隔离、连接池优化 |
| **核心协议引擎** | **Xray-core v26.6.27 (固定锁定版本)** | 仅 VLESS 协议，专注 TCP-REALITY 与 XHTTP |
| **前端框架** | **Vue 3.5 + Vite + TypeScript** | 组合式 API (Composition API)、Pinia 状态机 |
| **UI 组件库** | **Element Plus + SCSS** | SaaS 级响应式质感设计、深色/浅色优雅适配 |
| **拓扑画布** | **@vue-flow/core** | 自定义节点、贝塞尔绕行算法、DAG 分层排版 |
| **反向代理** | **用户自备（Caddy 2 / Nginx 等）** | 由用户部署的反代卸载 TLS、按域名/路径分流；仓库提供参考模板 `Caddyfile.reference`（release 包内 / `deploy/master/Caddyfile`），三端口默认仅绑 `127.0.0.1` |
| **安全与认证** | **JWT (HMAC-SHA256) + Argon2id + TOTP** | 无状态鉴权、会话版本吊销、多因素认证 |

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 开源。
