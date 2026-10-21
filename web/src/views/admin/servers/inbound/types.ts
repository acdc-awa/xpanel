import type { FallbackItem } from '@/api/types'

export type InboundProtocol = 'vless' | 'dokodemo-door' | 'hysteria'
export type InboundNetwork = 'tcp' | 'xhttp' | 'hysteria'
export type InboundSecurity = 'reality' | 'tls' | 'none'
export type InboundType = 'user' | 'forward' | 'tunnel'

export interface HysteriaConfig {
  udp_idle_timeout?: number
}

export interface TcpConfig {
  header_type: 'none' | 'http'
  request_host: string
  request_path: string
  acceptProxyProtocol: boolean
}

export interface XhttpConfig {
  mode: 'auto' | 'packet-up' | 'stream-up' | 'stream-one'
  path: string
  host: string
  acceptProxyProtocol: boolean
}

export interface TlsConfig {
  cert_id: number
  server_name: string
  alpn: string
  cert_file: string
  key_file: string
  cipher_suites: string
  min_version: string
  max_version: string
  // 仅影响用户订阅（skip-cert-verify）。入站服务端不消费。
  allow_insecure: boolean
  reject_unknown_sni: boolean
  // 历史遗留：入站曾允许配置 usage=verify 的 CA 证书（服务端不消费）。
  // 控件已移除，仅原样回写，避免存量配置在保存时丢字段。
  legacy_verify_certs: any[]
}

export interface SockoptConfig {
  enabled: boolean
  tcp_fast_open: boolean
  tcp_fast_open_qlen?: number
  tcp_congestion: string
  tcp_mptcp: boolean
  tcp_keep_alive_idle?: number
  tcp_keep_alive_interval?: number
  tcp_user_timeout?: number
  accept_proxy_protocol: boolean
  mark?: number
  tproxy: 'off' | 'tproxy' | 'redirect'
}


export interface RealityConfig {
  dest: string
  server_name: string
  short_id: string
  private_key: string
  public_key: string
  min_client_ver: string
  max_client_ver: string
  max_time_diff: number
}

export interface VlessConfig {
  flow: string
  fallbacks: FallbackItem[]
  decryption?: string
}

export interface DokodemoConfig {
  target_mode: 'inbound' | 'manual' | 'draft'
  target_inbound_id?: number
  target_address?: string
  target_port?: number
  network: string // 'tcp,udp' | 'tcp' | 'udp'
  proxy_protocol?: boolean // 是否向目标发送 PROXY Protocol v2
}

export function copyText(txt: string) {
  if (!txt) return
  if (navigator?.clipboard?.writeText) {
    navigator.clipboard.writeText(txt)
  }
}
