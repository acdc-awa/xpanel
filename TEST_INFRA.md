# End-to-End Test Infrastructure & Validation Specification

## 1. Overview & Architecture

This document defines the End-to-End (E2E) testing framework and validation protocol for the **Node-Centric WebUI & Backend Redesign** of the Xray Management System.

The testing architecture provides automated, requirement-driven verification across the complete full-stack data pipeline:

```
[ Master REST API Input ] 
         │
         ▼
[ GORM SQLite Persistence ] 
         │
         ▼
[ Xray Config Generator (internal/master/xray/config.go) ] 
         │
         ▼
[ Generated Xray JSON File ] 
         │
         ▼
[ Xray Binary Core Syntax & Semantic Check (xray -test -config) ]
```

---

## 2. Testing Philosophy & 4-Tier Methodology

The test suite operates under an **opaque-box, requirement-driven** methodology to verify functional completeness, boundary robustness, multi-component interaction, and real-world deployment viability.

### Tier 1: Feature Coverage
Validates each individual feature, protocol, transport layer, outbound type, and routing rule matcher in isolation. At least 5 distinct test scenarios per major sub-category:
- **Inbound Protocols & Transports**: VLESS + REALITY (Vision), VLESS + WebSocket + TLS, VLESS + gRPC + TLS, VLESS + xHTTP + REALITY, VLESS + TCP Fallbacks, VLESS Standard TLS.
- **Outbound Protocols**: Freedom (Direct), Blackhole (Blocked), Socks Proxy with auth, VMess Proxy Relay, VLESS Relay.
- **Routing Matchers**: Domain matcher (`geosite:cn`, `domain:baidu.com`), IP CIDR matcher (`geoip:private`, `192.168.0.0/16`), Port range matcher (`80,443,8000-9000`), InboundTag matcher (`api`, `vless-ws`), Network/Protocol matcher (`tcp`, `udp`, `bittorrent`).

### Tier 2: Boundary & Corner Cases
Tests system resilience, field validation, edge values, and negative input rejection:
- **Extreme Valid Ports**: Port 1 (min) and Port 65535 (max).
- **Invalid Ports**: Port 0, Port 65536, or negative ports (rejected with HTTP 400).
- **Missing Required Fields**: Omitting mandatory fields like `tag`, `network`, or `protocol` (rejected with HTTP 400).
- **Malformed JSON**: Invalid JSON in `settings_json` or `rule_json` (handled gracefully or rejected).
- **Invalid Transport Settings**: Invalid settings like missing `mode` in `xhttp` or missing `private_key` in `reality` (rejected by `ValidateSettings`).
- **Multi-Format String Lists**: Delimiter parsing in `domain`/`ip` (commas, newlines, semicolons, JSON arrays).
- **Port Conflict Detection**: Creating two inbounds on the same server with identical ports (rejected with HTTP 400).

### Tier 3: Cross-Feature Combinations
Validates multi-component interaction and topology synthesis:
- **Multi-Inbound + Multi-Outbound**: Node hosting 4 rich inbounds and 3 outbounds simultaneously.
- **Inbound-to-Outbound Tag Chaining**: Routing rules binding specific `inboundTag`s to custom `outboundTag`s.
- **Cascading Priority Rule Evaluation**: Verifying rules are sorted by `priority ASC, id ASC`.
- **Fallback Topologies**: Automatic appending of `direct` (Freedom) and `blocked` (Blackhole) outbounds when missing.
- **StreamSettings & Sockopt Overrides**: Merging multiplexing (Mux) and socket options (Sockopt mark/tproxy) into outbounds.

### Tier 4: Real-World Application Scenarios
Simulates full-scale, production-ready node configurations:
- **Production Server Setup**: High-density server node with 3 Rich Inbounds (REALITY, WS TLS, gRPC TLS), 3 Custom Outbounds (Freedom, Blackhole, Socks Upstream), 5 Granular Routing Rules (AdBlock, Private Block, CN Direct, HTTP Proxy, Default Fallback), and active user credentials.

---

## 3. Master API & Execution Protocol

### 3.1 Lifecycle & Daemon Management
1. **Compilation**: `go build -o /tmp/xray-master ./cmd/master`
2. **Database Isolation**: SQLite database placed at `/tmp/e2e_teamwork_ui.db` to prevent workspace pollution.
3. **Background Startup**: Executed on an isolated port (e.g. `18080`) with logs redirected to `/tmp/e2e_master.log`.
4. **Health Check**: Polling `GET /healthz` until HTTP status 200 is returned.

### 3.2 Authentication & Seeding Protocol
1. **Admin Login**: `POST /api/v1/auth/login` with `{"username":"admin","password":"admin123"}` retrieves JWT Bearer Token.
2. **User Seeding**: Create invitation code (`POST /api/v1/admin/invitations`) and register active user (`POST /api/v1/auth/register`). Seeded active user provides required client UUID credentials for Xray inbound generation.

### 3.3 Endpoint Mapping
| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/admin/servers` | `POST` | Create Server Node entity |
| `/api/v1/admin/inbounds` | `POST` | Create 3x-ui Inbound under Server Node |
| `/api/v1/admin/servers/:id/outbounds` | `POST` | Create 3x-ui Outbound under Server Node |
| `/api/v1/admin/servers/:id/routing` | `POST` | Create Routing Rule under Server Node |
| `/api/v1/admin/servers/:id/generate-config` | `POST` | Trigger configuration compilation |

---

## 4. Complete 3x-ui Payload JSON Schemas

### 4.1 Inbound Schemas (`settings` object)

#### VLESS + TCP + REALITY (Vision Flow)
```json
{
  "server_id": 1,
  "tag": "vless-reality-tcp",
  "protocol": "vless",
  "port": 10443,
  "network": "tcp",
  "tls_type": "reality",
  "settings": {
    "reality": {
      "server_name": "itunes.apple.com",
      "public_key": "q8K-3hJ5w_Z6Y_1mX2v8B4N3M5L6K7J8I9H0G1F2E3D4",
      "short_id": "6ba7b810",
      "private_key": "oH8v_Z6Y_1mX2v8B4N3M5L6K7J8I9H0G1F2E3D4C5B6",
      "dest": "itunes.apple.com:443"
    }
  },
  "ratio": 1.0
}
```

#### VLESS + WebSocket + TLS
```json
{
  "server_id": 1,
  "tag": "vless-ws-tls",
  "protocol": "vless",
  "port": 20443,
  "network": "ws",
  "tls_type": "tls",
  "settings": {
    "ws": {
      "path": "/ray-ws",
      "host": "cdn.example.com"
    },
    "tls": {
      "server_name": "cdn.example.com",
      "cert_file": "/etc/ssl/certs/cdn.crt",
      "key_file": "/etc/ssl/private/cdn.key"
    }
  },
  "ratio": 1.0
}
```

#### VLESS + gRPC + TLS
```json
{
  "server_id": 1,
  "tag": "vless-grpc-tls",
  "protocol": "vless",
  "port": 30443,
  "network": "grpc",
  "tls_type": "tls",
  "settings": {
    "grpc": {
      "service_name": "xray-grpc-service",
      "multi_mode": true
    },
    "tls": {
      "server_name": "grpc.example.com",
      "cert_file": "/etc/ssl/certs/grpc.crt",
      "key_file": "/etc/ssl/private/grpc.key"
    }
  },
  "ratio": 1.0
}
```

#### VLESS + xHTTP + REALITY
```json
{
  "server_id": 1,
  "tag": "vless-xhttp-reality",
  "protocol": "vless",
  "port": 40443,
  "network": "xhttp",
  "tls_type": "reality",
  "settings": {
    "xhttp": {
      "mode": "stream-up",
      "path": "/xhttp-path"
    },
    "reality": {
      "server_name": "dl.google.com",
      "public_key": "p9L-4iK6x_A7Z_2nY3w9C5O4N6M7L8K9J0I1H2G3F4E5",
      "short_id": "7cb8c921",
      "private_key": "nI9w_A7Z_2nY3w9C5O4N6M7L8K9J0I1H2G3F4E5D6C7",
      "dest": "dl.google.com:443"
    }
  },
  "ratio": 1.0
}
```

#### VLESS + TCP + Fallbacks
```json
{
  "server_id": 1,
  "tag": "vless-fallback-tcp",
  "protocol": "vless",
  "port": 50443,
  "network": "tcp",
  "tls_type": "tls",
  "settings": {
    "tls": {
      "server_name": "main.example.com",
      "cert_file": "/etc/ssl/certs/main.crt",
      "key_file": "/etc/ssl/private/main.key"
    },
    "fallbacks": [
      { "dest": "8080", "xver": 1 },
      { "path": "/web", "dest": "8081", "xver": 1 }
    ]
  },
  "ratio": 1.0
}
```

### 4.2 Outbound Schemas

#### Freedom Outbound (Direct)
```json
{
  "tag": "direct",
  "protocol": "freedom",
  "settings_json": "{\"domainStrategy\":\"UseIP\",\"userLevel\":0}",
  "stream_settings_json": "{\"sockopt\":{\"mark\":255}}",
  "send_through": "0.0.0.0",
  "enabled": true,
  "priority": 1,
  "remark": "Direct connection outbound"
}
```

#### Blackhole Outbound (Blocked)
```json
{
  "tag": "blocked",
  "protocol": "blackhole",
  "settings_json": "{\"response\":{\"type\":\"none\"}}",
  "stream_settings_json": "",
  "send_through": "",
  "enabled": true,
  "priority": 2,
  "remark": "Drop blocked packets outbound"
}
```

#### Socks Proxy Outbound
```json
{
  "tag": "socks-upstream",
  "protocol": "socks",
  "settings_json": "{\"servers\":[{\"address\":\"192.168.100.1\",\"port\":1080,\"users\":[{\"user\":\"admin\",\"pass\":\"secret123\"}]}]}",
  "stream_settings_json": "{\"network\":\"tcp\"}",
  "send_through": "",
  "enabled": true,
  "priority": 3,
  "remark": "Upstream Socks5 Proxy outbound"
}
```

### 4.3 Routing Rule Schemas

#### Domain & IP Rules
```json
{
  "outbound_tag": "direct",
  "rule_json": "{\"type\":\"field\",\"domain\":[\"geosite:cn\",\"domain:baidu.com\"],\"outboundTag\":\"direct\"}",
  "domain": "geosite:cn, domain:baidu.com",
  "ip": "",
  "port": "",
  "network": "",
  "enabled": true,
  "priority": 10,
  "remark": "Route CN sites direct"
}
```

---

## 5. Xray Binary Syntax Validation Protocol

Generated Xray JSON configurations are validated using the pre-compiled Xray core binary:

```bash
tools/xray/xray-bin/xray -test -config /path/to/generated_config.json
```

### Validation Criteria
1. **Exit Code**: Must equal `0`.
2. **Standard Output**: Must match pattern `Xray ... Configuration OK`.
3. **Negative Invalidation**: Invalid boundary payloads must produce HTTP 400 rejection or Xray binary error output with exit code non-zero.

---

## 6. Command-Line Usage & Maintenance Guidelines

### Running E2E Test Runner
```bash
chmod +x tests/teamwork_ui_validation.sh
./tests/teamwork_ui_validation.sh
```

### Exit Codes
- `0`: All test cases across Tiers 1–4 passed successfully.
- `1`: One or more test cases failed assertion checks.

### Maintenance & Extensibility
- To add a new test scenario, define a new test function in `tests/teamwork_ui_validation.sh` and register it in the main test execution flow.
- Ensure all temporary files are cleaned up upon script termination via POSIX `trap cleanup EXIT INT TERM`.
