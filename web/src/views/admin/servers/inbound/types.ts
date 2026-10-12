import type { FallbackItem } from '@/api/types'

export type InboundProtocol = 'vless' | 'dokodemo-door' | 'mixed' | 'socks' | 'http' | 'hysteria'
export type InboundNetwork = 'tcp' | 'xhttp' | 'hysteria'
export type InboundSecurity = 'reality' | 'tls' | 'none'
export type InboundType = 'user' | 'relay' | 'forward' | 'tunnel'

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
  allow_insecure: boolean
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

export interface AccountItem {
  user: string
  pass: string
}

export interface SocksHttpConfig {
  auth: 'password' | 'none'
  accounts: AccountItem[]
  udp: boolean // 仅 socks5
}

export function copyText(txt: string) {
  if (!txt) return
  if (navigator?.clipboard?.writeText) {
    navigator.clipboard.writeText(txt)
  }
}
