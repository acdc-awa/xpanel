# XrayPanel

主控-节点-用户三层机场面板：Go 后端 + Vue3 前端 + Xray-core 托管。

## 功能
- 主控：Web 面板、REST API、Xray 配置生成、订阅服务、节点 WebSocket 网关
- 节点 Agent：托管 Xray-core，WSS 长连接，心跳/流量上报，远程指令与自升级
- 用户端：邀请码注册、套餐购买、余额/礼品卡、订阅获取

## 快速开始
```bash
# 后端
cp configs/config.example.yaml configs/config.yaml
go build -o master.exe ./cmd/master
go run ./cmd/master -config configs/config.yaml

# 前端（开发）
cd web && npm install && npm run dev
```

## 测试
```bash
go test -count=1 ./...
go vet ./...
cd web && npm run typecheck && npm run build
bash tests/run_e2e.sh          # 核心 E2E（P0/P4/P5）
E2E_WITH_NODES=1 bash tests/run_e2e.sh   # 追加真实节点链路
```

## 部署与运维
- 主控：`deploy/master/`（Docker Compose + Caddy）
- 节点：`deploy/agent/install-agent.sh`
- 设计文档：`docs/`（按日期归档，入口见 `docs/进度总览.md`）
- 开发约定：`AGENTS.md` / `CLAUDE.md`

## 技术栈
Go 1.22+ / Gin / GORM（SQLite 开发、MySQL 8.4 可选）· Vue 3 / Element Plus / TypeScript · Xray v26.6.27（锁定，仅 VLESS）
