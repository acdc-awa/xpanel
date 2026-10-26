import type { HysteriaConfig, XhttpConfig } from './types'

// xray 的 Int32Range 只接受 "a-b" 字符串或整数；写成 {from,to} 对象会让整份配置
// 反序列化失败（-test 报 Invalid integer range）。这里统一按字符串校验。
export interface IntRange {
  from: number
  to: number
}

export function parseIntRange(raw: string): IntRange | null {
  const s = (raw || '').trim()
  if (!s) return null
  if (!/^\d+(\s*-\s*\d+)?$/.test(s)) return null
  const parts = s.split('-').map((x) => Number(x.trim()))
  const from = parts[0]
  const to = parts.length > 1 ? parts[1] : parts[0]
  if (!Number.isInteger(from) || !Number.isInteger(to)) return null
  if (from < 0 || to < 0 || to < from) return null
  return { from, to }
}

function rangeError(raw: string, label: string, min: number): string {
  const s = (raw || '').trim()
  if (!s) return ''
  const r = parseIntRange(s)
  if (!r) return `${label}：区间写法应为 "最小-最大" 或单个整数（如 "100-1000"）`
  if (r.from < min) return `${label}：最小值不能小于 ${min}`
  if (r.to > 0 && r.to < r.from) return `${label}：最大值不能小于最小值`
  return ''
}

// 端口集合：PortList 支持 "20000-30000" 或 "20000-30000,40000"。
export function parsePortList(raw: string): number[] | null {
  const s = (raw || '').trim()
  if (!s) return null
  const out: number[] = []
  for (const part of s.split(',')) {
    const p = part.trim()
    if (!p) return null
    if (!/^\d+(\s*-\s*\d+)?$/.test(p)) return null
    const [a, b] = p.split('-').map((x) => Number(x.trim()))
    const from = a
    const to = b === undefined ? a : b
    if (!Number.isInteger(from) || !Number.isInteger(to)) return null
    if (from < 1 || to > 65535 || to < from) return null
    out.push(from, to)
  }
  return out.length > 0 ? out : null
}

// xray Bandwidth.Bps()：单位按 1024，最终统一 /8；低于 65536 B/s 会被 xray 拒绝。
const BW_UNITS: Record<string, number> = {
  '': 1,
  b: 1,
  bps: 1,
  k: 1024,
  kb: 1024,
  kbps: 1024,
  m: 1024 ** 2,
  mb: 1024 ** 2,
  mbps: 1024 ** 2,
  g: 1024 ** 3,
  gb: 1024 ** 3,
  gbps: 1024 ** 3,
  t: 1024 ** 4,
  tb: 1024 ** 4,
  tbps: 1024 ** 4,
}

/** 返回 xray 口径的字节/秒；空串返回 0，非法返回 null。 */
export function parseBandwidthBps(raw: string): number | null {
  const s = (raw || '').trim().toLowerCase()
  if (!s) return 0
  const m = s.match(/^(\d+(?:\.\d+)?)\s*([a-z]*)$/)
  if (!m) return null
  const mul = BW_UNITS[m[2]]
  if (mul === undefined) return null
  return (Number(m[1]) * mul) / 8
}

function bandwidthError(raw: string, label: string): string {
  const s = (raw || '').trim()
  if (!s) return ''
  const bps = parseBandwidthBps(s)
  if (bps === null) return `${label}：格式应为数字+单位，如 "100 mbps" / "1 gbps"`
  if (bps > 0 && bps < 65536) return `${label}：不能低于 65536 B/s（约 0.5 mbps）`
  return ''
}

export function validateXhttp(c: XhttpConfig): string[] {
  const errs: string[] = []
  const push = (m: string) => {
    if (m) errs.push(m)
  }
  push(rangeError(c.xPaddingBytes, 'XHTTP 填充区间', 1))
  push(rangeError(c.sessionIDLength, '会话 ID 长度', 1))
  push(rangeError(c.scStreamUpServerSecs, '上行流服务端保持时长', 0))
  if (c.serverMaxHeaderBytes !== undefined && c.serverMaxHeaderBytes !== null && c.serverMaxHeaderBytes < 0) {
    errs.push('服务端最大请求头字节数不能为负数')
  }
  if (c.scMaxBufferedPosts !== undefined && c.scMaxBufferedPosts !== null && c.scMaxBufferedPosts < 0) {
    errs.push('服务端最大缓冲请求数不能为负数')
  }
  if (c.xmuxEnabled) {
    push(rangeError(c.xmuxMaxConnections, 'xmux maxConnections', 0))
    push(rangeError(c.xmuxMaxConcurrency, 'xmux maxConcurrency', 0))
    push(rangeError(c.xmuxMaxRequestTimes, 'xmux hMaxRequestTimes', 0))
    push(rangeError(c.xmuxMaxReusableSecs, 'xmux hMaxReusableSecs', 0))
    const conn = parseIntRange(c.xmuxMaxConnections)
    const conc = parseIntRange(c.xmuxMaxConcurrency)
    // xray 硬拒：maxConnections 与 maxConcurrency 不能同时 >0。
    if (conn && conc && conn.to > 0 && conc.to > 0) {
      errs.push('xmux：maxConnections 与 maxConcurrency 只能填一个')
    }
  }
  return errs
}

// finalmask 部分（obfs / 带宽 / 端口跳跃）——对应 streamSettings.finalmask，
// 与 hysteriaSettings（伪装站点）分开展示，各自在所属 Tab 显示错误。
export function validateHysteriaFinalMask(c: HysteriaConfig): string[] {
  const errs: string[] = []
  if (c.obfs_type !== 'none') {
    if (!c.obfs_password.trim()) {
      errs.push('Hysteria 2 混淆：启用后必须填写混淆密码')
    }
    if (c.obfs_type === 'gecko') {
      const min = Number(c.obfs_min_packet)
      const max = Number(c.obfs_max_packet)
      if (!Number.isInteger(min) || !Number.isInteger(max) || min <= 0) {
        errs.push('Hysteria 2 混淆：分片区间必须是正整数')
      } else if (min > max) {
        errs.push('Hysteria 2 混淆：最小分片不能大于最大分片')
      } else if (max > 2048) {
        errs.push('Hysteria 2 混淆：最大分片不能超过 2048（xray gecko 缓冲上限）')
      }
    }
  }
  const up = (c.bw_up || '').trim()
  const down = (c.bw_down || '').trim()
  if (bandwidthError(up, '服务端上行带宽')) errs.push(bandwidthError(up, '服务端上行带宽'))
  if (bandwidthError(down, '服务端下行带宽')) errs.push(bandwidthError(down, '服务端下行带宽'))
  if ((up && !down) || (!up && down)) {
    errs.push('Hysteria 2 带宽：上行与下行需同时填写才会启用 Brutal（只填一侧会退回 BBR）')
  }
  if (c.congestion === 'force-brutal' && !up) {
    errs.push('QUIC 拥塞控制：force-brutal 必须同时填写服务端上行带宽')
  }

  // 取值范围来自 xray 的 QuicParamsConfig.Build()：
  // maxIdleTimeout 4–120、maxIncomingStreams ≥8、接收窗口 ≥16384。
  const rangeErr = (v: number | undefined, min: number, max: number | undefined, label: string) => {
    if (v === undefined || v === null) return ''
    if (!Number.isInteger(v) || v < min || (max !== undefined && v > max)) {
      return max === undefined ? `${label}：必须是不小于 ${min} 的整数` : `${label}：必须是 ${min}–${max} 之间的整数`
    }
    return ''
  }
  const pushRange = (v: number | undefined, min: number, max: number | undefined, label: string) => {
    const m = rangeErr(v, min, max, label)
    if (m) errs.push(m)
  }
  pushRange(c.max_idle_timeout, 4, 120, 'QUIC 最大空闲超时')
  pushRange(c.max_incoming_streams, 8, undefined, 'QUIC 入流上限')
  pushRange(c.init_stream_receive_window, 16384, undefined, 'QUIC 初始流接收窗口')
  pushRange(c.max_stream_receive_window, 16384, undefined, 'QUIC 最大流接收窗口')
  pushRange(c.init_connection_receive_window, 16384, undefined, 'QUIC 初始连接接收窗口')
  pushRange(c.max_connection_receive_window, 16384, undefined, 'QUIC 最大连接接收窗口')

  const ports = (c.port_hop_ports || '').trim()
  if (ports) {
    if (!parsePortList(ports)) {
      errs.push('端口跳跃：端口集合写法应为 "20000-30000" 或 "20000-30000,40000"')
    }
    const interval = (c.port_hop_interval || '').trim()
    if (c.port_hop_mode !== 'perConnRemote' && interval) {
      const r = parseIntRange(interval)
      if (!r) {
        errs.push('端口跳跃：间隔应为整数秒或 "最小-最大" 区间')
      } else if (r.from < 5) {
        errs.push('端口跳跃：间隔不能小于 5 秒')
      }
    }
  }
  return errs
}

// hysteriaSettings 部分（伪装站点 masquerade）。
export function validateHysteriaSettings(c: HysteriaConfig): string[] {
  const errs: string[] = []
  const m = c.masquerade
  if (m) {
    if (m.type === 'file' && !m.dir.trim()) {
      errs.push('伪装站点：file 类型必须填写静态目录')
    }
    if (m.type === 'proxy') {
      const u = m.url.trim()
      if (!u) {
        errs.push('伪装站点：proxy 类型必须填写目标 URL')
      } else if (!/^(https?:\/\/|unix:)/i.test(u)) {
        errs.push('伪装站点：URL 需以 http://、https:// 或 unix: 开头')
      }
    }
    if (m.type === 'string' && m.statusCode != null && (m.statusCode < 100 || m.statusCode > 599)) {
      errs.push('伪装站点：状态码需在 100–599 之间')
    }
  }
  return errs
}

/** 保存门禁：finalmask + hysteriaSettings 一起校验。 */
export function validateHysteria(c: HysteriaConfig): string[] {
  return [...validateHysteriaFinalMask(c), ...validateHysteriaSettings(c)]
}
