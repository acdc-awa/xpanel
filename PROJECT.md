# Project: Node-Centric WebUI & Backend Redesign

## Architecture
- **Frontend Layer** (`web/src`): Vue 3.5 + Element Plus + Pinia + Vite. Refactors `servers.vue` into a Server-primary entity with modular drawer (`ServerNodeDrawer.vue`) for unified node management (Inbounds, Outbounds, Routing Rules, Config Preview, Agent Operations).
- **Backend API & Data Layer** (`internal/master/api`, `internal/models`): Gin REST handlers providing `/api/v1/admin/servers/:id/outbounds` and `/routing` CRUD endpoints. GORM models storing JSON payloads for Inbounds, Outbounds, and Routing Rules.
- **Xray Config Generator** (`internal/master/xray/config.go`): Compiles server-specific inbounds (VLESS, REALITY, WS, gRPC, xHTTP), outbounds (Freedom, Blackhole, Proxy), routing rules, and active user credentials into standard Xray `config.json`.
- **E2E Validation Layer** (`tests/teamwork_ui_validation.sh`): E2E test runner invoking Master API, generating `config.json`, and validating syntax with `xray -test -config`.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Server Node Primary View | Node list with quick actions, drawer trigger, status tags | M2 | R1 |
| 2 | Unified Node Drawer | Drawer UI with tabs: Overview, Inbounds, Outbounds, Routing, Config Preview | M2 | R1 |
| 3 | Frontend Outbound & Routing APIs | TypeScript types & Axios methods in `web/src/api/admin.ts` & `types.ts` | M2 | R1, R3 |
| 4 | 3x-ui Inbound Form Component | Visual form for VLESS, REALITY, WS/gRPC/xHTTP, Sniffing, Fallbacks | M2 | R2 |
| 5 | 3x-ui Outbound Form Component | Visual form for Freedom, Blackhole, Proxy outbounds, Mux, Sockopt | M2 | R2 |
| 6 | 3x-ui Routing Rules Form Component | Visual form for Domain, IP, Port, InboundTag, OutboundTag matching | M2 | R2 |
| 7 | Backend gRPC Transport Support | Add `GRPCSettings` to `config.go`, validate and build `case "grpc"` | M1 | R2, R3 |
| 8 | Backend Outbound & Routing Merge | Verify complex JSON payload parsing and merging in `Generate()` | M1 | R3 |
| 9 | Master API Payload Integration | Ensure API endpoints process rich 3x-ui JSON payloads seamlessly | M1 | R3 |
| 10 | E2E Test Suite & Test Harness | Automated test script calling Master API and verifying with `xray -test -config` | E2E Track | Acceptance Criteria |
| 11 | Full E2E Test Pass & Hardening | Pass 100% E2E tests (Phase 1) & Tier 5 Adversarial Coverage Hardening (Phase 2) | M3 | Acceptance Criteria |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Backend Data Pipeline & Generator | Extend `config.go` with gRPC transport, verify rich JSON merging, unit tests | None | **DONE** (gRPC/fallbacks/sniffing/inbound_tag 支持；门禁审查通过) |
| M2 | Node-Centric UI & 3x-ui Forms | Refactor `servers.vue`, add `ServerNodeDrawer.vue`, API types, rich forms | M1 (contracts) | **DONE** (抽屉 5 Tab + 出站/路由表单 + 入站表单增强；npm build/typecheck 通过) |
| M3 | Final E2E Validation & Hardening | Pass 100% E2E test suite (Phase 1) & Tier 5 Adversarial Coverage Hardening (Phase 2) | M1, M2, E2E Track | **DONE** (E2E 套件 235 PASS FAIL=0；对抗性单元测试与完整性审计全通过) |
| M4 | 数据可靠与升级（模块 B） | SQLite 备份/恢复、agent upgrade（推送预留）、compose 去 mysql、推送链路复查 | 无 | **DONE** (backup 调度/轮转/API + agent upgrade 校验替换 + compose 去 mysql；go vet/race 与 WSL 全量测试通过，E2E 235 PASS FAIL=0) |

## E2E Gate
- Spec: `TEST_INFRA.md`；Runner: `tests/teamwork_ui_validation.sh`（WSL 运行，235 断言 / 4 层）
- 门禁结论：`TEST_READY.md` — Auditor CLEAN、Reviewer/Challenger 通过、orchestrator 独立复跑 exit 0

## Interface Contracts
### Master Backend API ↔ Frontend WebUI
- `GET /api/v1/admin/servers/:id/outbounds` -> `{ "items": [ServerOutbound] }`
- `POST /api/v1/admin/servers/:id/outbounds` -> `{ "outbound": ServerOutbound }`
- `PUT /api/v1/admin/servers/:id/outbounds/:outbound_id` -> `{ "outbound": ServerOutbound }`
- `DELETE /api/v1/admin/servers/:id/outbounds/:outbound_id` -> `{ "deleted": id }` (HTTP 200)
- `GET /api/v1/admin/servers/:id/routing` -> `{ "items": [ServerRoutingRule] }`
- `POST /api/v1/admin/servers/:id/routing` -> `{ "rule": ServerRoutingRule }`
- `PUT /api/v1/admin/servers/:id/routing/:rule_id` -> `{ "rule": ServerRoutingRule }`
- `DELETE /api/v1/admin/servers/:id/routing/:rule_id` -> `{ "deleted": id }` (HTTP 200)
- `POST /api/v1/admin/servers/:id/generate-config` -> `{ "ok": bool, "pushed": bool, "message": string, "config": "<JSON>" }`（配置保存为待推送；节点在线立即下发，离线则上线自动补推）
- `POST /api/v1/admin/xray/preview-config` -> `{ "config": "<JSON>" }`（表单实时预览，不落库不推送）
- 统一响应包裹 `{ "code": 0, "message": "ok", "data": {...} }`（见 `internal/pkg/util/response.go`；错误为 HTTP 400/500 + `{ "code": <非0>, "message": "..." }`）

## Code Layout
- `web/src/api/types.ts`: TypeScript interfaces for ServerOutbound, ServerRoutingRule, InboundSettings
- `web/src/api/admin.ts`: Axios API functions for Outbound and Routing CRUD
- `web/src/views/admin/servers.vue`: Primary Node list view
- `web/src/views/admin/servers/ServerNodeDrawer.vue`: Unified Node Management Drawer
- `web/src/views/admin/servers/OutboundConfigEditor.vue`: 3x-ui Outbound Form Modal
- `web/src/views/admin/servers/RoutingRuleEditor.vue`: 3x-ui Routing Rule Form Modal
- `internal/master/xray/config.go`: Xray config generator
- `internal/master/xray/config_test.go`: Generator unit tests
- `tests/teamwork_ui_validation.sh`: E2E validation script
