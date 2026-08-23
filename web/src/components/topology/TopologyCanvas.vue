<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  VueFlow,
  Handle,
  Position,
  MarkerType,
  type Connection,
  type Edge,
  type EdgeMouseEvent,
  type GraphNode,
  type NodeDragEvent,
  type NodeMouseEvent,
} from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { ElMessage, ElMessageBox } from 'element-plus'
import { FullScreen, MagicStick, RefreshRight, Plus, CopyDocument, Link, Setting } from '@element-plus/icons-vue'
import {
  createServerOutbound,
  createServerRoutingRule,
  deleteServerOutbound,
  deleteServerRoutingRule,
  getTopologyLayout,
  saveTopologyLayout,
  updateInbound,
  updateServerOutbound,
  type TopologyData,
} from '@/api/admin'
import type { InboundEndpoint, InboundItem, ServerOutbound } from '@/api/types'
import { errMsg } from '@/api/http'
import InboundEndpointsDrawer from '@/views/admin/servers/InboundEndpointsDrawer.vue'
import OutboundConfigEditor from '@/views/admin/servers/OutboundConfigEditor.vue'

const props = defineProps<{
  topology: TopologyData | null
  editable?: boolean
}>()

const emit = defineEmits<{
  (e: 'changed'): void
  (e: 'open-server', serverId: number): void
  (e: 'open-create-inbound', serverId: number): void
  (e: 'open-create-outbound', serverId: number): void
}>()

// ---- 节点/边构建 ----
interface BoxOutbound {
  id: string // DB 出站 = 数字字符串；虚拟 direct = `direct-<serverId>`
  tag: string
  protocol: string
  inbound_ref?: number | null
  virtual?: boolean
}

interface BoxData {
  server: TopologyData['servers'][number]
  inbounds: TopologyData['inbounds']
  endpointsMap: Map<number, InboundEndpoint[]>
  outbounds: BoxOutbound[]
  boxWidth?: number // 用户拉伸的盒子宽度（未拉伸 = 默认 440）
}

const nodes = ref<GraphNode[]>([])
const edges = ref<Edge[]>([])
const isFullscreen = ref(false)

// ---- 布局管理：本地缓存 + 云端同步（内容哈希去重）----
const LAYOUT_KEY = 'topology-layout'

interface LayoutData {
  hash: string
  positions: Record<string, { x: number; y: number }>
  widths: Record<string, number>
}

function loadLayoutLocal(): LayoutData {
  try {
    const raw = localStorage.getItem(LAYOUT_KEY)
    if (raw) {
      const d = JSON.parse(raw) as LayoutData
      if (d && typeof d === 'object' && d.positions) return d
    }
  } catch {
    /* 解析失败 → 空布局 */
  }
  // 兼容旧版双 key 迁移
  try {
    const oldPos = localStorage.getItem('topology-box-positions')
    const oldWidths = localStorage.getItem('topology-box-widths')
    if (oldPos || oldWidths) {
      const d: LayoutData = { hash: '', positions: {}, widths: {} }
      if (oldPos) d.positions = Object.fromEntries(JSON.parse(oldPos) as [string, { x: number; y: number }][])
      if (oldWidths) d.widths = Object.fromEntries(JSON.parse(oldWidths) as [string, number][])
      localStorage.removeItem('topology-box-positions')
      localStorage.removeItem('topology-box-widths')
      saveLayoutLocal(d)
      return d
    }
  } catch {
    /* 迁移失败忽略 */
  }
  return { hash: '', positions: {}, widths: {} }
}

function saveLayoutLocal(d: LayoutData) {
  try {
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(d))
  } catch {
    /* 忽略写入失败 */
  }
}

// 内容哈希（djb2，key 排序保证 JSON 稳定）：用于「本地版本 vs 云端版本」去重判断
function sortKeys(o: unknown): unknown {
  if (Array.isArray(o)) return o.map(sortKeys)
  if (o && typeof o === 'object') {
    const out: Record<string, unknown> = {}
    for (const k of Object.keys(o as Record<string, unknown>).sort()) out[k] = sortKeys((o as Record<string, unknown>)[k])
    return out
  }
  return o
}

function contentHash(positions: Record<string, unknown>, widths: Record<string, unknown>): string {
  const s = JSON.stringify({ positions: sortKeys(positions), widths: sortKeys(widths) })
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return h.toString(36)
}

const localLayout = loadLayoutLocal()
const boxPositions = new Map<string, { x: number; y: number }>(Object.entries(localLayout.positions))
const boxWidths = new Map<string, number>(Object.entries(localLayout.widths))
const layoutDirty = ref(false)

// 云端同步：打开时拉取，hash 不一致 → 下载覆盖本地；一致 → 复用本地
onMounted(async () => {
  try {
    const { data } = await getTopologyLayout()
    if (data.code !== 0) return
    const srv = data.data ?? { hash: '', positions: {}, widths: {} }
    if (srv.hash === localLayout.hash) return // 版本一致 → 复用本地布局
    boxPositions.clear()
    boxWidths.clear()
    for (const [k, v] of Object.entries(srv.positions ?? {})) {
      if (v && typeof v.x === 'number' && typeof v.y === 'number') boxPositions.set(k, { x: v.x, y: v.y })
    }
    for (const [k, v] of Object.entries(srv.widths ?? {})) {
      if (typeof v === 'number') boxWidths.set(k, v)
    }
    localLayout.hash = srv.hash ?? ''
    localLayout.positions = Object.fromEntries(boxPositions)
    localLayout.widths = Object.fromEntries(boxWidths)
    saveLayoutLocal(localLayout)
    layoutDirty.value = false
    if (props.topology) buildGraph(props.topology)
  } catch {
    /* 拉取失败用本地布局 */
  }
})

// 「保存布局」按钮：算内容哈希上传云端，成功后更新本地 hash
async function saveLayoutToCloud() {
  const positions = Object.fromEntries(boxPositions)
  const widths = Object.fromEntries(boxWidths)
  const hash = contentHash(positions, widths)
  try {
    const { data } = await saveTopologyLayout({ hash, positions, widths })
    if (data.code === 0) {
      localLayout.hash = hash
      localLayout.positions = positions
      localLayout.widths = widths
      saveLayoutLocal(localLayout)
      layoutDirty.value = false
      ElMessage.success('布局已保存到云端')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  }
}

const ROW_H = 40
const HEADER_H = 48
const TITLE_H = 30
const DEFAULT_BOX_W = 440

function getBoxWidth(serverId: number | string): number {
  const id = typeof serverId === 'string' && serverId.startsWith('server-') ? serverId : `server-${serverId}`
  return boxWidths.get(id) ?? DEFAULT_BOX_W
}

function boxHeight(inbCount: number, outCount: number) {
  return HEADER_H + TITLE_H + Math.max(Math.max(inbCount, outCount), 1) * ROW_H + 16
}

function nodePosOf(s: TopologyData['servers'][number], idx: number) {
  return boxPositions.get(`server-${s.id}`) ?? { x: 40 + idx * 520, y: 24 }
}

function typeInfo(t?: string) {
  if (t === 'relay') return { cls: 'relay', text: '转发' }
  if (t === 'idle') return { cls: 'idle', text: '闲置' }
  return { cls: 'user', text: '用户' }
}

// 快速切换入站三态 (user -> relay -> idle -> user)
async function cycleInboundType(inb: InboundItem) {
  if (!props.editable) return
  const order = ['user', 'relay', 'idle']
  const curIdx = order.indexOf(inb.type || 'user')
  const nextType = order[(curIdx + 1) % order.length]
  const labels: Record<string, string> = { user: '用户入站', relay: '转发入站 (relay)', idle: '闲置 (idle)' }
  try {
    const { data } = await updateInbound(inb.id, { type: nextType })
    if (data.code === 0) {
      ElMessage.success(`入站「${inb.tag}」已切换为 ${labels[nextType]}`)
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '切换类型失败'))
  }
}

// 提取入站传输层与安全层摘要标签
function inboundSummary(inb: TopologyData['inbounds'][number]) {
  let net = 'TCP'
  let sec = ''
  try {
    if (inb.stream_settings) {
      const s = JSON.parse(inb.stream_settings)
      if (s.network) net = String(s.network).toUpperCase()
      if (s.security) {
        const secUpper = String(s.security).toUpperCase()
        if (secUpper === 'REALITY') sec = 'REALITY'
        else if (secUpper === 'TLS') sec = 'TLS'
      }
    }
  } catch {
    /* ignore json parse error */
  }
  return {
    port: inb.port,
    net,
    sec,
  }
}

// Caddy/Nginx 认知层：按域名/SNI 对 XHTTP 入站进行聚合
export interface CaddyGroup {
  domain: string
  inbounds: InboundItem[]
}

function getCaddyGrouping(inbounds: InboundItem[]): { caddyGroups: CaddyGroup[]; nativeInbounds: InboundItem[] } {
  const caddyInbs: InboundItem[] = []
  const nativeInbs: InboundItem[] = []

  for (const inb of inbounds) {
    const summary = inboundSummary(inb)
    const isXhttp = inb.protocol === 'vless' && (summary.net === 'XHTTP' || inb.stream_settings?.includes('"network":"xhttp"'))
    if (isXhttp) {
      caddyInbs.push(inb)
    } else {
      nativeInbs.push(inb)
    }
  }

  const map = new Map<string, InboundItem[]>()
  for (const inb of caddyInbs) {
    let domain = inb.share_sni || inb.share_addr || ''
    if (!domain) {
      try {
        const s = JSON.parse(inb.stream_settings || '{}')
        domain = s.tlsSettings?.serverName || ''
      } catch {}
    }
    if (!domain) domain = '默认域名 (Host)'
    if (!map.has(domain)) map.set(domain, [])
    map.get(domain)!.push(inb)
  }

  const caddyGroups: CaddyGroup[] = Array.from(map.entries()).map(([domain, inbs]) => ({ domain, inbounds: inbs }))
  return { caddyGroups, nativeInbounds: nativeInbs }
}

function copyCaddySnippet(group: CaddyGroup, srvHost: string) {
  const domain = group.domain !== '默认域名 (Host)' ? group.domain : srvHost || 'node.example.com'
  const lines = [`# === Caddyfile 示例 (${domain}) ===`, `${domain} {`]
  for (const inb of group.inbounds) {
    let path = inb.share_path || ''
    if (!path) {
      try {
        const s = JSON.parse(inb.stream_settings || '{}')
        path = s.xhttpSettings?.path || '/xhttp'
      } catch {
        path = '/xhttp'
      }
    }
    lines.push(`    # 入站 [${inb.tag}] 转发至本地 Xray 端口`)
    lines.push(`    reverse_proxy ${path} 127.0.0.1:${inb.port}`)
  }
  lines.push(`}`)
  const snippet = lines.join('\n')
  navigator.clipboard.writeText(snippet)
  ElMessage.success(`已复制 ${domain} 的 Caddyfile 配置片段`)
}

// 盒内路由线：S 形贝塞尔
function boxRulePath(sx: number, sy: number, tx: number, ty: number) {
  const mx = (sx + tx) / 2
  return `M ${sx} ${sy} C ${mx} ${sy}, ${mx} ${ty}, ${tx} ${ty}`
}

// 跨盒引用线：全向平滑 S 形贝塞尔走线（右出左入，遇中间盒子阻挡自动下垂 U 形绕行）
function refEdgePath(sx: number, sy: number, tx: number, ty: number, detour: boolean, drop: number) {
  if (detour && drop > 0) {
    const c1x = sx + 60
    const c2x = tx - 60
    const midX = (sx + tx) / 2
    return `M ${sx} ${sy} C ${c1x} ${sy}, ${c1x} ${drop}, ${midX} ${drop} C ${c2x} ${drop}, ${c2x} ${ty}, ${tx} ${ty}`
  }

  const dx = tx - sx
  const dy = ty - sy

  if (dx >= 40) {
    // 1. 标准正向走线（源在左，目标在右）：丝滑水平 S 形贝塞尔曲线
    const curvature = Math.max(40, dx * 0.45)
    return `M ${sx} ${sy} C ${sx + curvature} ${sy}, ${tx - curvature} ${ty}, ${tx} ${ty}`
  }

  // 2. 反向/垂直堆叠走线（目标在源的左方、同列或垂直堆叠）：
  // 严格保持「右侧水平向右引出 -> 大 S 弧度平滑过渡 -> 左侧水平从左接入」的接线感，彻底杜绝斜切穿模
  const offset = Math.max(60, Math.abs(dy) * 0.4, Math.abs(dx) * 0.3)
  let c1y = sy
  let c2y = ty
  if (Math.abs(dy) < 30) {
    // 水平高度接近时略向上弯曲
    c1y = sy - 40
    c2y = ty - 40
  }
  return `M ${sx} ${sy} C ${sx + offset} ${c1y}, ${tx - offset} ${c2y}, ${tx} ${ty}`
}

// 动态重算所有跨盒引用边的阻挡与绕行高度（支持动态 boxWidth）
function recalcDetours() {
  if (!props.topology) return
  const data = props.topology
  const idxByServer = new Map(data.servers.map((s, i) => [s.id, i]))
  const inbByServer = new Map<number, TopologyData['inbounds']>()
  for (const inb of data.inbounds) {
    if (!inbByServer.has(inb.server_id)) inbByServer.set(inb.server_id, [])
    inbByServer.get(inb.server_id)!.push(inb)
  }
  const outByServer = new Map<number, BoxOutbound[]>()
  for (const out of data.outbounds) {
    if (out.protocol === 'blackhole') continue
    if (!outByServer.has(out.server_id)) outByServer.set(out.server_id, [])
    outByServer.get(out.server_id)!.push({ ...out, id: String(out.id) })
  }

  const rawEdges = edges.value as unknown as Edge[]
  for (const edge of rawEdges) {
    if (!edge.id.startsWith('ref-')) continue
    const outboundId = Number(edge.id.slice(4))
    const out = data.outbounds.find((o) => o.id === outboundId)
    if (!out || !out.inbound_ref) continue
    const inb = data.inbounds.find((i) => i.id === out.inbound_ref)
    if (!inb) continue

    const srcSrv = data.servers.find((s) => s.id === out.server_id)
    const tgtSrv = data.servers.find((s) => s.id === inb.server_id)
    if (!srcSrv || !tgtSrv) continue

    const srcPos = nodePosOf(srcSrv, idxByServer.get(out.server_id) ?? 0)
    const tgtPos = nodePosOf(tgtSrv, idxByServer.get(inb.server_id) ?? 0)
    const srcW = getBoxWidth(srcSrv.id)

    const outList = outByServer.get(out.server_id) ?? []
    const srcRow = outList.findIndex((o) => o.id === String(out.id))
    const sx = srcPos.x + srcW
    const sy = srcPos.y + HEADER_H + TITLE_H + srcRow * ROW_H + ROW_H / 2

    const inbList = inbByServer.get(inb.server_id) ?? []
    const tgtRow = inbList.findIndex((i) => i.id === inb.id)
    const tx = tgtPos.x
    const ty = tgtPos.y + HEADER_H + TITLE_H + tgtRow * ROW_H + ROW_H / 2

    let detour = false
    let blockBottom = 0
    for (const s2 of data.servers) {
      if (s2.id === out.server_id || s2.id === inb.server_id) continue
      const p2 = nodePosOf(s2, idxByServer.get(s2.id) ?? 0)
      const w2 = getBoxWidth(s2.id)
      const h2 = boxHeight(inbByServer.get(s2.id)?.length ?? 0, (outByServer.get(s2.id)?.length ?? 0) + 1)
      if (
        p2.x < Math.max(sx, tx) &&
        p2.x + w2 > Math.min(sx, tx) &&
        p2.y < Math.max(sy, ty) &&
        p2.y + h2 > Math.min(sy, ty)
      ) {
        detour = true
        blockBottom = Math.max(blockBottom, p2.y + h2)
      }
    }
    const drop = detour ? Math.max(blockBottom + 50, Math.max(sy, ty) + 140) : 0
    edge.data = { detour, drop }
  }
}

function buildGraph(data: TopologyData) {
  const inbByServer = new Map<number, TopologyData['inbounds']>()
  for (const inb of data.inbounds) {
    if (!inbByServer.has(inb.server_id)) inbByServer.set(inb.server_id, [])
    inbByServer.get(inb.server_id)!.push(inb)
  }

  const endpointsMap = new Map<number, InboundEndpoint[]>()
  if (data.inbound_endpoints) {
    for (const ep of data.inbound_endpoints) {
      if (!endpointsMap.has(ep.inbound_id)) endpointsMap.set(ep.inbound_id, [])
      endpointsMap.get(ep.inbound_id)!.push(ep)
    }
  }

  const outByServer = new Map<number, BoxOutbound[]>()
  for (const out of data.outbounds) {
    if (!outByServer.has(out.server_id)) outByServer.set(out.server_id, [])
    const isDirect = (out.tag === 'direct' || out.protocol === 'freedom') && !out.inbound_ref
    outByServer.get(out.server_id)!.push({
      ...out,
      id: String(out.id),
      virtual: isDirect,
    })
  }

  nodes.value = data.servers.map((s, idx) => ({
    id: `server-${s.id}`,
    type: 'serverbox',
    position: nodePosOf(s, idx),
    data: {
      server: s,
      inbounds: inbByServer.get(s.id) || [],
      endpointsMap,
      outbounds: outByServer.get(s.id) || [],
      boxWidth: getBoxWidth(s.id),
    } as BoxData,
  })) as unknown as GraphNode[]

  const inbNode = new Map<number, string>()
  const outNode = new Map<number, string>()
  for (const s of data.servers) {
    for (const inb of inbByServer.get(s.id) ?? []) inbNode.set(inb.id, `server-${s.id}`)
    for (const out of data.outbounds) {
      if (out.server_id === s.id) outNode.set(out.id, `server-${s.id}`)
    }
  }

  const es: Edge[] = []
  // InboundRef 跨盒实线
  for (const out of data.outbounds) {
    if (out.protocol === 'blackhole' || !out.inbound_ref || !inbNode.has(out.inbound_ref)) continue
    const inb = data.inbounds.find((i) => i.id === out.inbound_ref)
    if (!inb) continue

    es.push({
      id: `ref-${out.id}`,
      source: outNode.get(out.id)!,
      sourceHandle: `out-src-${out.id}`,
      target: inbNode.get(out.inbound_ref)!,
      targetHandle: `inb-tgt-${out.inbound_ref}`,
      type: 'refedge',
      animated: true,
      markerEnd: { type: MarkerType.ArrowClosed },
      data: { detour: false, drop: 0 },
    })
  }

  // 路由规则盒内虚线
  for (const rule of data.routing_rules) {
    if (!rule.inbound_tag || !rule.enabled) continue
    const inb = data.inbounds.find((i) => i.tag === rule.inbound_tag && i.server_id === rule.server_id)
    if (!inb) continue
    const out = data.outbounds.find((o) => o.tag === rule.outbound_tag && o.server_id === rule.server_id)
    if (!out) continue
    es.push({
      id: `rule-${rule.id}`,
      source: `server-${rule.server_id}`,
      sourceHandle: `inb-src-${inb.id}`,
      target: `server-${rule.server_id}`,
      targetHandle: `out-tgt-${out.id}`,
      type: 'boxrule',
      markerEnd: { type: MarkerType.ArrowClosed },
    })
  }
  edges.value = es
  recalcDetours()
}

watch(
  () => props.topology,
  (t) => {
    if (t) buildGraph(t)
  },
  { immediate: true },
)

// ---- 交互：拖线实时合法性预检 ----
function isValidConnection(conn: Connection): boolean {
  if (!props.editable || !props.topology) return false
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const outSrc = src.match(/^out-src-(\d+)$/)
  const inbSrcExt = src.match(/^inb-src-ext-(\d+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const inbAny = tgt.match(/^(?:inb-src-ext|inb-tgt)-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(\d+)$/)

  if (outSrc && inbAny) {
    const out = props.topology.outbounds.find((o) => o.id === Number(outSrc[1]))
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbAny[1]))
    return !!(out && inb && out.server_id !== inb.server_id)
  }
  if (inbSrcExt && inbAny) {
    const inb1 = props.topology.inbounds.find((i) => i.id === Number(inbSrcExt[1]))
    const inb2 = props.topology.inbounds.find((i) => i.id === Number(inbAny[1]))
    return !!(inb1 && inb2 && inb1.server_id !== inb2.server_id)
  }
  if (inbSrc && outAny) {
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbSrc[1]))
    const out = props.topology.outbounds.find((o) => o.id === Number(outAny[1]))
    return !!(inb && out && inb.server_id === out.server_id)
  }
  return false
}

// ---- 交互：拖线建连接 ----
async function handleConnect(conn: Connection) {
  if (!props.editable || !props.topology) return
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const outSrc = src.match(/^out-src-(\d+)$/)
  const inbSrcExt = src.match(/^inb-src-ext-(\d+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const inbAny = tgt.match(/^(?:inb-src-ext|inb-tgt)-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(\d+)$/)

  if (outSrc && inbAny) {
    await createRef(Number(outSrc[1]), Number(inbAny[1]))
  } else if (inbSrcExt && inbAny) {
    await createViaOutbound(Number(inbSrcExt[1]), Number(inbAny[1]))
  } else if (inbSrc && outAny) {
    openRuleDialog(Number(inbSrc[1]), Number(outAny[1]))
  } else if (inbSrc && inbAny) {
    ElMessage.warning('盒内端点仅限服务器内连接（入站 → 出站）；跨服务器请从盒子边缘端点拖出')
  } else {
    ElMessage.warning('仅支持：入站内点 → 出站（盒内规则）、入站边缘点 → 他服务器入站（自动建中转出站）、出站边缘点 → 入站（设置引用）')
  }
}

// 入站 → 他服务器入站：自动创建「via 出站」+ 路由规则
async function createViaOutbound(srcInboundId: number, tgtInboundId: number) {
  const data = props.topology!
  const srcInb = data.inbounds.find((i) => i.id === srcInboundId)
  const tgtInb = data.inbounds.find((i) => i.id === tgtInboundId)
  if (!srcInb || !tgtInb) return
  if (srcInb.server_id === tgtInb.server_id) {
    ElMessage.warning('同服务器入站请拖到本盒「出站」（创建路由规则）')
    return
  }
  const srcSrv = data.servers.find((s) => s.id === srcInb.server_id)
  const tgtSrv = data.servers.find((s) => s.id === tgtInb.server_id)
  if (!srcSrv || !tgtSrv) return

  let tag = `via-${tgtInb.tag}`
  const sameServerOuts = data.outbounds.filter((o) => o.server_id === srcInb.server_id)
  let n = 2
  while (sameServerOuts.some((o) => o.tag === tag)) tag = `via-${tgtInb.tag}-${n++}`
  const promoteIdle = tgtInb.type === 'idle'
  try {
    await ElMessageBox.confirm(
      `将自动完成中转配置：\n` +
        (promoteIdle ? `⓪ 将「${tgtInb.tag}」由闲置自动改为 relay（落地转发）\n` : '') +
        `① 在「${srcSrv.name}」创建出站「${tag}」（vless 引用 ${tgtSrv.name}/${tgtInb.tag} 落地，地址/端口/UUID 自动构造）\n` +
        `② 创建路由规则：${srcInb.tag} → ${tag}`,
      '自动建出站 + 路由',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    if (promoteIdle) {
      const { data: r0 } = await updateInbound(tgtInb.id, { type: 'relay' })
      if (r0.code !== 0) {
        ElMessage.error(r0.message)
        return
      }
    }
    const { data: r1 } = await createServerOutbound(srcInb.server_id, {
      tag,
      protocol: 'vless',
      inbound_ref: tgtInb.id,
      enabled: true,
    })
    if (r1.code !== 0) {
      ElMessage.error(r1.message)
      return
    }
    const { data: r2 } = await createServerRoutingRule(srcInb.server_id, {
      inbound_tag: srcInb.tag,
      outbound_tag: tag,
    })
    if (r2.code !== 0) {
      ElMessage.warning(`出站「${tag}」已创建，但路由规则创建失败：${r2.message}`)
    } else {
      ElMessage.success('出站与路由规则已创建，配置将自动更新')
    }
    emit('changed')
  } catch (e) {
    ElMessage.error(errMsg(e, '创建失败'))
  }
}

async function createRef(outboundId: number, inboundId: number) {
  const data = props.topology!
  const out = data.outbounds.find((o) => o.id === outboundId)
  const inb = data.inbounds.find((i) => i.id === inboundId)
  if (!out || !inb) return
  if (out.inbound_ref === inboundId) {
    ElMessage.info('该出站已引用此入站')
    return
  }
  const inbServer = data.servers.find((s) => s.id === inb.server_id)?.name ?? `#${inb.server_id}`
  const promoteIdle = inb.type === 'idle'
  try {
    await ElMessageBox.confirm(
      `将出站「${out.tag}」设为引用落地入站「${inb.tag}」（${inbServer}）？\n` +
        (promoteIdle ? `「${inb.tag}」当前为闲置，将自动改为 relay（落地转发）后建立引用。\n` : '') +
        'vnext 地址/端口/UUID/传输参数由主控自动构造，无需手填。',
      '创建 InboundRef 引用',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    if (promoteIdle) {
      const { data: r0 } = await updateInbound(inb.id, { type: 'relay' })
      if (r0.code !== 0) {
        ElMessage.error(r0.message)
        return
      }
    }
    const { data: resp } = await updateServerOutbound(out.server_id, out.id, { inbound_ref: inboundId })
    if (resp.code === 0) {
      ElMessage.success('引用已建立，配置将自动更新')
      emit('changed')
    } else {
      ElMessage.error(resp.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建引用失败'))
  }
}

// ---- 附加接入点抽屉 ----
const epDrawerOpen = ref(false)
const epDrawerInbound = ref<InboundItem | null>(null)
const epDrawerServerName = ref('')

function openEndpointsManager(inb: InboundItem, serverName: string) {
  epDrawerInbound.value = inb
  epDrawerServerName.value = serverName
  epDrawerOpen.value = true
}

// ---- 出站编辑器弹窗 ----
const outboundEditorOpen = ref(false)
const outboundServerId = ref(0)
const outboundEditing = ref<ServerOutbound | null>(null)

function openCreateOutbound(serverId: number) {
  outboundServerId.value = serverId
  outboundEditing.value = null
  outboundEditorOpen.value = true
}

function handleOutboundSaved() {
  outboundEditorOpen.value = false
  emit('changed')
}

// ---- 入站新建事件触发 ----
function openCreateInbound(serverId: number) {
  emit('open-create-inbound', serverId)
}

// ---- 交互：路由规则弹窗 ----
const ruleOpen = ref(false)
const ruleSaving = ref(false)
const ruleForm = reactive({
  serverId: 0,
  inboundTag: '',
  outboundTag: '',
  domain: '',
  ip: '',
  protocol: '',
  port: '',
  network: '',
  priority: 0,
})

function openRuleDialog(inboundId: number, outboundId: number) {
  const data = props.topology!
  const inb = data.inbounds.find((i) => i.id === inboundId)
  const out = data.outbounds.find((o) => o.id === outboundId)
  if (!inb || !out) return
  if (inb.server_id !== out.server_id) {
    ElMessage.warning('路由规则只能在同一服务器内（入站 → 出站）')
    return
  }
  ruleForm.serverId = inb.server_id
  ruleForm.inboundTag = inb.tag
  ruleForm.outboundTag = out.tag
  ruleForm.domain = ''
  ruleForm.ip = ''
  ruleForm.protocol = ''
  ruleForm.port = ''
  ruleForm.network = ''
  ruleForm.priority = 0
  ruleOpen.value = true
}

async function saveRule() {
  ruleSaving.value = true
  try {
    const { data } = await createServerRoutingRule(ruleForm.serverId, {
      inbound_tag: ruleForm.inboundTag,
      outbound_tag: ruleForm.outboundTag,
      domain: ruleForm.domain || undefined,
      ip: ruleForm.ip || undefined,
      protocol: ruleForm.protocol || undefined,
      port: ruleForm.port || undefined,
      network: ruleForm.network || undefined,
      priority: ruleForm.priority,
    })
    if (data.code === 0) {
      ElMessage.success('路由规则已创建')
      ruleOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建规则失败'))
  } finally {
    ruleSaving.value = false
  }
}

// ---- 交互：删线 ----
function deselectEdge(id: string) {
  const raw = edges.value as unknown as { id: string; selected?: boolean }[]
  const edge = raw.find((e) => e.id === id)
  if (edge) {
    edge.selected = false
    edges.value = raw.slice() as unknown as Edge[]
  }
}

async function handleEdgeClick(evt: EdgeMouseEvent) {
  const edge = evt.edge
  if (!props.editable || !props.topology) return
  if (edge.id.startsWith('ref-')) {
    const outboundId = Number(edge.id.slice(4))
    const out = props.topology.outbounds.find((o) => o.id === outboundId)
    if (!out) return
    if (out.tag.startsWith('via-')) {
      try {
        await ElMessageBox.confirm(
          `删除自动创建的出站「${out.tag}」及其路由规则？目标落地入站将回退到引用前类型。`,
          '删除中转出站',
          { type: 'warning' },
        )
      } catch {
        deselectEdge(edge.id)
        return
      }
      try {
        const { data } = await deleteServerOutbound(out.server_id, out.id)
        if (data.code === 0) {
          ElMessage.success('已删除中转出站与路由规则')
          emit('changed')
        } else {
          ElMessage.error(data.message)
          deselectEdge(edge.id)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '删除失败'))
        deselectEdge(edge.id)
      }
      return
    }
    try {
      await ElMessageBox.confirm(
        `解除出站「${out.tag}」的 InboundRef 引用？目标落地入站在无其他引用时将回退到引用前类型。`,
        '解除引用',
        { type: 'warning' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await updateServerOutbound(out.server_id, out.id, { inbound_ref: 0 })
      if (data.code === 0) {
        ElMessage.success('已解除引用')
        emit('changed')
      } else {
        ElMessage.error(data.message)
        deselectEdge(edge.id)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '解除引用失败'))
      deselectEdge(edge.id)
    }
  } else if (edge.id.startsWith('rule-')) {
    const ruleId = Number(edge.id.slice(5))
    const rule = props.topology.routing_rules.find((r) => r.id === ruleId)
    if (!rule) return
    try {
      await ElMessageBox.confirm(
        `删除路由规则（${rule.inbound_tag} → ${rule.outbound_tag}）？规则包含的匹配字段将一并删除。`,
        '删除路由规则',
        { type: 'error' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await deleteServerRoutingRule(rule.server_id, ruleId)
      if (data.code === 0) {
        ElMessage.success('已删除')
        emit('changed')
      } else {
        ElMessage.error(data.message)
        deselectEdge(edge.id)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '删除规则失败'))
      deselectEdge(edge.id)
    }
  }
}

// 自定义横向拖拉伸逻辑
function startBoxResize(e: MouseEvent, data: BoxData) {
  const nodeId = `server-${data.server.id}`
  const startX = e.clientX
  const startWidth = data.boxWidth || DEFAULT_BOX_W
  const onMouseMove = (moveEvent: MouseEvent) => {
    const deltaX = moveEvent.clientX - startX
    const w = Math.max(340, Math.min(800, startWidth + deltaX))
    data.boxWidth = w
    boxWidths.set(nodeId, w)
    localLayout.widths = Object.fromEntries(boxWidths)
    saveLayoutLocal(localLayout)
  }
  const onMouseUp = () => {
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
    layoutDirty.value = true
    recalcDetours()
  }
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

// 双击盒子 → 打开服务器抽屉
function handleNodeDblClick(evt: NodeMouseEvent) {
  const box = evt.node.data as BoxData
  if (box?.server?.id) emit('open-server', box.server.id)
}

// 盒子拖动结束：更新本地布局 + 动态重算绕行
function onNodeDragStop(evt: NodeDragEvent) {
  const node = evt.node
  if (node.position) {
    boxPositions.set(node.id, { x: node.position.x, y: node.position.y })
    localLayout.positions = Object.fromEntries(boxPositions)
    saveLayoutLocal(localLayout)
    layoutDirty.value = true
    recalcDetours()
  }
}

// ---- 交互：一键分层自动排版（DAG 拓扑分层）----
function autoLayout() {
  if (!props.topology || props.topology.servers.length === 0) return
  const data = props.topology
  const srvIds = data.servers.map((s) => s.id)
  const upstream = new Map<number, Set<number>>()
  for (const id of srvIds) upstream.set(id, new Set())

  const inbServerMap = new Map<number, number>()
  for (const inb of data.inbounds) inbServerMap.set(inb.id, inb.server_id)

  for (const out of data.outbounds) {
    if (out.inbound_ref && inbServerMap.has(out.inbound_ref)) {
      const targetServerId = inbServerMap.get(out.inbound_ref)!
      if (targetServerId !== out.server_id) {
        upstream.get(targetServerId)?.add(out.server_id)
      }
    }
  }

  const layers = new Map<number, number>()
  function getLayer(id: number, visited = new Set<number>()): number {
    if (layers.has(id)) return layers.get(id)!
    if (visited.has(id)) return 0
    visited.add(id)
    const ups = Array.from(upstream.get(id) ?? [])
    if (ups.length === 0) {
      layers.set(id, 0)
      return 0
    }
    const maxUpLayer = Math.max(...ups.map((u) => getLayer(u, new Set(visited))))
    const layer = maxUpLayer + 1
    layers.set(id, layer)
    return layer
  }

  for (const id of srvIds) getLayer(id)

  const layerBuckets = new Map<number, number[]>()
  for (const id of srvIds) {
    const l = layers.get(id) ?? 0
    if (!layerBuckets.has(l)) layerBuckets.set(l, [])
    layerBuckets.get(l)!.push(id)
  }

  const sortedLayers = Array.from(layerBuckets.keys()).sort((a, b) => a - b)
  let curX = 40
  const GAP_X = 100
  const GAP_Y = 32

  for (const l of sortedLayers) {
    const srvsInLayer = layerBuckets.get(l)!
    let curY = 30
    let maxColW = DEFAULT_BOX_W
    for (const sid of srvsInLayer) {
      const w = getBoxWidth(sid)
      maxColW = Math.max(maxColW, w)
      boxPositions.set(`server-${sid}`, { x: curX, y: curY })
      const inbCount = data.inbounds.filter((i) => i.server_id === sid).length
      const outCount = data.outbounds.filter((o) => o.server_id === sid && o.protocol !== 'blackhole').length + 1
      curY += boxHeight(inbCount, outCount) + GAP_Y
    }
    curX += maxColW + GAP_X
  }

  localLayout.positions = Object.fromEntries(boxPositions)
  saveLayoutLocal(localLayout)
  layoutDirty.value = true

  for (const node of nodes.value) {
    const p = boxPositions.get(node.id)
    if (p) node.position = { ...p }
  }
  recalcDetours()
  ElMessage.success('已完成拓扑分层排版（入口 → 中转 → 落地）')
}

// 重置为默认一字排列
function resetLayout() {
  if (!props.topology) return
  props.topology.servers.forEach((s, idx) => {
    boxPositions.set(`server-${s.id}`, { x: 40 + idx * 520, y: 24 })
    boxWidths.set(`server-${s.id}`, DEFAULT_BOX_W)
  })
  localLayout.positions = Object.fromEntries(boxPositions)
  localLayout.widths = Object.fromEntries(boxWidths)
  saveLayoutLocal(localLayout)
  layoutDirty.value = true

  for (const node of nodes.value) {
    const p = boxPositions.get(node.id)
    if (p) node.position = { ...p }
    if (node.data) (node.data as BoxData).boxWidth = DEFAULT_BOX_W
  }
  recalcDetours()
  ElMessage.success('已重置为默认排列')
}

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value
}

const hasData = computed(() => !!props.topology && props.topology.servers.length > 0)
</script>

<template>
  <div class="topology-wrap" :class="{ 'is-fullscreen': isFullscreen }">
    <!-- 顶部微型操作栏 -->
    <div class="canvas-top-bar">
      <div class="top-bar-left">
        <el-button-group size="small">
          <el-button @click="autoLayout" title="根据拓扑链路（入口→中转→落地）自动分层排版">
            <el-icon><MagicStick /></el-icon>&nbsp;自动排版
          </el-button>
          <el-button @click="resetLayout" title="重置为默认一字排列">
            <el-icon><RefreshRight /></el-icon>&nbsp;重置网格
          </el-button>
        </el-button-group>
        <el-button size="small" class="fs-btn" @click="toggleFullscreen" :title="isFullscreen ? '退出全屏' : '全屏查看'">
          <el-icon><FullScreen /></el-icon>&nbsp;{{ isFullscreen ? '退出全屏' : '全屏' }}
        </el-button>
      </div>

      <!-- 布局未保存提示条 -->
      <div v-if="layoutDirty" class="layout-save-bar">
        <span class="layout-save-tip">布局已修改</span>
        <el-button size="small" type="primary" @click="saveLayoutToCloud">保存布局</el-button>
      </div>
    </div>

    <VueFlow
      v-if="hasData"
      v-model:nodes="nodes"
      v-model:edges="edges"
      class="topology-canvas"
      :nodes-draggable="editable"
      :nodes-connectable="editable"
      :is-valid-connection="isValidConnection"
      :min-zoom="0.15"
      :max-zoom="2"
      :fit-view-on-init="true"
      @connect="handleConnect"
      @edge-click="handleEdgeClick"
      @node-double-click="handleNodeDblClick"
      @node-drag-stop="onNodeDragStop"
    >
      <Background pattern-color="#334155" :gap="18" />
      <Controls position="bottom-left" />

      <!-- 盒内路由线：入站内点 → 出站内点的 S 形贝塞尔虚线 -->
      <template #edge-boxrule="e">
        <path
          class="vue-flow__edge-path vue-flow__edge-interaction edge-hit rule-hit"
          :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)"
        />
        <path class="boxrule-glow" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
        <path class="boxrule-path" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
      </template>

      <!-- 跨盒引用线：直-弧-直（阻挡 U 形绕行）-->
      <template #edge-refedge="e">
        <path
          class="vue-flow__edge-path vue-flow__edge-interaction edge-hit ref-hit"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
        />
        <path
          class="refedge-glow"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
        />
        <path
          class="refedge-path"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
          :marker-end="e.markerEnd"
        />
      </template>

      <template #node-serverbox="nodeProps">
        <div
          class="server-box"
          :class="{ offline: nodeProps.data.server.status !== 1 }"
          :style="{ width: (nodeProps.data.boxWidth || DEFAULT_BOX_W) + 'px' }"
        >
          <div
            v-show="nodeProps.selected"
            class="custom-resizer-right"
            @mousedown.stop.prevent="startBoxResize($event, nodeProps.data)"
          />
          <div class="sb-head">
            <div class="sb-head-left">
              <span class="status-dot" :class="nodeProps.data.server.status === 1 ? 'online' : 'offline'" />
              <span class="name" :title="nodeProps.data.server.name">{{ nodeProps.data.server.name }}</span>
              <span v-if="nodeProps.data.server.status !== 1" class="offline-badge">离线</span>
              <span class="host" :title="nodeProps.data.server.host">{{ nodeProps.data.server.host }}</span>
            </div>
            <div class="sb-head-right">
              <button class="sb-detail-btn" title="查看服务器详情与监控" @click.stop="emit('open-server', nodeProps.data.server.id)">
                <el-icon><Setting /></el-icon>&nbsp;详情
              </button>
            </div>
          </div>

          <div class="sb-cols">
            <!-- 入站列：包含 Caddy 认知层容器与常规入站 -->
            <div class="sb-col">
              <div class="sb-col-head">
                <span class="sb-title">入站 (Inbound)</span>
                <button
                  v-if="editable"
                  class="sb-add-btn"
                  title="为该服务器新建入站"
                  @click.stop="openCreateInbound(nodeProps.data.server.id)"
                >
                  <el-icon><Plus /></el-icon>&nbsp;入站
                </button>
              </div>

              <template v-if="nodeProps.data.inbounds.length > 0">
                <!-- 1. Caddy/Nginx 认知层反代胶囊 (XHTTP 入站按域名聚合) -->
                <div
                  v-for="cg in getCaddyGrouping(nodeProps.data.inbounds).caddyGroups"
                  :key="cg.domain"
                  class="caddy-capsule"
                >
                  <div class="caddy-capsule-head">
                    <div class="caddy-title">
                      <span class="caddy-icon">🌐</span>
                      <span class="caddy-domain" :title="cg.domain">{{ cg.domain }}:443</span>
                    </div>
                    <button
                      class="caddy-copy-btn"
                      title="一键复制 Caddyfile 反代配置片段"
                      @click.stop="copyCaddySnippet(cg, nodeProps.data.server.host)"
                    >
                      <el-icon><CopyDocument /></el-icon>&nbsp;Caddyfile
                    </button>
                  </div>
                  <div class="caddy-capsule-body">
                    <div v-for="inb in cg.inbounds" :key="inb.id" class="sb-row caddy-subrow">
                      <Handle
                        type="target"
                        :id="`inb-tgt-${inb.id}`"
                        :position="Position.Left"
                        :connectable="editable"
                        class="ep ext-tgt"
                      />
                      <Handle
                        type="source"
                        :id="`inb-src-ext-${inb.id}`"
                        :position="Position.Left"
                        :connectable="editable"
                        class="ep ext-src"
                      />
                      <span
                        class="type-tag"
                        :class="typeInfo(inb.type).cls"
                        title="点击快速切换三态 (用户/转发/闲置)"
                        @click.stop="cycleInboundType(inb)"
                      >
                        {{ typeInfo(inb.type).text }}
                      </span>
                      <span class="path-badge" :title="`反代至 127.0.0.1:${inb.port}`">
                        {{ inb.share_path || '/xhttp' }}
                      </span>
                      <span class="port-badge">:{{ inb.port }}</span>
                      <span class="tag" :title="inb.tag">{{ inb.tag }}</span>

                      <!-- 附加接入点胶囊 -->
                      <div class="sb-ep-pills">
                        <span
                          v-for="ep in (nodeProps.data.endpointsMap?.get(inb.id) || [])"
                          :key="ep.id"
                          class="ep-pill"
                          :class="{ disabled: !ep.enabled }"
                          :title="`附加接入点: ${ep.name} (${ep.host}:${ep.port})`"
                          @click.stop="openEndpointsManager(inb, nodeProps.data.server.name)"
                        >
                          🔀 {{ ep.name }}
                        </span>
                        <button
                          v-if="editable"
                          class="ep-add-btn"
                          title="管理/添加附加接入点"
                          @click.stop="openEndpointsManager(inb, nodeProps.data.server.name)"
                        >
                          + 接入点
                        </button>
                      </div>

                      <Handle
                        type="source"
                        :id="`inb-src-${inb.id}`"
                        :position="Position.Right"
                        :connectable="editable"
                        class="ep in-ep"
                      />
                    </div>
                  </div>
                </div>

                <!-- 2. 原生入站行 (TCP REALITY / WebSocket 等) -->
                <div
                  v-for="inb in getCaddyGrouping(nodeProps.data.inbounds).nativeInbounds"
                  :key="inb.id"
                  class="sb-row"
                >
                  <Handle
                    type="target"
                    :id="`inb-tgt-${inb.id}`"
                    :position="Position.Left"
                    :connectable="editable"
                    class="ep ext-tgt"
                  />
                  <Handle
                    type="source"
                    :id="`inb-src-ext-${inb.id}`"
                    :position="Position.Left"
                    :connectable="editable"
                    class="ep ext-src"
                  />
                  <span
                    class="type-tag"
                    :class="typeInfo(inb.type).cls"
                    title="点击快速切换三态 (用户/转发/闲置)"
                    @click.stop="cycleInboundType(inb)"
                  >
                    {{ typeInfo(inb.type).text }}
                  </span>
                  <span class="port-badge">:{{ inb.port }}</span>
                  <span class="tag" :title="inb.tag">{{ inb.tag }}</span>
                  <span v-if="inboundSummary(inb).sec" class="proto-badge sec">{{ inboundSummary(inb).sec }}</span>
                  <span v-else class="proto-badge net">{{ inboundSummary(inb).net }}</span>

                  <!-- 附加接入点胶囊 -->
                  <div class="sb-ep-pills">
                    <span
                      v-for="ep in (nodeProps.data.endpointsMap?.get(inb.id) || [])"
                      :key="ep.id"
                      class="ep-pill"
                      :class="{ disabled: !ep.enabled }"
                      :title="`附加接入点: ${ep.name} (${ep.host}:${ep.port})`"
                      @click.stop="openEndpointsManager(inb, nodeProps.data.server.name)"
                    >
                      🔀 {{ ep.name }}
                    </span>
                    <button
                      v-if="editable"
                      class="ep-add-btn"
                      title="管理/添加附加接入点"
                      @click.stop="openEndpointsManager(inb, nodeProps.data.server.name)"
                    >
                      + 接入点
                    </button>
                  </div>

                  <Handle
                    type="source"
                    :id="`inb-src-${inb.id}`"
                    :position="Position.Right"
                    :connectable="editable"
                    class="ep in-ep"
                  />
                </div>
              </template>
              <div v-else class="sb-empty">暂无入站</div>
            </div>

            <!-- 出站列：盒内点（中线右）= 收盒内规则；边缘外点（右缘）= 发引用线；direct 仅右缘绿点收线 -->
            <div class="sb-col">
              <div class="sb-col-head">
                <span class="sb-title">出站 (Outbound)</span>
                <button
                  v-if="editable"
                  class="sb-add-btn"
                  title="为该服务器新建出站"
                  @click.stop="openCreateOutbound(nodeProps.data.server.id)"
                >
                  <el-icon><Plus /></el-icon>&nbsp;出站
                </button>
              </div>

              <template v-if="nodeProps.data.outbounds.length > 0">
                <div
                  v-for="out in nodeProps.data.outbounds"
                  :key="out.id"
                  class="sb-row"
                  :class="{ 'direct-row': out.virtual }"
                >
                  <!-- 左侧内端接口（所有出站包括 direct 均有，接收盒内路由规则） -->
                  <Handle
                    type="target"
                    :id="`out-tgt-${out.id}`"
                    :position="Position.Left"
                    :connectable="editable"
                    class="ep in-ep"
                  />
                  <span
                    class="tag out-tag"
                    :class="out.inbound_ref ? 'ref' : out.virtual ? 'direct' : out.protocol === 'blackhole' ? 'blocked' : ''"
                    :title="out.tag"
                  >
                    {{ out.tag }}
                  </span>
                  <span v-if="out.inbound_ref" class="out-proto-badge ref">InboundRef</span>
                  <span v-else-if="out.virtual" class="out-proto-badge direct">DIRECT</span>
                  <span v-else-if="out.protocol === 'blackhole'" class="out-proto-badge blocked">BLOCKED</span>
                  <span v-else class="out-proto-badge">{{ out.protocol }}</span>
                  <!-- 右侧外端接口：普通出站发跨盒引用；direct 行显示绿色出口圆点 -->
                  <Handle
                    v-if="!out.virtual"
                    type="source"
                    :id="`out-src-${out.id}`"
                    :position="Position.Right"
                    :connectable="editable"
                    class="ep ext-src"
                  />
                  <span
                    v-else
                    class="ep direct-ep-dot"
                    title="直连出口（freedom）"
                  />
                </div>
              </template>
              <div v-else class="sb-empty">暂无出站</div>
            </div>
          </div>
        </div>
      </template>
    </VueFlow>
    <div v-else class="topology-empty">
      <el-empty description="暂无服务器。先到「服务器」页添加节点，再回来拖线接线。" />
    </div>

    <!-- 附加接入点抽屉 -->
    <InboundEndpointsDrawer
      v-model="epDrawerOpen"
      :inbound="epDrawerInbound"
      :server-name="epDrawerServerName"
      @changed="emit('changed')"
    />

    <!-- 出站新建/编辑弹窗 -->
    <OutboundConfigEditor
      v-if="outboundEditorOpen"
      :server-id="outboundServerId"
      :outbound="outboundEditing"
      @saved="handleOutboundSaved"
      @close="outboundEditorOpen = false"
    />

    <!-- 路由规则弹窗 -->
    <el-dialog v-model="ruleOpen" title="新建路由规则（拖线）" width="540px" append-to-body>
      <el-form label-position="top">
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="入站（源）">
            <el-input :model-value="ruleForm.inboundTag" disabled />
          </el-form-item>
          <el-form-item label="出站（目标）">
            <el-input :model-value="ruleForm.outboundTag" disabled />
          </el-form-item>
        </div>

        <div class="rule-presets">
          <span class="preset-label">快捷填充:</span>
          <el-tag size="small" class="preset-btn" @click="ruleForm.domain = 'geosite:cn'">国内域名 (geosite:cn)</el-tag>
          <el-tag size="small" class="preset-btn" @click="ruleForm.ip = 'geoip:cn,geoip:private'">国内/内网 IP</el-tag>
          <el-tag size="small" class="preset-btn" @click="ruleForm.protocol = 'http,tls'">HTTP/TLS 嗅探</el-tag>
          <el-tag size="small" class="preset-btn" @click="ruleForm.port = '80,443'">Web 端口 (80,443)</el-tag>
        </div>

        <p class="muted tip">匹配字段选填；留空 = 该入站全部流量走目标出站</p>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="域名匹配"><el-input v-model="ruleForm.domain" placeholder="如 google.com（逗号分隔）" /></el-form-item>
          <el-form-item label="IP 匹配"><el-input v-model="ruleForm.ip" placeholder="如 8.8.8.0/24" /></el-form-item>
          <el-form-item label="协议（嗅探）"><el-input v-model="ruleForm.protocol" placeholder="如 tcp,http（逗号分隔）" /></el-form-item>
          <el-form-item label="端口"><el-input v-model="ruleForm.port" placeholder="如 443,80" /></el-form-item>
          <el-form-item label="网络"><el-input v-model="ruleForm.network" placeholder="如 tcp / udp" /></el-form-item>
          <el-form-item label="优先级"><el-input-number v-model="ruleForm.priority" :min="0" style="width: 100%" /></el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="ruleOpen = false">取消</el-button>
        <el-button type="primary" :loading="ruleSaving" @click="saveRule">创建规则</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.topology-wrap {
  height: 640px;
  border: 1px solid var(--x-border);
  border-radius: 10px;
  overflow: hidden;
  background: #0f172a;
  position: relative;
  transition: all 0.25s ease-in-out;

  &.is-fullscreen {
    position: fixed;
    inset: 0;
    width: 100vw;
    height: 100vh;
    z-index: 1500;
    border-radius: 0;
    border: none;
  }
}
.topology-canvas {
  width: 100%;
  height: 100%;
}

/* 顶部微型操作栏 */
.canvas-top-bar {
  position: absolute;
  top: 12px;
  left: 14px;
  right: 14px;
  z-index: 20;
  display: flex;
  justify-content: space-between;
  align-items: center;
  pointer-events: none;

  .top-bar-left {
    display: flex;
    gap: 8px;
    align-items: center;
    pointer-events: auto;
    background: rgba(15, 23, 42, 0.85);
    backdrop-filter: blur(8px);
    padding: 4px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.35);

    .fs-btn {
      margin-left: 2px;
    }
  }
}

/* 布局未保存提示条（右上角浮动） */
.layout-save-bar {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  background: rgba(15, 23, 42, 0.85);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(251, 191, 36, 0.4);
  border-radius: 8px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.35);
  .layout-save-tip {
    font-size: 12px;
    color: #fbbf24;
    font-weight: 500;
  }
}

.topology-empty {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}
.muted {
  color: var(--x-text-3);
}
.tip {
  font-size: 12px;
  margin: 8px 0 12px;
}

.rule-presets {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 6px;
  .preset-label {
    font-size: 12px;
    color: var(--x-text-3);
  }
  .preset-btn {
    cursor: pointer;
    user-select: none;
    transition: all 0.15s;
    &:hover {
      opacity: 0.85;
      transform: translateY(-1px);
    }
  }
}

/* ---- ServerBox 自定义节点 ---- */
.server-box {
  width: 440px;
  min-width: 340px;
  position: relative;
  background: rgba(30, 41, 59, 0.75);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 16px;
  font-size: 12px;
  color: #e2e8f0;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: visible;
  transition: border-color 0.2s, box-shadow 0.2s;

  &.offline {
    border-color: rgba(239, 68, 68, 0.2);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    .sb-head {
      background: linear-gradient(180deg, rgba(239, 68, 68, 0.08) 0%, transparent 100%);
    }
  }

  .sb-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 16px;
    height: 48px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.06) 0%, transparent 100%);
    border-radius: 16px 16px 0 0;

    .sb-head-left {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
    }

    .sb-head-right {
      display: flex;
      align-items: center;
      gap: 6px;
      flex: none;
    }

    .sb-detail-btn {
      display: inline-flex;
      align-items: center;
      background: rgba(255, 255, 255, 0.08);
      border: 1px solid rgba(255, 255, 255, 0.15);
      border-radius: 6px;
      color: #cbd5e1;
      font-size: 11px;
      padding: 3px 8px;
      cursor: pointer;
      transition: all 0.2s;
      &:hover {
        background: rgba(56, 189, 248, 0.2);
        border-color: #38bdf8;
        color: #38bdf8;
      }
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      flex: none;
      &.online {
        background: #34d399;
        box-shadow: 0 0 8px #34d399;
      }
      &.offline {
        background: #ef4444;
        box-shadow: 0 0 6px rgba(239, 68, 68, 0.6);
      }
    }
    .name {
      font-weight: 600;
      font-size: 14px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 140px;
      text-shadow: 0 2px 4px rgba(0, 0, 0, 0.5);
    }
    .offline-badge {
      font-size: 10px;
      padding: 1px 6px;
      border-radius: 4px;
      background: rgba(239, 68, 68, 0.15);
      color: #f87171;
      border: 1px solid rgba(239, 68, 68, 0.3);
      font-weight: 500;
    }
    .host {
      color: #94a3b8;
      font-size: 11px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 140px;
    }
  }

  .sb-cols {
    display: flex;
    gap: 36px;
    justify-content: space-between;
    padding: 0 16px 16px;
    width: 100%;
    box-sizing: border-box;

    .sb-col {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 6px;
      align-items: flex-start;
    }
    .sb-col:last-child {
      align-items: flex-end;
      .sb-col-head {
        flex-direction: row-reverse;
      }
      .sb-title {
        text-align: right;
      }
    }
    .sb-col-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      width: 100%;
      height: 32px;
      padding: 6px 4px 2px;
      box-sizing: border-box;
    }
    .sb-title {
      color: #94a3b8;
      font-size: 12px;
      letter-spacing: 0.5px;
      font-weight: 600;
      text-transform: uppercase;
    }
    .sb-add-btn {
      display: inline-flex;
      align-items: center;
      background: rgba(56, 189, 248, 0.12);
      border: 1px solid rgba(56, 189, 248, 0.25);
      border-radius: 4px;
      color: #38bdf8;
      font-size: 11px;
      padding: 2px 6px;
      cursor: pointer;
      transition: all 0.2s;
      &:hover {
        background: rgba(56, 189, 248, 0.25);
        border-color: #38bdf8;
        transform: translateY(-1px);
      }
    }
  }

  /* Caddy/Nginx 认知层反代网关胶囊 */
  .caddy-capsule {
    width: 100%;
    background: rgba(15, 23, 42, 0.7);
    border: 1px solid rgba(45, 212, 191, 0.25);
    border-radius: 12px;
    padding: 6px 8px 8px;
    box-sizing: border-box;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    display: flex;
    flex-direction: column;
    gap: 6px;

    .caddy-capsule-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 2px 4px 4px;
      border-bottom: 1px dashed rgba(45, 212, 191, 0.2);

      .caddy-title {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 11.5px;
        font-weight: 600;
        color: #2dd4bf;
      }
      .caddy-copy-btn {
        display: inline-flex;
        align-items: center;
        background: rgba(45, 212, 191, 0.15);
        border: 1px solid rgba(45, 212, 191, 0.35);
        border-radius: 4px;
        color: #2dd4bf;
        font-size: 10.5px;
        padding: 1px 6px;
        cursor: pointer;
        transition: all 0.15s;
        &:hover {
          background: rgba(45, 212, 191, 0.3);
          border-color: #2dd4bf;
        }
      }
    }

    .caddy-capsule-body {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }

    .caddy-subrow {
      background: rgba(45, 212, 191, 0.06);
      border-color: rgba(45, 212, 191, 0.15);
      &:hover {
        background: rgba(45, 212, 191, 0.12);
        border-color: rgba(45, 212, 191, 0.3);
      }
    }
  }

  .path-badge {
    font-size: 10px;
    color: #2dd4bf;
    font-family: monospace;
    background: rgba(45, 212, 191, 0.15);
    padding: 1px 5px;
    border-radius: 4px;
    border: 1px solid rgba(45, 212, 191, 0.25);
    flex: none;
  }

  /* 附加接入点小药丸 */
  .sb-ep-pills {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: 2px;

    .ep-pill {
      font-size: 9.5px;
      padding: 1px 5px;
      border-radius: 10px;
      background: rgba(56, 189, 248, 0.15);
      border: 1px solid rgba(56, 189, 248, 0.3);
      color: #38bdf8;
      cursor: pointer;
      white-space: nowrap;
      transition: all 0.15s;
      &:hover {
        background: rgba(56, 189, 248, 0.3);
        transform: translateY(-1px);
      }
      &.disabled {
        opacity: 0.5;
        border-style: dashed;
      }
    }

    .ep-add-btn {
      font-size: 9px;
      padding: 0 4px;
      border-radius: 8px;
      background: transparent;
      border: 1px dashed rgba(255, 255, 255, 0.2);
      color: #94a3b8;
      cursor: pointer;
      transition: all 0.15s;
      &:hover {
        border-color: #38bdf8;
        color: #38bdf8;
      }
    }
  }

  .sb-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 8px;
    height: 34px;
    max-width: 100%;
    box-sizing: border-box;
    background: rgba(15, 23, 42, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 17px;
    transition: all 0.2s;

    &:hover {
      background: rgba(15, 23, 42, 0.85);
      border-color: rgba(255, 255, 255, 0.15);
    }

    .tag {
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 110px;
    }
    .out-tag {
      font-weight: 600;
      color: #93c5fd;
      &.ref {
        color: #fbbf24;
      }
      &.direct {
        color: #34d399;
      }
    }
    &.direct-row {
      background: rgba(52, 211, 153, 0.1);
      border-color: rgba(52, 211, 153, 0.2);
    }

    .port-badge {
      font-size: 11px;
      font-weight: 600;
      color: #38bdf8;
      font-family: monospace;
      flex: none;
    }

    .proto-badge {
      font-size: 9px;
      padding: 1px 4px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      &.sec {
        background: rgba(168, 85, 247, 0.15);
        color: #c084fc;
        border: 1px solid rgba(168, 85, 247, 0.3);
      }
      &.net {
        background: rgba(56, 189, 248, 0.12);
        color: #7dd3fc;
      }
    }

    .out-proto-badge {
      font-size: 9px;
      padding: 1px 5px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      background: rgba(148, 163, 184, 0.15);
      color: #cbd5e1;
      &.ref {
        background: rgba(251, 191, 36, 0.15);
        color: #fbbf24;
        border: 1px solid rgba(251, 191, 36, 0.3);
      }
      &.direct {
        background: rgba(52, 211, 153, 0.15);
        color: #34d399;
        border: 1px solid rgba(52, 211, 153, 0.3);
      }
      &.blocked {
        background: rgba(248, 113, 113, 0.15);
        color: #f87171;
        border: 1px solid rgba(248, 113, 113, 0.3);
      }
    }

    .type-tag {
      flex: none;
      font-size: 10px;
      padding: 1px 5px;
      border-radius: 10px;
      font-weight: 600;
      &.user {
        background: rgba(52, 211, 153, 0.15);
        color: #34d399;
      }
      &.relay {
        background: rgba(251, 191, 36, 0.15);
        color: #fbbf24;
      }
      &.idle {
        background: rgba(148, 163, 184, 0.15);
        color: #94a3b8;
      }
    }
  }

  .sb-empty {
    padding: 4px 10px;
    color: #64748b;
    font-size: 11px;
    height: 34px;
    display: flex;
    align-items: center;
    background: rgba(15, 23, 42, 0.3);
    border-radius: 17px;
    border: 1px dashed rgba(255, 255, 255, 0.05);
  }
}

/* 端点双点结构 */
:deep(.server-box .ep) {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #0f172a;
  border: 2px solid rgba(255, 255, 255, 0.3);
  transition: all 0.2s ease;
  &:hover {
    border-color: #38bdf8;
    background: rgba(56, 189, 248, 0.2);
    box-shadow: 0 0 10px rgba(56, 189, 248, 0.6);
  }
  &.valid,
  &.connecting {
    border-color: #38bdf8;
    background: rgba(56, 189, 248, 0.2);
    box-shadow: 0 0 10px rgba(56, 189, 248, 0.6);
  }
  &.ext-src {
    z-index: 3;
    border-color: rgba(255, 255, 255, 0.4);
  }
  &.ext-tgt {
    z-index: 2;
    opacity: 0;
  }
  &.in-ep {
    border-color: rgba(255, 255, 255, 0.15);
  }
  &.direct-ep-dot {
    position: absolute;
    right: -6px;
    top: 50%;
    transform: translateY(-50%);
    border-color: #34d399;
    box-shadow: 0 0 6px rgba(52, 211, 153, 0.4);
    pointer-events: none;
  }
}

:deep(.vue-flow__edge-path) {
  stroke-width: 2;
}
:deep(.vue-flow__controls button) {
  background: #1e293b;
  border-bottom: 1px solid #334155;
  color: #cbd5e1;
  &:hover {
    background: #273449;
  }
}
:deep(.vue-flow__minimap) {
  background: #0f172a;
}
</style>

<!-- 连线专属样式（限定在 .topology-wrap 作用域内，确保连线始终在节点上方，且防穿透 Dialog） -->
<style>
.topology-wrap .vue-flow__edges {
  z-index: 100 !important;
  pointer-events: none !important;
}

.topology-wrap .vue-flow__nodes {
  z-index: 10 !important;
}

.topology-wrap .vue-flow__node,
.topology-wrap .vue-flow__node.selected {
  z-index: 10 !important;
}

.boxrule-glow {
  stroke: #0284c7;
  stroke-width: 6;
  opacity: 0.18;
  fill: none;
  filter: drop-shadow(0 0 6px rgba(56, 189, 248, 0.3));
  transition:
    stroke 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    stroke-width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none !important;
}
.boxrule-path {
  stroke: #38bdf8;
  stroke-width: 2;
  stroke-dasharray: 6 5;
  fill: none;
  stroke-linecap: round;
  transition:
    stroke 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    stroke-width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none !important;
  animation: dash-flow-slow 30s linear infinite;
}

.refedge-glow {
  stroke: #d97706;
  stroke-width: 8;
  opacity: 0.2;
  fill: none;
  filter: drop-shadow(0 0 8px rgba(245, 158, 11, 0.4));
  transition:
    stroke 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    stroke-width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none !important;
}
.refedge-path {
  stroke: #fbbf24;
  stroke-width: 2.5;
  stroke-linejoin: round;
  stroke-linecap: round;
  fill: none;
  filter: drop-shadow(0 0 3px rgba(251, 191, 36, 0.4));
  transition:
    stroke 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    stroke-width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none !important;
}

.edge-hit {
  stroke: rgba(0, 0, 0, 0.01) !important;
  stroke-width: 24;
  fill: none;
  pointer-events: stroke !important;
  cursor: pointer !important;
}
.edge-hit.ref-hit {
  stroke-width: 28;
}
.edge-hit.rule-hit {
  stroke-width: 24;
}

.boxrule-glow,
.boxrule-path,
.refedge-glow,
.refedge-path {
  pointer-events: none !important;
}

.vue-flow__edge,
.vue-flow__edge-path,
.vue-flow__edge-interaction {
  cursor: pointer !important;
}

.vue-flow__edge:not(.selected):hover .boxrule-glow {
  opacity: 0.38;
  stroke-width: 7;
}
.vue-flow__edge:not(.selected):hover .boxrule-path {
  stroke: #e0f2fe;
  stroke-width: 3;
}
.vue-flow__edge:not(.selected):hover .refedge-glow {
  opacity: 0.42;
  stroke-width: 9;
}
.vue-flow__edge:not(.selected):hover .refedge-path {
  stroke: #fef3c7;
  stroke-width: 3.5;
}

@keyframes dash-flow-slow {
  to {
    stroke-dashoffset: -100;
  }
}

.custom-resizer-right {
  position: absolute;
  right: -3px;
  top: 38px;
  transform: translateY(-50%);
  width: 6px;
  height: 36px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.4);
  z-index: 10;
  cursor: ew-resize;
  transition: all 0.2s;
  &:hover {
    background: rgba(56, 189, 248, 0.8);
    border-color: #38bdf8;
    box-shadow: 0 0 10px rgba(56, 189, 248, 0.5);
  }
}
</style>