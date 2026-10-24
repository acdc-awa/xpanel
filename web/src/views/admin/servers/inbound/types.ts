import type { FallbackItem } from '@/api/types'

export type InboundProtocol = 'vless' | 'dokodemo-door' | 'hysteria'
export type InboundNetwork = 'tcp' | 'xhttp' | 'hysteria'
export type InboundSecurity = 'reality' | 'tls' | 'none'
export type InboundType = 'user' | 'forward' | 'tunnel'

export type Hy2ObfsType = 'none' | 'salamander' | 'gecko'
export type Hy2MasqType = '404' | 'file' | 'proxy' | 'string'
export type Hy2HopMode = 'intervalRemote' | 'perConnRemote'

export interface HysteriaMasquerade {
  type: Hy2MasqType
  dir: string
  url: string
  rewriteHost: boolean
  xForwarded: boolean
  insecure: boolean
  content: string
  statusCode?: number
  // 表单未覆盖的 masquerade 键（headers 等）原样回写。
  extra?: Record<string, any>
}

export interface HysteriaConfig {
  udp_idle_timeout?: number
  // finalmask.udp[type=salamander]
  obfs_type: Hy2ObfsType
  obfs_password: string
  // gecko 分片区间（xray packetSize 是 "min-max" 字符串）
  obfs_min_packet: number
  obfs_max_packet: number
  // finalmask.quicParams：服务端方向（brutalUp=服务端上行=客户端下行）
  bw_up: string
  bw_down: string
  // finalmask.udp[type=udphop]：仅客户端消费，生成服务端配置时会被剥离
  port_hop_mode: Hy2HopMode
  port_hop_ports: string
  port_hop_interval: string
  // 手改 JSON 里其他 finalmask 条目（rand/tcp/filter、未知 udp 类型、自定义 quicParams），
  // 表单只编辑自己认识的字段，其余原样回写，避免保存时静默丢配置。
  finalmaskExtra?: Record<string, any>
  // 服务端伪装站点；mihomo 侧无对应字段（纯服务端能力）。
  masquerade: HysteriaMasquerade
  // hysteriaSettings 下表单未覆盖的键，原样回写。
  hysteriaSettingsExtra?: Record<string, any>
}

export interface TcpConfig {
  header_type: 'none' | 'http'
  request_host: string
  request_path: string
  acceptProxyProtocol: boolean
}

export type XhttpPlacement = '' | 'path' | 'cookie' | 'header' | 'query'
export type XhttpPaddingPlacement = '' | 'cookie' | 'header' | 'query' | 'queryInHeader'
export type XhttpPaddingMethod = '' | 'repeat-x' | 'tokenish'

export interface XhttpConfig {
  mode: 'auto' | 'packet-up' | 'stream-up' | 'stream-one'
  path: string
  host: string
  // 区间类字段在 xray 里是 Int32Range：只接受 "a-b" 字符串或整数，写成 {from,to} 会拒绝加载。
  xPaddingBytes: string
  sessionIDPlacement: XhttpPlacement
  sessionIDKey: string
  sessionIDLength: string
  sessionIDTable: string
  paddingObfsMode: boolean
  paddingKey: string
  paddingHeader: string
  paddingPlacement: XhttpPaddingPlacement
  paddingMethod: XhttpPaddingMethod
  noSSEHeader: boolean
  scMaxBufferedPosts?: number
  scStreamUpServerSecs: string
  serverMaxHeaderBytes?: number
  // xray 的 xmux 留空即由内核补 maxConnections=3 等默认值；显式写时必须整组写全。
  xmuxEnabled: boolean
  xmuxMaxConnections: string
  xmuxMaxConcurrency: string
  xmuxMaxRequestTimes: string
  xmuxMaxReusableSecs: string
  // 表单未覆盖的 xhttpSettings 键（headers/extra/downloadSettings/uplink*/seq* 等）原样回写。
  rawExtra?: Record<string, any>
  // xray 的 xhttp 传输没有 acceptProxyProtocol（只有 tcp/ws/httpupgrade/sockopt 有），
  // 不再提供该开关；需要接收 PROXY Protocol 时用安全层的 sockopt 统一配置。
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
  // 「跳过客户端证书校验」是订阅字段（share_allow_insecure），不放在 TLS 配置里：
  // 安全层只负责调度它，值由编辑器统一持有，避免同一个语义出现两个开关。
  reject_unknown_sni: boolean
  // 历史遗留：入站曾允许配置 usage=verify 的 CA 证书（服务端不消费）。
  // 控件已移除，仅原样回写，避免存量配置在保存时丢字段。
  legacy_verify_certs: any[]
  // 手写 JSON 里的内联证书（certificate/key 数组，非文件路径）：表单不编辑，
  // 但保存时必须原样带回，否则存量配置会被静默清空。
  legacy_inline_certs: any[]
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
  // 接收 PROXY Protocol 只在传输层（tcpSettings.acceptProxyProtocol）维护一个开关，
  // sockopt 不再重复提供（xray 运行时对两者取 OR，双开关会出现「关不掉」的假象）。
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
