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
import { FullScreen, MagicStick, RefreshRight, Plus, CopyDocument, Link, Setting, Delete } from '@element-plus/icons-vue'
import {
  createServerOutbound,
  createServerRoutingRule,
  deleteServerOutbound,
  deleteServerRoutingRule,
  getTopologyLayout,
  saveTopologyLayout,
  updateInbound,
  createInbound,
  updateServerOutbound,
  createL4Rule,
  updateL4Rule,
  deleteL4Rule,
  createAccessPoint,
  updateAccessPoint,
  setAccessPointTarget,
  deleteAccessPoint,
  getPermissionGroups,
  type TopologyData,
} from '@/api/admin'
import type { InboundItem, ServerOutbound, L4PortRule, PermissionGroup, UserAccessPoint } from '@/api/types'
import { errMsg } from '@/api/http'
import OutboundConfigEditor from '@/views/admin/servers/OutboundConfigEditor.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from '@/views/admin/servers/InboundConfigEditor.vue'

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
  l4Rules?: L4PortRule[]
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
  return { cls: 'user', text: '用户' }
}

// 快速切换入站二态 (user <-> relay)
async function cycleInboundType(inb: InboundItem) {
  if (!props.editable) return
  const nextType = inb.type === 'relay' ? 'user' : 'relay'
  const labels: Record<string, string> = { user: '用户入站', relay: '转发入站 (relay)' }
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

  const l4RulesByServer = new Map<number, L4PortRule[]>()
  if (data.l4_rules) {
    for (const r of data.l4_rules) {
      if (!l4RulesByServer.has(r.server_id)) l4RulesByServer.set(r.server_id, [])
      l4RulesByServer.get(r.server_id)!.push(r)
    }
  }

  const serverNodes = data.servers.map((s, idx) => ({
    id: `server-${s.id}`,
    type: 'serverbox',
    position: nodePosOf(s, idx),
    data: {
      server: s,
      inbounds: inbByServer.get(s.id) || [],
      l4Rules: l4RulesByServer.get(s.id) || [],
      outbounds: outByServer.get(s.id) || [],
      boxWidth: getBoxWidth(s.id),
    } as BoxData,
  }))

  // 用户接入点：每个 AP 一个独立小盒（订阅入口），连线方向 AP → 目标入站/L4 转发
  const apNodes = (data.access_points || []).map((ap, idx) => ({
    id: `ap-${ap.id}`,
    type: 'apbox',
    position: boxPositions.get(`ap-${ap.id}`) ?? { x: 40, y: 24 + idx * 170 },
    data: { ap },
  }))

  nodes.value = [...apNodes, ...serverNodes] as unknown as GraphNode[]

  const inbNode = new Map<number, string>()
  const outNode = new Map<number, string>()
  for (const s of data.servers) {
    for (const inb of inbByServer.get(s.id) ?? []) inbNode.set(inb.id, `server-${s.id}`)
    for (const out of data.outbounds) {
      if (out.server_id === s.id) outNode.set(out.id, `server-${s.id}`)
    }
  }

  const es: Edge[] = []

  // 用户接入点连接线（方向：AP → 目标入站 / L4 转发端口，符合「订阅入口指向管道」认知）
  if (data.access_points) {
    for (const ap of data.access_points) {
      if (!ap.enabled) continue
      if (ap.target_type === 'inbound' && ap.target_inbound_id && inbNode.has(ap.target_inbound_id)) {
        es.push({
          id: `ap-${ap.id}`,
          source: `ap-${ap.id}`,
          sourceHandle: `ap-src-${ap.id}`,
          target: inbNode.get(ap.target_inbound_id)!,
          targetHandle: `inb-tgt-${ap.target_inbound_id}`,
          type: 'refedge',
          animated: true,
          markerEnd: { type: MarkerType.ArrowClosed },
          data: { detour: false, drop: 0, isAP: true },
        })
      } else if (ap.target_type === 'l4_rule' && ap.target_l4_rule_id) {
        const l4 = data.l4_rules?.find((r) => r.id === ap.target_l4_rule_id)
        if (l4) {
          es.push({
            id: `ap-${ap.id}`,
            source: `ap-${ap.id}`,
            sourceHandle: `ap-src-${ap.id}`,
            target: `server-${l4.server_id}`,
            targetHandle: `l4-tgt-${l4.id}`,
            type: 'refedge',
            animated: true,
            markerEnd: { type: MarkerType.ArrowClosed },
            data: { detour: false, drop: 0, isAP: true },
          })
        }
      }
    }
  }

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

  // L4 端口转发跨盒连接线
  if (data.l4_rules) {
    for (const r of data.l4_rules) {
      if (!r.enabled || !r.target_inbound_id || !inbNode.has(r.target_inbound_id)) continue
      es.push({
        id: `l4-${r.id}`,
        source: `server-${r.server_id}`,
        sourceHandle: `l4-src-${r.id}`,
        target: inbNode.get(r.target_inbound_id)!,
        targetHandle: `inb-tgt-${r.target_inbound_id}`,
        type: 'refedge',
        animated: true,
        markerEnd: { type: MarkerType.ArrowClosed },
        data: { detour: false, drop: 0, isL4: true },
      })
    }
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
  const apSrc = src.match(/^ap-src-(\d+)$/)
  const outSrc = src.match(/^out-src-(\d+)$/)
  const inbSrcExt = src.match(/^inb-src-ext-(\d+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const l4Src = src.match(/^l4-src-(\d+)$/)
  const inbTgt = tgt.match(/^inb-tgt-(\d+)$/)
  const inbAny = tgt.match(/^(?:inb-src-ext|inb-tgt)-(\d+)$/)
  const l4Tgt = tgt.match(/^l4-tgt-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(\d+)$/)

  // 用户接入点 -> 用户入站（仅限 type=user 物理入站）或 L4 转发端口（订阅入口指向管道）
  if (apSrc && inbTgt) {
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbTgt[1]))
    return !!inb && inb.type === 'user'
  }
  if (apSrc && l4Tgt) return true

  if (l4Src && inbAny) {
    const rule = props.topology.l4_rules?.find((r) => r.id === Number(l4Src[1]))
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbAny[1]))
    return !rule || !inb || rule.server_id !== inb.server_id
  }
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
  const apSrc = src.match(/^ap-src-(\d+)$/)
  const outSrc = src.match(/^out-src-(\d+)$/)
  const inbSrcExt = src.match(/^inb-src-ext-(\d+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const l4Src = src.match(/^l4-src-(\d+)$/)
  const inbTgt = tgt.match(/^inb-tgt-(\d+)$/)
  const inbAny = tgt.match(/^(?:inb-src-ext|inb-tgt)-(\d+)$/)
  const l4Tgt = tgt.match(/^l4-tgt-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(\d+)$/)

  // 用户接入点 → 目标（订阅入口指向管道：直连用户入站 / L4 转发端口）
  if (apSrc) {
    const apId = Number(apSrc[1])
    if (inbTgt) {
      const inbId = Number(inbTgt[1])
      try {
        const { data } = await setAccessPointTarget(apId, { target_type: 'inbound', target_inbound_id: inbId })
        if (data.code === 0) {
          ElMessage.success('已连接接入点至用户入站')
          emit('changed')
        } else ElMessage.error(data.message)
      } catch (e) {
        ElMessage.error(errMsg(e, '连接失败'))
      }
      return
    }
    if (l4Tgt) {
      const l4Id = Number(l4Tgt[1])
      try {
        const { data } = await setAccessPointTarget(apId, { target_type: 'l4_rule', target_l4_rule_id: l4Id })
        if (data.code === 0) {
          ElMessage.success('已连接接入点至 L4 端口转发')
          emit('changed')
        } else ElMessage.error(data.message)
      } catch (e) {
        ElMessage.error(errMsg(e, '连接失败'))
      }
      return
    }
    return
  }
  if (l4Src && inbAny) {
    await connectL4Rule(Number(l4Src[1]), Number(inbAny[1]))
  } else if (outSrc && inbAny) {
    await createRef(Number(outSrc[1]), Number(inbAny[1]))
  } else if (inbSrcExt && inbAny) {
    await createViaOutbound(Number(inbSrcExt[1]), Number(inbAny[1]))
  } else if (inbSrc && outAny) {
    openRuleDialog(Number(inbSrc[1]), Number(outAny[1]))
  } else if (inbSrc && inbAny) {
    ElMessage.warning('盒内端点仅限服务器内连接（入站 → 出站）；跨服务器请从盒子边缘端点拖出')
  } else {
    ElMessage.warning('仅支持：接入点右侧端点 -> 用户入站/L4 端口、L4 端口 -> 目标入站、出站边缘点 -> 入站（设置引用）、入站边缘点 -> 他服务器入站（自动建中转出站）、入站内点 -> 出站（盒内规则）')
  }
}

// 拖线连接 L4 端口规则 -> 目标用户入站
async function connectL4Rule(ruleId: number, targetInboundId: number) {
  const data = props.topology!
  const rule = data.l4_rules?.find((r) => r.id === ruleId)
  const inb = data.inbounds.find((i) => i.id === targetInboundId)
  if (!rule || !inb) return
  const targetServer = data.servers.find((s) => s.id === inb.server_id)
  try {
    await ElMessageBox.confirm(
      `将 L4 转发端口 :${rule.listen_port} 映射至目标「${targetServer?.name || ''}」的入站「${inb.tag}」？\n订阅系统将自动为授权用户派生该端口的中转接入点。`,
      '设置 L4 端口转发目标',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    const { data: resp } = await updateL4Rule(rule.server_id, rule.id, {
      target_server_id: inb.server_id,
      target_inbound_id: inb.id,
      listen_port: rule.listen_port,
      remark: rule.remark,
      enabled: rule.enabled,
    })
    if (resp.code === 0) {
      ElMessage.success('L4 转发映射已更新')
      emit('changed')
    } else {
      ElMessage.error(resp.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '更新映射失败'))
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
  try {
    await ElMessageBox.confirm(
      `将自动完成中转配置：\n` +
        `① 在「${srcSrv.name}」创建出站「${tag}」（vless 引用 ${tgtSrv.name}/${tgtInb.tag} 落地，地址/端口/UUID 自动构造）\n` +
        `② 创建路由规则：${srcInb.tag} → ${tag}`,
      '自动建出站 + 路由',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
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
  try {
    await ElMessageBox.confirm(
      `将出站「${out.tag}」设为引用落地入站「${inb.tag}」（${inbServer}）？\n` +
        'vnext 地址/端口/UUID/传输参数由主控自动构造，无需手填。',
      '创建 InboundRef 引用',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
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

// ---- 用户接入点 (User Access Points) 管理弹窗 ----
const apDialogOpen = ref(false)
const apEditingId = ref(0)
const apSaving = ref(false)
const apTargetServerId = ref(0)
const apTargetL4ServerId = ref(0)

const apForm = reactive({
  name: '',
  custom_host: '',
  custom_port: 0,
  target_type: '' as 'inbound' | 'l4_rule' | '',
  target_inbound_id: undefined as number | undefined,
  target_l4_rule_id: undefined as number | undefined,
  permission_group_ids: [] as number[],
  enabled: true,
  remark: '',
})

function groupName(id: number) {
  return permissionGroups.value.find((g) => g.id === id)?.name ?? `组#${id}`
}

const availableL4Servers = computed(() => {
  return (props.topology?.servers || []).filter((s) => s.server_type === 'l4_relay')
})

const apAvailableInbounds = computed(() => {
  if (!apTargetServerId.value) return []
  return (props.topology?.inbounds || []).filter(
    (i) => i.server_id === apTargetServerId.value && i.enabled && i.type === 'user',
  )
})

const apAvailableL4Rules = computed(() => {
  if (!apTargetL4ServerId.value) return []
  return (props.topology?.l4_rules || []).filter(
    (r) => r.server_id === apTargetL4ServerId.value && r.enabled,
  )
})

async function openCreateAccessPoint() {
  apEditingId.value = 0
  apForm.name = ''
  apForm.custom_host = ''
  apForm.custom_port = 0
  apForm.target_type = ''
  apForm.target_inbound_id = undefined
  apForm.target_l4_rule_id = undefined
  apTargetServerId.value = availableXrayServers.value[0]?.id || 0
  apTargetL4ServerId.value = availableL4Servers.value[0]?.id || 0
  apForm.permission_group_ids = []
  apForm.enabled = true
  apForm.remark = ''
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) permissionGroups.value = data.data.items
  } catch {}
  apDialogOpen.value = true
}

async function openEditAccessPoint(ap: UserAccessPoint) {
  apEditingId.value = ap.id
  apForm.name = ap.name
  apForm.custom_host = ap.custom_host || ''
  apForm.custom_port = ap.custom_port || 0
  apForm.target_type = (ap.target_type || '') as 'inbound' | 'l4_rule' | ''
  apForm.target_inbound_id = ap.target_inbound_id
  apForm.target_l4_rule_id = ap.target_l4_rule_id

  if (ap.target_type === 'inbound' && ap.target_inbound_id) {
    const inb = props.topology?.inbounds.find((i) => i.id === ap.target_inbound_id)
    if (inb) apTargetServerId.value = inb.server_id
  } else {
    apTargetServerId.value = availableXrayServers.value[0]?.id || 0
  }

  if (ap.target_type === 'l4_rule' && ap.target_l4_rule_id) {
    const rule = props.topology?.l4_rules?.find((r) => r.id === ap.target_l4_rule_id)
    if (rule) apTargetL4ServerId.value = rule.server_id
  } else {
    apTargetL4ServerId.value = availableL4Servers.value[0]?.id || 0
  }

  apForm.permission_group_ids = ap.permission_group_ids || []
  apForm.enabled = ap.enabled
  apForm.remark = ap.remark || ''
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) permissionGroups.value = data.data.items
  } catch {}
  apDialogOpen.value = true
}

async function handleSaveAccessPoint() {
  if (!apForm.name.trim()) {
    ElMessage.warning('请填写接入点 Tag 名称')
    return
  }
  apSaving.value = true
  try {
    const payload = {
      name: apForm.name.trim(),
      custom_host: apForm.custom_host.trim(),
      custom_port: apForm.custom_port || 0,
      target_type: apForm.target_type,
      target_inbound_id: apForm.target_type === 'inbound' ? (apForm.target_inbound_id || undefined) : undefined,
      target_l4_rule_id: apForm.target_type === 'l4_rule' ? (apForm.target_l4_rule_id || undefined) : undefined,
      permission_group_ids: apForm.permission_group_ids,
      enabled: apForm.enabled,
      remark: apForm.remark,
    }
    const { data } = apEditingId.value
      ? await updateAccessPoint(apEditingId.value, payload)
      : await createAccessPoint(payload)
    if (data.code === 0) {
      ElMessage.success(apEditingId.value ? '接入点已更新' : '接入点已创建')
      apDialogOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存接入点失败'))
  } finally {
    apSaving.value = false
  }
}

async function handleDeleteAccessPoint(id: number) {
  try {
    await ElMessageBox.confirm('确定删除该用户接入点？', '删除接入点', {
      type: 'warning',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    const { data } = await deleteAccessPoint(id)
    if (data.code === 0) {
      ElMessage.success('接入点已删除')
      apDialogOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- L4 端口转发规则编辑弹窗 ----
const l4RuleOpen = ref(false)
const l4RuleSaving = ref(false)
const l4RuleEditing = ref<L4PortRule | null>(null)
const l4RuleServerId = ref(0)
const permissionGroups = ref<PermissionGroup[]>([])
const l4RuleForm = reactive({
  listen_port: 30001,
  target_server_id: 0,
  target_inbound_id: 0,
  remark: '',
  enabled: true,
})

const availableXrayServers = computed(() => {
  return (props.topology?.servers || []).filter((s) => s.server_type !== 'l4_relay')
})

const availableTargetInbounds = computed(() => {
  if (!l4RuleForm.target_server_id) return []
  return (props.topology?.inbounds || []).filter(
    (i) => i.server_id === l4RuleForm.target_server_id && i.enabled && i.type === 'user',
  )
})

async function openCreateL4Rule(serverId: number) {
  l4RuleServerId.value = serverId
  l4RuleEditing.value = null
  l4RuleForm.listen_port = 30001
  l4RuleForm.target_server_id = availableXrayServers.value[0]?.id || 0
  l4RuleForm.target_inbound_id = 0
  l4RuleForm.remark = ''
  l4RuleForm.enabled = true
  l4RuleOpen.value = true
}

async function openEditL4Rule(rule: L4PortRule) {
  l4RuleServerId.value = rule.server_id
  l4RuleEditing.value = rule
  l4RuleForm.listen_port = rule.listen_port
  l4RuleForm.target_server_id = rule.target_server_id
  l4RuleForm.target_inbound_id = rule.target_inbound_id
  l4RuleForm.remark = rule.remark || ''
  l4RuleForm.enabled = rule.enabled
  l4RuleOpen.value = true
}

async function saveL4Rule() {
  if (!l4RuleForm.listen_port || l4RuleForm.listen_port <= 0 || l4RuleForm.listen_port > 65535) {
    ElMessage.warning('请填写有效的中转监听端口 (1-65535)')
    return
  }
  if (!l4RuleForm.target_inbound_id) {
    ElMessage.warning('请选择目标用户入站')
    return
  }
  l4RuleSaving.value = true
  try {
    const payload = {
      listen_port: l4RuleForm.listen_port,
      target_server_id: l4RuleForm.target_server_id,
      target_inbound_id: l4RuleForm.target_inbound_id,
      remark: l4RuleForm.remark,
      enabled: l4RuleForm.enabled,
    }
    const { data } = l4RuleEditing.value
      ? await updateL4Rule(l4RuleServerId.value, l4RuleEditing.value.id, payload)
      : await createL4Rule(l4RuleServerId.value, payload)
    if (data.code === 0) {
      ElMessage.success(l4RuleEditing.value ? '转发规则已更新' : '转发规则已创建')
      l4RuleOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存转发规则失败'))
  } finally {
    l4RuleSaving.value = false
  }
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

// ---- 入站新建弹窗（直接在画布中唤起） ----
const inboundCreateOpen = ref(false)
const inboundCreateServerId = ref(0)
const inboundChangePayload = ref<InboundEditorChangePayload | null>(null)
const inboundSaving = ref(false)

function openCreateInbound(serverId: number) {
  inboundCreateServerId.value = serverId
  inboundChangePayload.value = null
  inboundCreateOpen.value = true
}

function onInboundEditorChange(payload: InboundEditorChangePayload) {
  inboundChangePayload.value = payload
}

async function handleSaveInbound() {
  const c = inboundChangePayload.value
  if (!c) {
    ElMessage.warning('请先在表单中编辑入站配置')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }
  inboundSaving.value = true
  try {
    const { data } = await createInbound({
      server_id: inboundCreateServerId.value,
      tag: c.tag,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: c.ratio,
      type: 'user',
      flow: c.flow || undefined,
      share_addr_strategy: c.shareAddrStrategy || undefined,
      share_addr: c.shareAddr || undefined,
      share_port: c.sharePort || undefined,
    })
    if (data.code === 0) {
      ElMessage.success('入站已创建')
      inboundCreateOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建入站失败'))
  } finally {
    inboundSaving.value = false
  }
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
  } else if (edge.id.startsWith('ap-')) {
    const apId = Number(edge.id.slice(3))
    const ap = props.topology.access_points?.find((a) => a.id === apId)
    if (!ap) return
    try {
      await ElMessageBox.confirm(
        `解除用户接入点「${ap.name}」的目标连线？`,
        '解除接入点连接',
        { type: 'warning' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await setAccessPointTarget(apId, { target_type: '', target_inbound_id: null, target_l4_rule_id: null })
      if (data.code === 0) {
        ElMessage.success('已解除接入点连接')
        emit('changed')
      } else {
        ElMessage.error(data.message)
        deselectEdge(edge.id)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '解除连接失败'))
      deselectEdge(edge.id)
    }
  } else if (edge.id.startsWith('l4-')) {
    const l4RuleId = Number(edge.id.slice(3))
    const rule = props.topology.l4_rules?.find((r) => r.id === l4RuleId)
    if (!rule) return
    try {
      await ElMessageBox.confirm(
        `删除 L4 端口转发规则（:${rule.listen_port} → ${rule.target_inbound_tag || '目标入站'}）？`,
        '删除 L4 转发规则',
        { type: 'warning' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await deleteL4Rule(rule.server_id, l4RuleId)
      if (data.code === 0) {
        ElMessage.success('已删除转发规则')
        emit('changed')
      } else {
        ElMessage.error(data.message)
        deselectEdge(edge.id)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '删除转发规则失败'))
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
  // 接入点小盒固定最左列，自上而下排布
  const aps = data.access_points || []
  aps.forEach((ap, i) => boxPositions.set(`ap-${ap.id}`, { x: 40, y: 30 + i * 170 }))
  let curX = 40 + (aps.length > 0 ? 280 + 100 : 0)
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
  ElMessage.success('已完成拓扑分层排版（接入点 → 中转 → 落地）')
}

// 重置为默认一字排列
function resetLayout() {
  if (!props.topology) return
  const aps = props.topology.access_points || []
  aps.forEach((ap, i) => boxPositions.set(`ap-${ap.id}`, { x: 40, y: 24 + i * 170 }))
  const offsetX = 40 + (aps.length > 0 ? 280 + 80 : 0)
  props.topology.servers.forEach((s, idx) => {
    boxPositions.set(`server-${s.id}`, { x: offsetX + idx * 520, y: 24 })
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
        <el-button v-if="editable" size="small" type="primary" plain @click="openCreateAccessPoint" title="新建用户接入点（订阅入口）">
          <el-icon><Plus /></el-icon>&nbsp;接入点
        </el-button>
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
          :class="{ 'is-ap': e.data?.isAP, 'is-l4': e.data?.isL4 }"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
        />
        <path
          class="refedge-path"
          :class="{ 'is-ap': e.data?.isAP, 'is-l4': e.data?.isL4 }"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
          :marker-end="e.markerEnd"
        />
      </template>

      <!-- 用户接入点独立小盒（订阅入口；右侧端点拖线指向目标入站 / L4 转发端口） -->
      <template #node-apbox="nodeProps">
        <div
          class="ap-node"
          :class="{ unlinked: !nodeProps.data.ap.target_type, disabled: !nodeProps.data.ap.enabled }"
          @click.stop="openEditAccessPoint(nodeProps.data.ap)"
        >
          <!-- 右侧源端点：拖线至目标入站（inb-tgt）或 L4 转发端口（l4-tgt） -->
          <Handle
            type="source"
            :id="`ap-src-${nodeProps.data.ap.id}`"
            :position="Position.Right"
            :connectable="editable"
            class="ep ap-src-handle"
            title="拖线至目标用户入站或 L4 转发端口"
          />

          <div class="ap-node-head">
            <span class="ap-icon">🌐</span>
            <span class="ap-name-text" :title="nodeProps.data.ap.name">{{ nodeProps.data.ap.name }}</span>
            <span v-if="!nodeProps.data.ap.enabled" class="x-chip gray" style="font-size: 10px; padding: 1px 4px">禁用</span>
          </div>

          <div class="ap-node-perms">
            <template v-if="nodeProps.data.ap.permission_group_ids && nodeProps.data.ap.permission_group_ids.length > 0">
              <span
                v-for="gid in nodeProps.data.ap.permission_group_ids.slice(0, 3)"
                :key="gid"
                class="x-chip blue"
                style="font-size: 10px; padding: 1px 5px"
              >
                {{ groupName(gid) }}
              </span>
              <span v-if="nodeProps.data.ap.permission_group_ids.length > 3" class="x-chip gray" style="font-size: 10px">
                +{{ nodeProps.data.ap.permission_group_ids.length - 3 }}
              </span>
            </template>
            <span v-else class="x-chip orange" style="font-size: 10px">未授权（全员不可见）</span>
          </div>

          <!-- 管道消费的实时端点展示 -->
          <div v-if="nodeProps.data.ap.resolved_host" class="ap-resolved-text">
            <span class="ap-dot online" />
            <code class="cell-mono">{{ nodeProps.data.ap.resolved_host }}:{{ nodeProps.data.ap.resolved_port }}</code>
            <span class="x-chip purple" style="font-size: 10px">
              {{ nodeProps.data.ap.resolved_protocol?.toUpperCase() || 'VLESS' }}
            </span>
          </div>
          <div v-else class="ap-resolved-text waiting">
            <span class="ap-dot offline" />
            <span style="font-size: 11px">待连线：从右侧端点拖出</span>
          </div>

          <div v-if="nodeProps.data.ap.resolved_target_desc" class="ap-desc-text" :title="nodeProps.data.ap.resolved_target_desc">
            {{ nodeProps.data.ap.target_type === 'inbound' ? '直连 ➜' : '中转 ➜' }} {{ nodeProps.data.ap.resolved_target_desc }}
          </div>
        </div>
      </template>

      <template #node-serverbox="nodeProps">
        <!-- 1. L4 纯四层端口转发服务器卡片 -->
        <div
          v-if="nodeProps.data.server.server_type === 'l4_relay'"
          class="server-box l4-relay-box"
          :class="{ offline: nodeProps.data.server.status !== 1 }"
          :style="{ width: (nodeProps.data.boxWidth || DEFAULT_BOX_W) + 'px' }"
        >
          <div
            v-show="nodeProps.selected"
            class="custom-resizer-right"
            @mousedown.stop.prevent="startBoxResize($event, nodeProps.data)"
          />
          <div class="sb-head l4-head">
            <div class="sb-head-left">
              <span class="status-dot online" />
              <span class="l4-relay-badge">L4 中转</span>
              <span class="name" :title="nodeProps.data.server.name">{{ nodeProps.data.server.name }}</span>
              <span class="host" :title="nodeProps.data.server.host">{{ nodeProps.data.server.host }}</span>
            </div>
            <div class="sb-head-right">
              <button class="sb-detail-btn" title="查看服务器详情" @click.stop="emit('open-server', nodeProps.data.server.id)">
                <el-icon><Setting /></el-icon>&nbsp;详情
              </button>
            </div>
          </div>

          <div class="l4-body">
            <div class="sb-col-head" style="margin-bottom: 8px">
              <span class="sb-title">端口转发映射 (Port Rules)</span>
              <button
                v-if="editable"
                class="sb-add-btn l4-add-btn"
                title="为该中转机新建端口转发"
                @click.stop="openCreateL4Rule(nodeProps.data.server.id)"
              >
                <el-icon><Plus /></el-icon>&nbsp;转发端口
              </button>
            </div>

            <div v-if="nodeProps.data.l4Rules && nodeProps.data.l4Rules.length > 0" class="l4-rules-list">
              <div
                v-for="rule in nodeProps.data.l4Rules"
                :key="rule.id"
                class="sb-row l4-rule-row"
                @click.stop="openEditL4Rule(rule)"
              >
                <!-- 左侧 Target Handle: 接收来自 UserAccessPoint 的连线 -->
                <Handle
                  type="target"
                  :id="`l4-tgt-${rule.id}`"
                  :position="Position.Left"
                  :connectable="editable"
                  class="ep ext-tgt l4-tgt-handle"
                  title="接收来自用户接入点的连线"
                />
                <div class="l4-rule-left">
                  <span class="port-badge l4-port">:{{ rule.listen_port }}</span>
                  <span v-if="rule.remark" class="l4-remark">{{ rule.remark }}</span>
                  <span v-else class="l4-remark muted">TCP/UDP 转发</span>
                </div>
                <div class="l4-rule-target">
                  <span v-if="rule.target_inbound_tag" class="tag out-tag ref" :title="`目标入站: ${rule.target_inbound_tag}`">
                    ➜ {{ rule.target_inbound_tag }}
                  </span>
                  <span v-else class="tag out-tag draft" title="未映射：请拖拽右侧端点至目标节点入站">➜ 待连线</span>
                </div>
                <Handle
                  type="source"
                  :id="`l4-src-${rule.id}`"
                  :position="Position.Right"
                  :connectable="editable"
                  class="ep ext-src l4-handle"
                />
              </div>
            </div>
            <div v-else class="sb-empty">暂无端口转发规则，点击右上角「+ 转发端口」创建</div>
          </div>
        </div>

        <!-- 2. 标准 Xray 节点服务器卡片 -->
        <div
          v-else
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
                        title="点击快速切换二态 (用户/转发)"
                        @click.stop="cycleInboundType(inb)"
                      >
                        {{ typeInfo(inb.type).text }}
                      </span>
                      <span class="path-badge" :title="`反代至 127.0.0.1:${inb.port}`">
                        {{ inb.share_path || '/xhttp' }}
                      </span>
                      <span class="port-badge">:{{ inb.port }}</span>
                      <span class="tag" :title="inb.tag">{{ inb.tag }}</span>

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
                    title="点击快速切换二态 (用户/转发)"
                    @click.stop="cycleInboundType(inb)"
                  >
                    {{ typeInfo(inb.type).text }}
                  </span>
                  <span class="port-badge">:{{ inb.port }}</span>
                  <span class="tag" :title="inb.tag">{{ inb.tag }}</span>
                  <span v-if="inboundSummary(inb).sec" class="proto-badge sec">{{ inboundSummary(inb).sec }}</span>
                  <span v-else class="proto-badge net">{{ inboundSummary(inb).net }}</span>

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
                    :class="out.inbound_ref ? 'ref' : out.virtual ? 'direct' : out.protocol === 'blackhole' ? 'blocked' : (out.protocol === 'vless' && !out.inbound_ref) ? 'draft' : ''"
                    :title="out.tag"
                  >
                    {{ (out.protocol === 'vless' && !out.inbound_ref) ? '⚠️ ' + out.tag : out.tag }}
                  </span>
                  <span v-if="out.inbound_ref" class="out-proto-badge ref">InboundRef</span>
                  <span v-else-if="out.virtual" class="out-proto-badge direct">DIRECT</span>
                  <span v-else-if="out.protocol === 'blackhole'" class="out-proto-badge blocked">BLOCKED</span>
                  <span v-else-if="out.protocol === 'vless' && !out.inbound_ref" class="out-proto-badge draft" title="草稿出站：请拖拽右侧端点至目标入站完成绑定">待连线</span>
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

    <!-- 出站新建/编辑弹窗 -->
    <OutboundConfigEditor
      v-if="outboundEditorOpen"
      :server-id="outboundServerId"
      :outbound="outboundEditing"
      @saved="handleOutboundSaved"
      @close="outboundEditorOpen = false"
    />

    <!-- 用户接入点新建/编辑弹窗 (Consumer Pipeline Model) -->
    <el-dialog
      v-model="apDialogOpen"
      :title="apEditingId ? '编辑用户接入点 (Endpoint)' : '新建用户接入点 (Endpoint)'"
      width="580px"
      append-to-body
    >
      <el-form label-position="top">
        <el-alert
          title="用户接入点是面向客户端订阅与分发的入口端点。定义 Tag 名称与开放权限组即可，连接配置沿拓扑管道自适应继承（亦可在下方进行高级覆写）。"
          type="info"
          :closable="false"
          style="margin-bottom: 16px"
        />

        <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0 16px; align-items: start">
          <el-form-item label="接入点 Tag 名称" required>
            <el-input v-model="apForm.name" placeholder="如 🇭🇰 香港直连 01, 🇨🇳 广州移动 BGP" />
          </el-form-item>
          <el-form-item label="启用状态">
            <el-switch v-model="apForm.enabled" active-text="启用" inactive-text="禁用" style="margin-top: 4px" />
          </el-form-item>
        </div>

        <el-form-item label="开放权限组（显式白名单权限控制，勾选可见的用户组）">
          <el-select
            v-model="apForm.permission_group_ids"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="请勾选可见的权限组"
            style="width: 100%"
          >
            <el-option v-for="g in permissionGroups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>

        <!-- 目标绑定 (手动选择 / 拓扑连线) -->
        <el-form-item label="目标绑定方式（亦可在画布上拖拽连线）">
          <el-radio-group v-model="apForm.target_type" style="width: 100%">
            <el-radio-button value="">待连线 / 未绑定</el-radio-button>
            <el-radio-button value="inbound">直连落地入站</el-radio-button>
            <el-radio-button value="l4_rule">L4 端口中转</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 当选择直连落地入站 -->
        <div v-if="apForm.target_type === 'inbound'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; background: rgba(56, 189, 248, 0.05); padding: 12px; border-radius: 8px; margin-bottom: 16px; border: 1px dashed rgba(56, 189, 248, 0.2)">
          <el-form-item label="目标落地服务器" style="margin-bottom: 0">
            <el-select v-model="apTargetServerId" placeholder="选择 Xray 服务器" style="width: 100%" @change="apForm.target_inbound_id = undefined">
              <el-option v-for="s in availableXrayServers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="目标用户入站 (Target Inbound)" style="margin-bottom: 0">
            <el-select v-model="apForm.target_inbound_id" placeholder="选择用户入站" style="width: 100%">
              <el-option v-for="inb in apAvailableInbounds" :key="inb.id" :label="`${inb.tag} (:${inb.port})`" :value="inb.id" />
            </el-select>
          </el-form-item>
        </div>

        <!-- 当选择 L4 端口中转 -->
        <div v-if="apForm.target_type === 'l4_rule'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; background: rgba(168, 85, 247, 0.05); padding: 12px; border-radius: 8px; margin-bottom: 16px; border: 1px dashed rgba(168, 85, 247, 0.2)">
          <el-form-item label="L4 中转服务器" style="margin-bottom: 0">
            <el-select v-model="apTargetL4ServerId" placeholder="选择中转服务器" style="width: 100%" @change="apForm.target_l4_rule_id = undefined">
              <el-option v-for="s in availableL4Servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="端口转发映射 (Port Rule)" style="margin-bottom: 0">
            <el-select v-model="apForm.target_l4_rule_id" placeholder="选择端口规则" style="width: 100%">
              <el-option v-for="r in apAvailableL4Rules" :key="r.id" :label="`:${r.listen_port} ${r.remark ? ' - ' + r.remark : ''} ${r.target_inbound_tag ? '➜ ' + r.target_inbound_tag : ''}`" :value="r.id" />
            </el-select>
          </el-form-item>
        </div>

        <!-- 高级覆写（可选） -->
        <div style="background: rgba(255, 255, 255, 0.02); border: 1px solid rgba(255, 255, 255, 0.08); border-radius: 8px; padding: 12px; margin-bottom: 16px">
          <div style="font-size: 12px; font-weight: 600; color: #94a3b8; margin-bottom: 8px">
            高级自定义覆写（选填，留空则完全自动继承连线机器的 Host 与 Port）
          </div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="自定义连接 Host" style="margin-bottom: 0">
              <el-input v-model="apForm.custom_host" placeholder="留空自动继承" />
            </el-form-item>
            <el-form-item label="自定义连接 Port" style="margin-bottom: 0">
              <el-input-number v-model="apForm.custom_port" :min="0" :max="65535" placeholder="0 自动继承" style="width: 100%" />
            </el-form-item>
          </div>
        </div>

        <el-form-item label="备注说明" style="margin-bottom: 0">
          <el-input v-model="apForm.remark" placeholder="选填，如 VIP 专享中转" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <el-button v-if="apEditingId" type="danger" plain @click="handleDeleteAccessPoint(apEditingId)">
              删除接入点
            </el-button>
          </div>
          <div>
            <el-button @click="apDialogOpen = false">取消</el-button>
            <el-button type="primary" :loading="apSaving" @click="handleSaveAccessPoint">
              保存接入点
            </el-button>
          </div>
        </div>
      </template>
    </el-dialog>

    <!-- 入站新建弹窗（直接在拓扑画布中呼出） -->
    <el-dialog
      v-model="inboundCreateOpen"
      title="为服务器新建入站"
      width="840px"
      append-to-body
      destroy-on-close
    >
      <InboundConfigEditor
        @change="onInboundEditorChange"
      />
      <template #footer>
        <el-button @click="inboundCreateOpen = false">取消</el-button>
        <el-button type="primary" :loading="inboundSaving" @click="handleSaveInbound">
          创建入站
        </el-button>
      </template>
    </el-dialog>

    <!-- L4 端口转发规则编辑弹窗 -->
    <el-dialog v-model="l4RuleOpen" :title="l4RuleEditing ? '编辑 L4 端口转发规则' : '新建 L4 端口转发规则'" width="520px" append-to-body>
      <el-form label-position="top">
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="中转监听端口 ListenPort" required>
            <el-input-number v-model="l4RuleForm.listen_port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="目标落地服务器" required>
            <el-select v-model="l4RuleForm.target_server_id" style="width: 100%" placeholder="选择目标节点" @change="l4RuleForm.target_inbound_id = 0">
              <el-option v-for="s in availableXrayServers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="目标用户入站 (Target Inbound)" required>
          <el-select v-model="l4RuleForm.target_inbound_id" style="width: 100%" placeholder="请选择目标用户入站">
            <el-option v-for="inb in availableTargetInbounds" :key="inb.id" :label="`${inb.tag} (:${inb.port})`" :value="inb.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注说明">
          <el-input v-model="l4RuleForm.remark" placeholder="如 广州移动 10G BGP 优化" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="l4RuleOpen = false">取消</el-button>
        <el-button type="primary" :loading="l4RuleSaving" @click="saveL4Rule">保存规则</el-button>
      </template>
    </el-dialog>

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

/* 用户接入点独立小盒（订阅入口；右侧源端点拖线指向管道目标） */
  .ap-node {
    position: relative;
    width: 280px;
    background: rgba(15, 23, 42, 0.85);
    border: 1px solid rgba(56, 189, 248, 0.35);
    border-radius: 10px;
    padding: 10px 14px;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 6px;
    box-shadow: 0 4px 20px rgba(56, 189, 248, 0.1);
    transition: all 0.2s ease;

    &:hover {
      border-color: rgba(56, 189, 248, 0.6);
      box-shadow: 0 6px 24px rgba(56, 189, 248, 0.2);
      transform: translateY(-1px);
    }
    &.unlinked {
      border-style: dashed;
      border-color: rgba(251, 191, 36, 0.45);
      box-shadow: 0 4px 16px rgba(251, 191, 36, 0.08);
    }
    &.disabled {
      opacity: 0.6;
      filter: grayscale(0.4);
    }

    .ap-src-handle {
      right: -6px !important;
      top: 50% !important;
      transform: translateY(-50%) !important;
    }
    .ap-node-head {
      display: flex;
      align-items: center;
      gap: 6px;
    }
    .ap-icon {
      font-size: 14px;
      flex: none;
    }
    .ap-name-text {
      font-size: 13px;
      font-weight: 700;
      color: #f1f5f9;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      flex: 1;
      min-width: 0;
    }
    .ap-node-perms {
      display: flex;
      align-items: center;
      gap: 4px;
      flex-wrap: wrap;
    }
    .ap-resolved-text {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 11.5px;
      color: #38bdf8;
      &.waiting {
        color: #fbbf24;
      }
    }
    .ap-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      flex: none;
      &.online {
        background: #10b981;
        box-shadow: 0 0 6px #10b981;
      }
      &.offline {
        background: #fbbf24;
        box-shadow: 0 0 4px #fbbf24;
      }
    }
    .ap-desc-text {
      font-size: 10.5px;
      color: #94a3b8;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
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

  &.is-ap {
    stroke: #0284c7;
    filter: drop-shadow(0 0 8px rgba(56, 189, 248, 0.5));
  }
  &.is-l4 {
    stroke: #9333ea;
    filter: drop-shadow(0 0 8px rgba(192, 132, 252, 0.5));
  }
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

  &.is-ap {
    stroke: #38bdf8;
    filter: drop-shadow(0 0 4px rgba(56, 189, 248, 0.6));
    stroke-dasharray: 6 4;
    animation: dash-flow-slow 20s linear infinite;
  }
  &.is-l4 {
    stroke: #c084fc;
    filter: drop-shadow(0 0 4px rgba(192, 132, 252, 0.6));
  }
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