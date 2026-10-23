import type { TopologyData } from '@/api/admin'
import type { AccessLayer } from '@/api/types'

/** 画布内一个出站行（DB 行 + 生成期派生/虚拟行统一成这个形状）。 */
export interface BoxOutbound {
  /** DB 出站 id 的字符串形式；草稿直通为 `draft-tunnel-<inboundId>`。 */
  id: string
  tag: string
  protocol: string
  inbound_ref?: number | null
  virtual?: boolean
  owner_inbound_id?: number
  has_payload?: boolean
  isDraftTunnel?: boolean
  server_id?: number
}

export interface BoxData {
  server: TopologyData['servers'][number]
  inbounds: TopologyData['inbounds']
  outbounds: BoxOutbound[]
  layers: AccessLayer[]
  /** 用户拉伸后的盒子宽度（未拉伸 = 默认 440）。 */
  boxWidth?: number
}

export interface LayerGroup {
  layer: AccessLayer
  inbounds: TopologyData['inbounds']
}

export const DEFAULT_BOX_W = 440
export const ROW_H = 40
export const HEADER_H = 48
/** 层胶囊头部开销（容器上边距 6 + 头 ~26 + 头下间距 6），与 CSS 对齐。 */
export const CAP_HED = 38
/** 列头开销（列头 32 + 行首间距 6）。 */
export const COL_HED = 38

/**
 * 草稿判定：没有任何引用目标、不是派生/虚拟行、也没有载荷的非基础出站。
 * 引用目标、派生行、direct 虚拟行、已配置载荷（基础 settings / 自定义 JSON）都不算草稿。
 */
export function isOutboundDraft(out: {
  isDraftTunnel?: boolean
  inbound_ref?: number | null
  owner_inbound_id?: number
  virtual?: boolean
  has_payload?: boolean
  protocol?: string
}): boolean {
  if (out.isDraftTunnel) return true
  if (out.inbound_ref || out.owner_inbound_id || out.virtual || out.has_payload) return false
  if (out.protocol === 'freedom' || out.protocol === 'blackhole') return false
  return true
}

export function typeInfo(t?: string): { cls: string; text: string } {
  if (t === 'tunnel') return { cls: 'tunnel', text: '直通' }
  if (t === 'forward') return { cls: 'forward', text: '代理' }
  if (t === 'channel') return { cls: 'channel', text: '通道' }
  return { cls: '', text: '' }
}

/** 入站协议摘要标签（VLESS / HY2 / TUNNEL / 其它协议名）。 */
export function inboundSummary(
  inb: TopologyData['inbounds'][number]
): { port: number; proto: string; cls: string } {
  let proto = 'VLESS'
  let cls = 'vless'
  const p = (inb.protocol || '').toLowerCase()
  if (p === 'hysteria') {
    proto = 'HY2'
    cls = 'hy2'
  } else if (p === 'dokodemo-door' || inb.type === 'tunnel') {
    proto = 'TUNNEL'
    cls = 'tunnel'
  } else if (p === 'vless') {
    proto = 'VLESS'
    cls = 'vless'
  } else if (p) {
    proto = p.toUpperCase()
    cls = 'other'
  }
  return { port: inb.port, proto, cls }
}

/** 盒内路由线：S 形贝塞尔。 */
export function boxRulePath(sx: number, sy: number, tx: number, ty: number): string {
  const mx = (sx + tx) / 2
  return `M ${sx} ${sy} C ${mx} ${sy}, ${mx} ${ty}, ${tx} ${ty}`
}

/**
 * 跨盒引用线：水平引出后平滑过渡。
 * 有意不做阻挡检测——多盒同列堆叠时允许直穿中间盒子（视觉取舍）。
 */
export function refEdgePath(sx: number, sy: number, tx: number, ty: number): string {
  const dx = tx - sx
  const dy = ty - sy
  if (dx >= 40) {
    const seg = Math.min(24, dx * 0.15)
    const ax = sx + seg
    const bx = tx - seg
    // arc 钳制在半距内，防止横向轻微错位（dx≈40~130）时控制点交叉打出小回环
    const arc = Math.min(Math.max(40, (bx - ax) * 0.35), (bx - ax) / 2)
    return `M ${sx} ${sy} L ${ax} ${sy} C ${ax + arc} ${sy}, ${bx - arc} ${ty}, ${bx} ${ty} L ${tx} ${ty}`
  }
  // 反向/同列特殊走线：右侧引出 -> 绕背 S 弧度 -> 左侧接入
  const offset = Math.max(60, Math.abs(dy) * 0.4, Math.abs(dx) * 0.3)
  return `M ${sx} ${sy} C ${sx + offset} ${sy}, ${tx - offset} ${ty}, ${tx} ${ty}`
}
