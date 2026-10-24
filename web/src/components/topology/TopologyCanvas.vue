<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  VueFlow,
  MarkerType,
  useVueFlow,
  type Connection,
  type Edge,
  type EdgeMouseEvent,
  type GraphNode,
  type NodeDragEvent,
  type NodeMouseEvent,
} from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import {
  CsButton,
  CsEmpty,
  CsSpaceBetween,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import {
  deleteServerOutbound,
  deleteServerRoutingRule,
  deleteLayer,
  getTopologyLayout,
  saveTopologyLayout,
  updateInbound,
  updateServerOutbound,
  setAccessPointTarget,
  getPermissionGroups,
  type TopologyData,
} from '@/api/admin'
import type { InboundItem, ServerOutbound, PermissionGroup, UserAccessPoint, AccessLayer } from '@/api/types'
import { errMsg } from '@/api/http'
import OutboundConfigEditor from '@/views/admin/servers/OutboundConfigEditor.vue'
import AccessPointDialog from '@/views/admin/servers/AccessPointDialog.vue'
import ServerBoxNode from './nodes/ServerBoxNode.vue'
import APBoxNode from './nodes/APBoxNode.vue'
import CanvasRulers from './CanvasRulers.vue'
import {
  AP_EST_H,
  AP_W,
  BOTTOM_PAD,
  CAP_HED,
  CARD_GAP_X,
  CARD_GAP_Y,
  COL_HED,
  DEFAULT_BOX_W,
  GAP,
  GRID,
  HEADER_H,
  ROW_BOX_H,
  ROW_H,
  clampBoxWidth,
  snapPoint,
  snapToGrid,
  boxRulePath,
  refEdgePath,
  type BoxData,
  type BoxOutbound,
} from './model'
import { findFreeSlot, rectsOverlap, type Rect } from './placement'
import { useFillViewportHeight } from './useFillViewportHeight'

import LayerDialog from './dialogs/LayerDialog.vue'
import BindInboundDialog from './dialogs/BindInboundDialog.vue'
import RuleDialog from './dialogs/RuleDialog.vue'
import InboundDialog from './dialogs/InboundDialog.vue'

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
const nodes = ref<GraphNode[]>([])
const edges = ref<Edge[]>([])
const flowRef = ref<{
  updateNodeInternals?: (id: string) => void
  fitView?: (options?: Record<string, unknown>) => void
} | null>(null)
const isFullscreen = ref(false)
const wrapEl = ref<HTMLElement | null>(null)

// 画布高度吃掉视口剩余高度：上方 chrome（顶栏/页头/页签/工具条）随宽度变化，不能写死 calc 偏移
useFillViewportHeight(wrapEl)

// 画布视口：网格对齐、标尺读数与「新卡片居中」都依赖它。
// 在渲染 <VueFlow> 的同一组件里调用 useVueFlow 会创建 store 并由 VueFlow 注入复用。
const { viewport, dimensions, findNode, setCenter } = useVueFlow()
const minorGridColor = computed(() =>
  viewport.value.zoom < 0.5 ? 'transparent' : 'rgba(148, 163, 184, 0.055)',
)

// ---- 布局管理：本地缓存 + 云端同步（内容哈希去重）----
const LAYOUT_KEY = 'topology-layout'

interface ServerTagOrder {
  inbounds?: number[]
  outbounds?: string[]
}

interface LayoutData {
  hash: string
  positions: Record<string, { x: number; y: number }>
  widths: Record<string, number>
  tag_orders?: Record<string, ServerTagOrder>
  /** 本地布局版本：2 = 已完成 8pt 归一与新卡落点迁移（仅本地，不上传）。 */
  version?: number
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
      const d: LayoutData = { hash: '', positions: {}, widths: {}, tag_orders: {} }
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
  return { hash: '', positions: {}, widths: {}, tag_orders: {} }
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

function contentHash(
  positions: Record<string, unknown>,
  widths: Record<string, unknown>,
  tagOrders?: Record<string, unknown>,
): string {
  const s = JSON.stringify({
    positions: sortKeys(positions),
    widths: sortKeys(widths),
    tag_orders: sortKeys(tagOrders ?? {}),
  })
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return h.toString(36)
}

const localLayout = loadLayoutLocal()
const boxPositions = new Map<string, { x: number; y: number }>(Object.entries(localLayout.positions))
const boxWidths = new Map<string, number>(Object.entries(localLayout.widths))
const boxTagOrders = reactive(new Map<string, ServerTagOrder>())
if (localLayout.tag_orders) {
  for (const [k, v] of Object.entries(localLayout.tag_orders)) {
    if (v) boxTagOrders.set(k, { inbounds: v.inbounds ? [...v.inbounds] : undefined, outbounds: v.outbounds ? [...v.outbounds] : undefined })
  }
}
const layoutDirty = ref(false)

// 存量布局归一：旧版本允许自由像素拖拽，坐标/宽度可能不是 8 的倍数。
// 打开时一次性吸附（视觉位移 ≤ 4px）并落本地；云端仍由「保存布局」手动提交。
function normalizeLayout(): boolean {
  let changed = false
  for (const [k, p] of boxPositions) {
    const next = snapPoint(p)
    if (next.x !== p.x || next.y !== p.y) {
      boxPositions.set(k, next)
      changed = true
    }
  }
  for (const [k, w] of boxWidths) {
    const next = clampBoxWidth(w)
    if (next !== w) {
      boxWidths.set(k, next)
      changed = true
    }
  }
  if (changed) {
    localLayout.positions = Object.fromEntries(boxPositions)
    localLayout.widths = Object.fromEntries(boxWidths)
    saveLayoutLocal(localLayout)
  }
  return changed
}
normalizeLayout()

// 云端同步：打开时拉取，hash 不一致 → 下载覆盖本地；一致 → 复用本地
onMounted(async () => {
  // 兜底：极端情况下 nodes-initialized 不触发（全部节点离屏），也不能让视口停在初始值
  setTimeout(onNodesInitialized, 1500)
  loadPermissionGroups()
  try {
    const { data } = await getTopologyLayout()
    if (data.code !== 0) return
    const srv = data.data ?? { hash: '', positions: {}, widths: {} }
    if (srv.hash === localLayout.hash) return // 版本一致 → 复用本地布局
    boxPositions.clear()
    boxWidths.clear()
    boxTagOrders.clear()
    for (const [k, v] of Object.entries(srv.positions ?? {})) {
      if (v && typeof v.x === 'number' && typeof v.y === 'number') boxPositions.set(k, { x: v.x, y: v.y })
    }
    for (const [k, v] of Object.entries(srv.widths ?? {})) {
      if (typeof v === 'number') boxWidths.set(k, v)
    }
    for (const [k, v] of Object.entries(srv.tag_orders ?? {})) {
      if (v) boxTagOrders.set(k, { inbounds: v.inbounds ? [...v.inbounds] : undefined, outbounds: v.outbounds ? [...v.outbounds] : undefined })
    }
    normalizeLayout()
    localLayout.hash = srv.hash ?? ''
    localLayout.positions = Object.fromEntries(boxPositions)
    localLayout.widths = Object.fromEntries(boxWidths)
    localLayout.tag_orders = Object.fromEntries(boxTagOrders)
    saveLayoutLocal(localLayout)
    layoutDirty.value = false
    if (props.topology) {
      const focusId = buildGraph(props.topology)
      // 云端布局可能和本地完全不同：重新适配一次视口；有待定位卡片时让聚焦接管
      await nextTick()
      if (!focusId) flowRef.value?.fitView?.({ padding: 0.15 })
    }
  } catch {
    /* 拉取失败用本地布局 */
  }
})

// 「保存布局」按钮：算内容哈希上传云端，成功后更新本地 hash
async function saveLayoutToCloud() {
  const positions = Object.fromEntries(boxPositions)
  const widths = Object.fromEntries(boxWidths)
  const tag_orders = Object.fromEntries(boxTagOrders)
  const hash = contentHash(positions, widths, tag_orders)
  try {
    const { data } = await saveTopologyLayout({ hash, positions, widths, tag_orders })
    if (data.code === 0) {
      localLayout.hash = hash
      localLayout.positions = positions
      localLayout.widths = widths
      localLayout.tag_orders = tag_orders
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

function getBoxWidth(serverId: number | string): number {
  const id = typeof serverId === 'string' && serverId.startsWith('server-') ? serverId : `server-${serverId}`
  return boxWidths.get(id) ?? DEFAULT_BOX_W
}

function measuredHeight(id: string): number {
  const h = findNode(id)?.dimensions?.height ?? 0
  return h > 0 ? snapToGrid(h) : 0
}

// 盒高：优先用 Vue Flow 实测尺寸（层胶囊头会换行、挂载行数也不定），
// 离屏未渲染时退回按行数估算。结果始终落在 8pt 网格上。
function estBoxHeight(serverId: number, outCount: number): number {
  const measured = measuredHeight(`server-${serverId}`)
  if (measured) return measured
  const inbCol = inboundColumnRows(serverId).colH
  const outCol = HEADER_H + COL_HED + Math.max(outCount, 1) * ROW_H + BOTTOM_PAD
  return snapToGrid(Math.max(inbCol, outCol))
}

function apHeightOf(apId: number): number {
  return measuredHeight(`ap-${apId}`) || AP_EST_H
}

function nodePosOf(s: TopologyData['servers'][number], idx: number) {
  return boxPositions.get(`server-${s.id}`) ?? snapPoint({ x: 40 + idx * (DEFAULT_BOX_W + CARD_GAP_X), y: 24 })
}

// 对外接入层：入站列先渲染层胶囊（显式端点点位），后渲染未挂层原生行。
// 行号/盒高估算必须与显示顺序同源（见 inboundColumnRows / estBoxHeight）。
function layerGroupsOf(serverId: number): { layer: AccessLayer; inbounds: TopologyData['inbounds'] }[] {
  const data = props.topology
  if (!data) return []
  const groups: { layer: AccessLayer; inbounds: TopologyData['inbounds'] }[] = []
  const order = boxTagOrders.get(`server-${serverId}`)?.inbounds
  const rank = order && order.length > 0 ? new Map(order.map((id, idx) => [id, idx])) : null
  for (const l of data.layers ?? []) {
    if (l.server_id !== serverId) continue
    const rawInbs = data.inbounds.filter((i) => i.server_id === serverId && i.layer_id === l.id && i.type !== 'channel')
    const inbounds = rank
      ? [...rawInbs].sort((a, b) => (rank.has(a.id) ? rank.get(a.id)! : 9999) - (rank.has(b.id) ? rank.get(b.id)! : 9999))
      : rawInbs
    groups.push({ layer: l, inbounds })
  }
  return groups
}

function nativeInboundsOf(serverId: number): TopologyData['inbounds'] {
  const data = props.topology
  if (!data) return []
  const list = data.inbounds.filter((i) => i.server_id === serverId && !i.layer_id && i.type !== 'channel')
  const order = boxTagOrders.get(`server-${serverId}`)?.inbounds
  if (!order || order.length === 0) return list
  const rank = new Map(order.map((id, idx) => [id, idx]))
  return [...list].sort((a, b) => {
    const ra = rank.has(a.id) ? rank.get(a.id)! : 9999
    const rb = rank.has(b.id) ? rank.get(b.id)! : 9999
    return ra - rb
  })
}

// 入站列显示布局：返回每行中心 Y（相对盒顶，含 HEADER_H）与列总高（顶到底含底部内边距）。
// 列内元素以 8px 间距依次排列：层胶囊 = 内边距 8 + 头 32 + 间距 8 + 行；普通行 = 32。
function inboundColumnRows(serverId: number): { rows: { id: number; centerY: number }[]; colH: number } {
  const data = props.topology
  const rows: { id: number; centerY: number }[] = []
  if (!data) return { rows, colH: HEADER_H + COL_HED + GAP + ROW_BOX_H + BOTTOM_PAD }
  let y = HEADER_H + COL_HED
  for (const g of layerGroupsOf(serverId)) {
    y += GAP // 列内元素间距
    y += CAP_HED // 胶囊上内边距 + 头 + 头下间距
    if (g.inbounds.length === 0) {
      rows.push({ id: -g.layer.id, centerY: y + ROW_BOX_H / 2 })
      y += ROW_BOX_H + GAP // 空层仍占一行「暂无挂载」
      continue
    }
    for (const inb of g.inbounds) {
      rows.push({ id: inb.id, centerY: y + ROW_BOX_H / 2 })
      y += ROW_BOX_H + GAP
    }
  }
  for (const inb of nativeInboundsOf(serverId)) {
    y += GAP
    rows.push({ id: inb.id, centerY: y + ROW_BOX_H / 2 })
    y += ROW_BOX_H
  }
  return { rows, colH: y + BOTTOM_PAD }
}

// 复制层级 Caddyfile 反代片段（domain = 层 Host；每个挂载入站一个 path → 本地 xray 端口）
function copyLayerCaddyfile(layer: AccessLayer) {
  const inbs = layerGroupsOf(layer.server_id).find((g) => g.layer.id === layer.id)?.inbounds ?? []
  const lines = [`# === Caddyfile 示例 (${layer.host}) ===`, `${layer.host} {`]
  for (const inb of inbs) {
    let path = inb.share_path || '/xhttp'
    lines.push(`    # 入站 [${inb.tag}] 转发至本地 Xray 端口`)
    lines.push(`    reverse_proxy ${path} 127.0.0.1:${inb.port}`)
  }
  lines.push(`}`)
  navigator.clipboard.writeText(lines.join('\n'))
  ElMessage.success(`已复制 ${layer.host} 的 Caddyfile 配置片段`)
}

// 按 tag_orders 排序列出服务器出站项
function getOrderedOutbounds(serverId: number, rawList: BoxOutbound[]): BoxOutbound[] {
  const order = boxTagOrders.get(`server-${serverId}`)?.outbounds
  if (!order || order.length === 0) return rawList
  const rank = new Map(order.map((id, idx) => [id, idx]))
  return [...rawList].sort((a, b) => {
    const ra = rank.has(a.id) ? rank.get(a.id)! : 9999
    const rb = rank.has(b.id) ? rank.get(b.id)! : 9999
    return ra - rb
  })
}

// 该服务器全部入站 id（原生 + 挂层），顺序 = 后端返回序（显示排序的回退基准）
function allInboundsOf(serverId: number): number[] {
  return (props.topology?.inbounds ?? []).filter((i) => i.server_id === serverId && i.type !== 'channel').map((i) => i.id)
}

// 更新并持久化卡片内出入站自定义顺序
function setServerTagOrder(serverId: number, patch: { inbounds?: number[]; outbounds?: string[] }) {
  const key = `server-${serverId}`
  const existing = boxTagOrders.get(key) ?? {}
  const nodeData = (nodes.value as any[]).find((n) => n.id === key)?.data as BoxData | undefined
  const next: ServerTagOrder = {
    // 播种必须含全部入站（原生 + 挂层）：只播种单组会让另一组成员后续被当作
    // 「不在序内」走兜底分支，交替微调时互相清零自定义顺序
    inbounds: patch.inbounds ?? (existing.inbounds ? [...existing.inbounds] : allInboundsOf(serverId)),
    outbounds:
      patch.outbounds ??
      (existing.outbounds
        ? [...existing.outbounds]
        : (nodeData?.outbounds ?? []).map((o) => o.id)),
  }
  boxTagOrders.set(key, next)
  localLayout.tag_orders = Object.fromEntries(boxTagOrders)
  saveLayoutLocal(localLayout)
  layoutDirty.value = true

  // 刷新 node.data.outbounds
  const srvNode = (nodes.value as any[]).find((n) => n.id === key)
  if (srvNode && srvNode.data) {
    const data = srvNode.data as BoxData
    if (props.topology) {
      const rawOut = (props.topology.outbounds || []).filter((o) => o.server_id === serverId && o.protocol !== 'blackhole')
      const isDirect = (o: TopologyData['outbounds'][number]) => (o.tag === 'direct' || o.protocol === 'freedom') && !o.inbound_ref && !o.owner_inbound_id
      const list: BoxOutbound[] = rawOut.map((o) => ({
        ...o,
        id: String(o.id),
        virtual: isDirect(o),
        owner_inbound_id: o.owner_inbound_id,
        server_id: o.server_id,
      }))
      for (const inb of props.topology.inbounds) {
        if (inb.server_id === serverId && (inb.type === 'tunnel' || inb.protocol === 'dokodemo-door')) {
          const hasOut = rawOut.some((o) => o.owner_inbound_id === inb.id)
          if (!hasOut) {
            list.push({
              id: `draft-tunnel-${inb.id}`,
              tag: `out-${inb.tag}`,
              protocol: 'freedom',
              virtual: false,
              owner_inbound_id: inb.id,
              isDraftTunnel: true,
              server_id: inb.server_id,
            })
          }
        }
      }
      data.outbounds = getOrderedOutbounds(serverId, list)
    }
  }

  nextTick(() => {
    flowRef.value?.updateNodeInternals?.(key)
  })
}

function moveInboundOrder(serverId: number, inboundId: number, direction: 'up' | 'down') {
  if (!props.topology) return
  const inb = props.topology.inbounds.find((i) => i.id === inboundId)
  if (!inb) return
  const list = inb.layer_id
    ? (layerGroupsOf(serverId).find((g) => g.layer.id === inb.layer_id)?.inbounds ?? []).map((i) => i.id)
    : nativeInboundsOf(serverId).map((i) => i.id)
  const idx = list.indexOf(inboundId)
  if (idx === -1) return
  const targetIdx = direction === 'up' ? idx - 1 : idx + 1
  if (targetIdx < 0 || targetIdx >= list.length) return
  const swapped = list[targetIdx]
  const tmp = list[idx]
  list[idx] = list[targetIdx]
  list[targetIdx] = tmp

  const existingOrder = boxTagOrders.get(`server-${serverId}`)?.inbounds ?? allInboundsOf(serverId)
  const nextOrder = [...existingOrder]
  const posA = nextOrder.indexOf(inboundId)
  const posB = nextOrder.indexOf(swapped)
  if (posA !== -1 && posB !== -1) {
    const t = nextOrder[posA]
    nextOrder[posA] = nextOrder[posB]
    nextOrder[posB] = t
    setServerTagOrder(serverId, { inbounds: nextOrder })
  } else {
    // 兜底：existingOrder 缺本组成员（旧数据只播种过部分入站）时按占位合并而非整组替换，
    // 他组成员保留原相对序，避免抹掉另一组的自定义顺序
    const groupSet = new Set(list)
    const queue = [...list]
    const merged = existingOrder.map((id) => (groupSet.has(id) ? (queue.shift() ?? id) : id))
    while (queue.length > 0) merged.push(queue.shift()!)
    setServerTagOrder(serverId, { inbounds: merged })
  }
}

function moveOutboundOrder(serverId: number, outboundId: string, direction: 'up' | 'down') {
  const srvNode = (nodes.value as any[]).find((n) => n.id === `server-${serverId}`)
  const currentOutbounds: BoxOutbound[] = (srvNode?.data as BoxData)?.outbounds ?? []
  const currentList = currentOutbounds.map((o) => o.id)
  const idx = currentList.indexOf(outboundId)
  if (idx === -1) return
  const targetIdx = direction === 'up' ? idx - 1 : idx + 1
  if (targetIdx < 0 || targetIdx >= currentList.length) return
  const tmp = currentList[idx]
  currentList[idx] = currentList[targetIdx]
  currentList[targetIdx] = tmp
  setServerTagOrder(serverId, { outbounds: currentList })
}

// 根据当前入站顺序与路由规则，自动重排出站顺序，消除盒内走线交叉
function autoAlignServerTags(serverId: number) {
  if (!props.topology) return
  const inbs = nativeInboundsOf(serverId)
  const rules = (props.topology.routing_rules || []).filter((r) => r.server_id === serverId && r.enabled)
  const srvNode = (nodes.value as any[]).find((n) => n.id === `server-${serverId}`)
  const outbounds: BoxOutbound[] = (srvNode?.data as BoxData)?.outbounds ?? []
  if (inbs.length === 0 || outbounds.length === 0) return

  const matchedOutboundIds: string[] = []
  const usedOutboundIds = new Set<string>()

  for (const inb of inbs) {
    const rule = rules.find((r) => r.inbound_tag === inb.tag)
    if (rule) {
      const targetOut = outbounds.find((o) => o.tag === rule.outbound_tag && !usedOutboundIds.has(o.id))
      if (targetOut) {
        matchedOutboundIds.push(targetOut.id)
        usedOutboundIds.add(targetOut.id)
      }
    } else if (inb.type === 'tunnel' || inb.protocol === 'dokodemo-door') {
      const tunnelOut = outbounds.find((o) => o.owner_inbound_id === inb.id && !usedOutboundIds.has(o.id))
      if (tunnelOut) {
        matchedOutboundIds.push(tunnelOut.id)
        usedOutboundIds.add(tunnelOut.id)
      }
    }
  }
  for (const out of outbounds) {
    if (!usedOutboundIds.has(out.id)) {
      matchedOutboundIds.push(out.id)
    }
  }

  setServerTagOrder(serverId, { outbounds: matchedOutboundIds })
  ElMessage.success('已自动整理出入站对应走线')
}

// ---- 布局落点：新卡片放哪、旧卡片怎么迁移 ----

/** 剪掉数据库里已不存在的节点，避免孤儿坐标被反复上传。 */
function pruneLayout(data: TopologyData): boolean {
  // 空拓扑（接口异常/加载中）不能当作「全部删除」，否则会清掉布局表
  if (data.servers.length === 0 && (data.access_points?.length ?? 0) === 0) return false
  const ids = new Set<string>()
  for (const s of data.servers) ids.add(`server-${s.id}`)
  for (const ap of data.access_points ?? []) ids.add(`ap-${ap.id}`)
  let changed = false
  for (const k of [...boxPositions.keys()]) {
    if (!ids.has(k)) {
      boxPositions.delete(k)
      changed = true
    }
  }
  for (const k of [...boxWidths.keys()]) {
    if (!ids.has(k)) {
      boxWidths.delete(k)
      changed = true
    }
  }
  for (const k of [...boxTagOrders.keys()]) {
    if (!ids.has(k)) {
      boxTagOrders.delete(k)
      changed = true
    }
  }
  return changed
}

function viewportCenter(): { x: number; y: number } | null {
  const d = dimensions.value
  const z = viewport.value.zoom || 1
  if (!d?.width || !d?.height) return null
  return {
    x: (d.width / 2 - viewport.value.x) / z,
    y: (d.height / 2 - viewport.value.y) / z,
  }
}

function occupiedRects(data: TopologyData): Rect[] {
  const rects: Rect[] = []
  for (const s of data.servers) {
    const p = boxPositions.get(`server-${s.id}`)
    if (!p) continue
    const outCount = data.outbounds.filter((o) => o.server_id === s.id && o.protocol !== 'blackhole').length
    rects.push({ x: p.x, y: p.y, w: getBoxWidth(s.id), h: estBoxHeight(s.id, outCount) })
  }
  for (const ap of data.access_points ?? []) {
    const p = boxPositions.get(`ap-${ap.id}`)
    if (!p) continue
    rects.push({ x: p.x, y: p.y, w: AP_W, h: apHeightOf(ap.id) })
  }
  return rects
}

/** 视口中心优先；视口尚未测量时退到现有卡片右侧。 */
function anchorForNew(data: TopologyData): { x: number; y: number } {
  const vc = viewportCenter()
  if (vc) return vc
  let x = 40
  let y = 24
  for (const r of occupiedRects(data)) {
    x = Math.max(x, r.x + r.w + CARD_GAP_X)
    y = Math.min(y, r.y)
  }
  return { x, y }
}

function pickPosition(
  preferred: { x: number; y: number } | null,
  size: { w: number; h: number },
  anchor: { x: number; y: number },
  occupied: Rect[],
): { x: number; y: number } {
  if (preferred) {
    const rect: Rect = { ...preferred, ...size }
    if (!occupied.some((o) => rectsOverlap(rect, o, 24))) return snapPoint(preferred)
  }
  return findFreeSlot({ anchor, size, occupied })
}

/** 首次进入（没有任何存量布局）时的确定性排列，与「重置排列」保持一致。 */
function applyDefaultLayout(data: TopologyData) {
  let apY = 24
  for (const ap of data.access_points ?? []) {
    boxPositions.set(`ap-${ap.id}`, snapPoint({ x: 40, y: apY }))
    apY += apHeightOf(ap.id) + CARD_GAP_Y
  }
  const offsetX = 40 + ((data.access_points?.length ?? 0) > 0 ? AP_W + CARD_GAP_X : 0)
  data.servers.forEach((s, idx) => {
    boxPositions.set(`server-${s.id}`, snapPoint({ x: offsetX + idx * (DEFAULT_BOX_W + CARD_GAP_X), y: 24 }))
  })
}

/**
 * 给还没有坐标的节点定位置。
 * 升级后的第一次打开（version < 2）沿用旧版兜底坐标，避免把从未拖动过的卡片
 * 一次性重排；之后新建的卡片才落到视口中心附近的空位。
 */
function ensurePositions(data: TopologyData): string[] {
  const total = data.servers.length + (data.access_points?.length ?? 0)
  if (total === 0) return []
  if (boxPositions.size === 0) {
    applyDefaultLayout(data)
    localLayout.positions = Object.fromEntries(boxPositions)
    localLayout.version = 2
    saveLayoutLocal(localLayout)
    return []
  }

  const migrating = (localLayout.version ?? 1) < 2
  const anchor = anchorForNew(data)
  const placed: string[] = []

  data.servers.forEach((s, idx) => {
    const id = `server-${s.id}`
    if (boxPositions.has(id)) return
    const outCount = data.outbounds.filter((o) => o.server_id === s.id && o.protocol !== 'blackhole').length
    const size = { w: getBoxWidth(s.id), h: estBoxHeight(s.id, outCount) }
    const legacy = migrating ? snapPoint({ x: 40 + idx * (DEFAULT_BOX_W + CARD_GAP_X), y: 24 }) : null
    boxPositions.set(id, pickPosition(legacy, size, anchor, occupiedRects(data)))
    placed.push(id)
  })

  ;(data.access_points ?? []).forEach((ap, idx) => {
    const id = `ap-${ap.id}`
    if (boxPositions.has(id)) return
    const size = { w: AP_W, h: apHeightOf(ap.id) }
    const legacy = migrating ? snapPoint({ x: 40, y: 24 + idx * (AP_EST_H + CARD_GAP_Y) }) : null
    // 已指定目标的接入点优先贴到目标服务器左侧，符合「入口 → 管道」的阅读方向
    let apAnchor = anchor
    if (ap.target_type === 'inbound' && ap.target_inbound_id) {
      const inb = data.inbounds.find((i) => i.id === ap.target_inbound_id)
      const sp = inb ? boxPositions.get(`server-${inb.server_id}`) : null
      if (sp) apAnchor = { x: sp.x - AP_W - CARD_GAP_X, y: sp.y }
    }
    boxPositions.set(id, pickPosition(legacy, size, apAnchor, occupiedRects(data)))
    placed.push(id)
  })

  if (migrating) localLayout.version = 2
  if (placed.length || migrating) {
    localLayout.positions = Object.fromEntries(boxPositions)
    saveLayoutLocal(localLayout)
    if (placed.length) layoutDirty.value = true
  }
  return placed
}

// ---- 新卡片聚焦：居中 + 短暂高亮 ----
const FOCUS_KEY = 'topology-focus-id'
let flashTimer: number | undefined
let pendingFocusId: string | null = null
// 首次视口适配由 nodes-initialized 决定：有待定位卡片就居中它，否则 fitView。
// 不能同时开 fit-view-on-init，否则它会在 setCenter 之后把视口又拉回去。
let initialViewDone = false

function requestFocus(nodeId: string) {
  pendingFocusId = nodeId
  if (initialViewDone) applyPendingFocus()
}

function applyPendingFocus() {
  const id = pendingFocusId
  if (!id) return
  pendingFocusId = null
  focusNode(id)
}

function onNodesInitialized() {
  if (initialViewDone) return
  initialViewDone = true
  nextTick(() => {
    setTimeout(() => {
      if (pendingFocusId) applyPendingFocus()
      else flowRef.value?.fitView?.({ padding: 0.15 })
    }, 60)
  })
}

function focusNode(nodeId: string) {
  nextTick(() => {
    setTimeout(() => {
      const node = (nodes.value as any[]).find((n) => n.id === nodeId)
      if (!node) return
      const w = node.dimensions?.width || (nodeId.startsWith('ap-') ? AP_W : DEFAULT_BOX_W)
      const h = node.dimensions?.height || (nodeId.startsWith('ap-') ? AP_EST_H : 200)
      setCenter(node.position.x + w / 2, node.position.y + h / 2, {
        zoom: viewport.value.zoom || 1,
        duration: 300,
      })
      node.class = 'just-created'
      clearTimeout(flashTimer)
      flashTimer = window.setTimeout(() => {
        const n = (nodes.value as any[]).find((x) => x.id === nodeId)
        if (n) n.class = undefined
      }, 2200)
    }, 80)
  })
}

/** 优先消费服务器页留下的待定位标记；否则只在新卡片恰好一张时聚焦。 */
function takeFocusId(placed: string[]): string | null {
  let saved: string | null = null
  try {
    saved = sessionStorage.getItem(FOCUS_KEY)
  } catch {
    /* 隐私模式等场景忽略 */
  }
  if (saved) {
    if (!(nodes.value as any[]).some((n) => n.id === saved)) return null
    try {
      sessionStorage.removeItem(FOCUS_KEY)
    } catch {
      /* ignore */
    }
    return saved
  }
  return placed.length === 1 ? placed[0] : null
}

function buildGraph(data: TopologyData): string | null {
  if (pruneLayout(data)) {
    localLayout.positions = Object.fromEntries(boxPositions)
    localLayout.widths = Object.fromEntries(boxWidths)
    localLayout.tag_orders = Object.fromEntries(boxTagOrders)
    saveLayoutLocal(localLayout)
  }
  const placedIds = ensurePositions(data)

  const inbByServer = new Map<number, TopologyData['inbounds']>()
  for (const inb of data.inbounds) {
    if (!inbByServer.has(inb.server_id)) inbByServer.set(inb.server_id, [])
    inbByServer.get(inb.server_id)!.push(inb)
  }

  const outByServer = new Map<number, BoxOutbound[]>()
  for (const out of data.outbounds) {
    if (out.protocol === 'blackhole') continue // blocked 保留口不入画布（后端保留，仅供规则引用）
    if (!outByServer.has(out.server_id)) outByServer.set(out.server_id, [])
    const isDirect = (out.tag === 'direct' || out.protocol === 'freedom') && !out.inbound_ref && !out.owner_inbound_id
    outByServer.get(out.server_id)!.push({
      ...out,
      id: String(out.id),
      virtual: isDirect,
      owner_inbound_id: out.owner_inbound_id,
      server_id: out.server_id,
    })
  }

  // 为没有派生出站的四层直通入站注入草稿出站行，供用户从右侧把手拖线直连落地入站
  for (const inb of data.inbounds) {
    if (inb.type === 'tunnel' || inb.protocol === 'dokodemo-door') {
      const hasOut = data.outbounds.some((o) => o.owner_inbound_id === inb.id)
      if (!hasOut) {
        if (!outByServer.has(inb.server_id)) outByServer.set(inb.server_id, [])
        outByServer.get(inb.server_id)!.push({
          id: `draft-tunnel-${inb.id}`,
          tag: `out-${inb.tag}`,
          protocol: 'freedom',
          virtual: false,
          owner_inbound_id: inb.id,
          isDraftTunnel: true,
          server_id: inb.server_id,
        })
      }
    }
  }

  const serverNodes = data.servers.map((s, idx) => ({
    id: `server-${s.id}`,
    type: 'serverbox',
    position: nodePosOf(s, idx),
    data: {
      server: s,
      inbounds: inbByServer.get(s.id) || [],
      outbounds: getOrderedOutbounds(s.id, outByServer.get(s.id) || []),
      layers: (data.layers ?? []).filter((l) => l.server_id === s.id),
      boxWidth: getBoxWidth(s.id),
    } as BoxData,
  }))

  // 用户接入点：每个 AP 一个独立小盒（订阅入口），连线方向 AP → 目标入站
  const apNodes = (data.access_points || []).map((ap, idx) => ({
    id: `ap-${ap.id}`,
    type: 'apbox',
    position: boxPositions.get(`ap-${ap.id}`) ?? snapPoint({ x: 40, y: 24 + idx * (AP_EST_H + CARD_GAP_Y) }),
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

  // 用户接入点连接线（方向：AP → 目标入站，符合「订阅入口指向管道」认知）
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
          markerEnd: { type: MarkerType.ArrowClosed },
          data: { isAP: true },
        })
      }
    }
  }

  // InboundRef 跨盒实线（含四层直通派生链路 out-<tag>）
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
      markerEnd: { type: MarkerType.ArrowClosed },
      data: { isTunnel: !!out.owner_inbound_id },
    })
  }

  // 路由规则盒内虚线
  for (const rule of data.routing_rules) {
    if (!rule.inbound_tag || !rule.enabled) continue
    const inb = data.inbounds.find((i) => i.tag === rule.inbound_tag && i.server_id === rule.server_id)
    if (!inb) continue
    const out = data.outbounds.find((o) => o.tag === rule.outbound_tag && o.server_id === rule.server_id)
    if (!out || out.protocol === 'blackhole') continue // blocked 行已隐藏，指向它的规则线不绘制
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

  const focusId = takeFocusId(placedIds)
  if (focusId) requestFocus(focusId)
  return focusId
}

// 行内 Handle 锚在行盒（.sb-row）的左右边缘，而行盒宽度由内容撑开：行内元素增删都会
// 让行宽变化、端点 X 跟着平移。节点外框尺寸不变时 VueFlow 的 ResizeObserver 不触发，
// handleBounds 会停留在旧值，盒内走线就与端点脱节 —— 内容变化后必须手动重算。
function refreshNodeInternals() {
  const t = props.topology
  if (!t) return
  for (const s of t.servers) flowRef.value?.updateNodeInternals?.(`server-${s.id}`)
  for (const ap of t.access_points ?? []) flowRef.value?.updateNodeInternals?.(`ap-${ap.id}`)
}

watch(
  () => props.topology,
  async (t) => {
    if (t) buildGraph(t)
    await nextTick()
    refreshNodeInternals()
  },
  { immediate: true },
)

// 编辑模式开关会显隐行内排序按钮（.tag-order-actions，约 26px 宽），
// 受影响的正是盒内规则线的两个端点：左列行的右缘（inb-src-*）与右列行的左缘
// （out-tgt-*）。盒子外框尺寸不变 → 不重算就会残留编辑模式下的旧锚点。
watch(
  () => props.editable,
  async () => {
    await nextTick()
    refreshNodeInternals()
  },
)

// ---- 交互：拖线实时合法性预检 ----
function isValidConnection(conn: Connection): boolean {
  if (!props.editable || !props.topology) return false
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const apSrc = src.match(/^ap-src-(\d+)$/)
  const outSrc = src.match(/^out-src-(.+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const inbTgt = tgt.match(/^inb-tgt-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(.+)$/)

  // 1. 用户接入点 -> 入站管道
  if (apSrc && inbTgt) {
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbTgt[1]))
    return !!inb
  }

  // 2. 出站右侧 -> 他服务器入站左侧（设置 InboundRef 或建立四层直通管道）
  if (outSrc && inbTgt) {
    const outKey = outSrc[1]
    const tgtInb = props.topology.inbounds.find((i) => i.id === Number(inbTgt[1]))
    if (!tgtInb) return false

    // 草稿直通出站: out-src-draft-tunnel-{inboundId}
    const draftMatch = outKey.match(/^draft-tunnel-(\d+)$/)
    if (draftMatch) {
      const srcInb = props.topology.inbounds.find((i) => i.id === Number(draftMatch[1]))
      return !!(srcInb && srcInb.server_id !== tgtInb.server_id)
    }

    // 数据库出站: out-src-{outboundId}
    const out = props.topology.outbounds.find((o) => o.id === Number(outKey))
    return !!(out && out.server_id !== tgtInb.server_id)
  }

  // 3. 入站右侧 -> 同服务器出站左侧（盒内路由规则）
  if (inbSrc && outAny) {
    const inb = props.topology.inbounds.find((i) => i.id === Number(inbSrc[1]))
    if (!inb) return false
    const outKey = outAny[1]
    const draftMatch = outKey.match(/^draft-tunnel-(\d+)$/)
    if (draftMatch) {
      return Number(draftMatch[1]) === inb.id
    }
    const out = props.topology.outbounds.find((o) => o.id === Number(outKey))
    return !!(out && inb.server_id === out.server_id)
  }

  return false
}

// ---- 交互：拖线建连接 ----
async function handleConnect(conn: Connection) {
  if (!props.editable || !props.topology) return
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const apSrc = src.match(/^ap-src-(\d+)$/)
  const outSrc = src.match(/^out-src-(.+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const inbTgt = tgt.match(/^inb-tgt-(\d+)$/)
  const outAny = tgt.match(/^out-tgt-(.+)$/)

  // 1. 用户接入点 → 目标（用户入站或四层直通管道）
  if (apSrc) {
    const apId = Number(apSrc[1])
    if (inbTgt) {
      const inbId = Number(inbTgt[1])
      try {
        const { data } = await setAccessPointTarget(apId, { target_type: 'inbound', target_inbound_id: inbId })
        if (data.code === 0) {
          ElMessage.success('已连接接入点至入站/管道')
          emit('changed')
        } else ElMessage.error(data.message)
      } catch (e) {
        ElMessage.error(errMsg(e, '连接失败'))
      }
      return
    }
    return
  }

  // 2. 出站右侧 -> 他服务器入站
  if (outSrc && inbTgt) {
    const outKey = outSrc[1]
    const tgtInbId = Number(inbTgt[1])

    // 草稿直通出站连接
    const draftMatch = outKey.match(/^draft-tunnel-(\d+)$/)
    if (draftMatch) {
      const srcInbId = Number(draftMatch[1])
      await connectTunnel(srcInbId, tgtInbId)
      return
    }

    // 数据库出站连接
    const outId = Number(outKey)
    const out = props.topology.outbounds.find((o) => o.id === outId)
    if (out && out.owner_inbound_id) {
      await connectTunnel(out.owner_inbound_id, tgtInbId)
    } else if (out) {
      await createRef(outId, tgtInbId)
    }
    return
  }

  // 3. 入站右侧 -> 同服务器出站（盒内规则）
  if (inbSrc && outAny) {
    const outKey = outAny[1]
    if (outKey.startsWith('draft-tunnel-')) {
      ElMessage.info('四层直通管道在连线落地入站后将自动生成专属转发规则')
      return
    }
    openRuleDialog(Number(inbSrc[1]), Number(outKey))
    return
  }

  if (inbSrc && inbTgt) {
    ElMessage.warning('入站端点仅限连接服务器内部出站；跨服务器请从右侧「出站」端点拖出连接至落地入站')
  } else {
    ElMessage.warning('仅支持：接入点 -> 入站/管道、出站 -> 他服务器落地入站、入站 -> 盒内出站')
  }
}

async function connectTunnel(tunnelInboundId: number, targetInboundId: number) {
  const data = props.topology!
  const srcInb = data.inbounds.find((i) => i.id === tunnelInboundId)
  const tgtInb = data.inbounds.find((i) => i.id === targetInboundId)
  if (!srcInb || !tgtInb) return
  if (srcInb.server_id === tgtInb.server_id) {
    ElMessage.warning('四层直通管道仅支持跨服务器直通落地入站')
    return
  }
  const srcSrv = data.servers.find((s) => s.id === srcInb.server_id)?.name ?? `#${srcInb.server_id}`
  const tgtSrv = data.servers.find((s) => s.id === tgtInb.server_id)?.name ?? `#${tgtInb.server_id}`

  let targetAcceptsPP = false
  try {
    const ss = JSON.parse(tgtInb.stream_settings || '{}')
    targetAcceptsPP = !!(ss.acceptProxyProtocol || ss.tcpSettings?.acceptProxyProtocol || ss.xhttpSettings?.acceptProxyProtocol)
  } catch {}

  let confirmMsg = `将四层直通管道「${srcInb.tag}」（${srcSrv}）连接至落地入站「${tgtInb.tag}」（${tgtSrv}）？\n\n开启 PROXY Protocol v2 可将客户端真实 IP 传递给落地节点；若落地入站未开启接收或不需要真实 IP，请选择「直接连接（不开启 PPv2）」。`
  if (targetAcceptsPP) {
    confirmMsg += `\n\n【提示】落地入站「${tgtInb.tag}」当前已开启「接收 Proxy Protocol」。`
  }

  let enablePP = true
  try {
    await ElMessageBox.confirm(confirmMsg, '建立四层直通管道', {
      type: 'info',
      distinguishCancelAndClose: true,
      confirmButtonText: '开启 PPv2 并连接',
      cancelButtonText: '直接连接（不开启 PPv2）',
      showClose: true,
    })
    enablePP = true
  } catch (action) {
    if (action === 'cancel') {
      enablePP = false
    } else {
      // 点击右上角叉号或遮罩关闭，取消本次连接
      return
    }
  }

  try {
    let srcSS: Record<string, any> = {}
    try {
      srcSS = JSON.parse(srcInb.stream_settings || '{}')
    } catch {}
    // 转发网络属于 dokodemo 的 settings.network；streamSettings.network 不是合法传输名。
    let srcSettings: Record<string, any> = {}
    try {
      srcSettings = JSON.parse(srcInb.settings_json || '{}')
    } catch {}
    srcSS.proxy_protocol = enablePP
    if (!srcSettings.network) {
      srcSettings.network = 'tcp,udp'
    }
    // 目标住在派生 freedom 出站的 redirect；settings.address/port 被字段声明禁止。
    delete srcSettings.address
    delete srcSettings.port

    if (enablePP && !targetAcceptsPP) {
      let ss: Record<string, any> = {}
      try {
        ss = JSON.parse(tgtInb.stream_settings || '{}')
      } catch {}
      if (!ss.tcpSettings || typeof ss.tcpSettings !== 'object') {
        ss.tcpSettings = {}
      }
      ss.tcpSettings.acceptProxyProtocol = true
      await updateInbound(tgtInb.id, { stream_settings: JSON.stringify(ss) })
    }

    const { data: res } = await updateInbound(tunnelInboundId, {
      target_inbound_id: targetInboundId,
      target_address: '',
      target_port: 0,
      settings_json: JSON.stringify(srcSettings),
      stream_settings: JSON.stringify(srcSS),
    })
    if (res.code === 0) {
      ElMessage.success(enablePP ? '四层直通管道已连接（已启用 PPv2）' : '四层直通管道已连接（未启用 PPv2）')
      emit('changed')
    } else {
      ElMessage.error(res.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '连接直通管道失败'))
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
const apEditingItem = ref<UserAccessPoint | null>(null)
const permissionGroups = ref<PermissionGroup[]>([])

async function loadPermissionGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) permissionGroups.value = data.data.items
  } catch {}
}

function openCreateAccessPoint() {
  apEditingItem.value = null
  loadPermissionGroups()
  apDialogOpen.value = true
}

function openEditAccessPoint(ap: UserAccessPoint) {
  apEditingItem.value = ap
  loadPermissionGroups()
  apDialogOpen.value = true
}

// 目标落地服务器候选（全部 Xray 托管节点；l4_relay 已于 2026-08-24 退役）
const availableXrayServers = computed(() => props.topology?.servers || [])

// ---- 对外接入层弹窗（组件与表单在 dialogs/LayerDialog.vue）----
const layerDialogOpen = ref(false)
const layerEditing = ref<AccessLayer | null>(null)
const layerServerId = ref(0)

function openCreateLayer(serverId: number) {
  layerServerId.value = serverId
  layerEditing.value = null
  layerDialogOpen.value = true
}

function openEditLayer(layer: AccessLayer) {
  layerServerId.value = layer.server_id
  layerEditing.value = layer
  layerDialogOpen.value = true
}

// 画布胶囊上的「删除」快捷入口与弹窗底部按钮共用这一个删除流程
async function handleDeleteLayer(layer: AccessLayer) {
  try {
    await ElMessageBox.confirm(
      `删除对外层「${layer.name}」？挂载的入站将回退为直连端点（原生行），订阅地址改为入站自身分享地址。`,
      '删除对外层',
      { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const { data } = await deleteLayer(layer.server_id, layer.id)
    if (data.code === 0) {
      ElMessage.success('已删除对外层')
      layerDialogOpen.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- 入站挂载对外层（组件在 dialogs/BindInboundDialog.vue）----
const bindDialogOpen = ref(false)
const bindServerId = ref(0)
const bindLayerId = ref(0)

const availableBindInbounds = computed(() => {
  if (!props.topology) return []
  return props.topology.inbounds.filter((i) => i.server_id === bindServerId.value && !i.layer_id)
})

function openBindInbound(layer: AccessLayer) {
  bindServerId.value = layer.server_id
  bindLayerId.value = layer.id
  if (availableBindInbounds.value.length === 0) {
    ElMessage.info('该服务器暂无可挂载的入站（已全部挂层）')
    return
  }
  bindDialogOpen.value = true
}

async function unbindInbound(inb: InboundItem) {
  try {
    await ElMessageBox.confirm(`将入站「${inb.tag}」移出对外层，恢复为直连端点？`, '移出对外层', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await updateInbound(inb.id, { layer_id: 0 })
    if (data.code === 0) {
      ElMessage.success('已移出对外层')
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
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

function openEditOutbound(out: TopologyData['outbounds'][number] | BoxOutbound) {
  if (out.virtual) return
  if (out.owner_inbound_id) {
    const inb = props.topology?.inbounds.find((i) => i.id === out.owner_inbound_id)
    if (inb) {
      openEditInbound(inb)
      return
    }
  }
  if (!out.server_id) return
  outboundServerId.value = out.server_id
  outboundEditing.value = out as any
  outboundEditorOpen.value = true
}

function handleOutboundSaved() {
  outboundEditorOpen.value = false
  emit('changed')
}

// ---- 入站新建/编辑弹窗（组件在 dialogs/InboundDialog.vue）----
const inboundCreateOpen = ref(false)
const inboundCreateServerId = ref(0)
const inboundEditingItem = ref<InboundItem | null>(null)

function openCreateInbound(serverId: number) {
  inboundCreateServerId.value = serverId
  inboundEditingItem.value = null
  inboundCreateOpen.value = true
}

function openEditInbound(inb: InboundItem) {
  inboundCreateServerId.value = inb.server_id
  inboundEditingItem.value = inb
  inboundCreateOpen.value = true
}

// ---- 交互：路由规则弹窗（组件在 dialogs/RuleDialog.vue）----
const ruleOpen = ref(false)
const ruleServerId = ref(0)
const ruleInboundTag = ref('')
const ruleOutboundTag = ref('')

function openRuleDialog(inboundId: number, outboundId: number) {
  const data = props.topology!
  const inb = data.inbounds.find((i) => i.id === inboundId)
  const out = data.outbounds.find((o) => o.id === outboundId)
  if (!inb || !out) return
  if (inb.server_id !== out.server_id) {
    ElMessage.warning('路由规则只能在同一服务器内（入站 → 出站）')
    return
  }
  ruleServerId.value = inb.server_id
  ruleInboundTag.value = inb.tag
  ruleOutboundTag.value = out.tag
  ruleOpen.value = true
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
    if (out.owner_inbound_id) {
      const inb = props.topology.inbounds.find((i) => i.id === out.owner_inbound_id)
      const inbTag = inb ? inb.tag : out.tag
      try {
        await ElMessageBox.confirm(
          `解除四层直通管道「${inbTag}」与目标入站的连线？解除后管道将恢复为草稿状态。`,
          '解除直通管道',
          { type: 'warning' },
        )
      } catch {
        deselectEdge(edge.id)
        return
      }
      try {
        const { data } = await updateInbound(out.owner_inbound_id, { target_inbound_id: 0 })
        if (data.code === 0) {
          ElMessage.success('已解除直通连线')
          emit('changed')
        } else {
          ElMessage.error(data.message)
          deselectEdge(edge.id)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '解除连线失败'))
        deselectEdge(edge.id)
      }
      return
    }
    try {
      await ElMessageBox.confirm(
        `解除出站「${out.tag}」的 InboundRef 引用？`,
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
        `解除接入点「${ap.name}」的目标连线？`,
        '解除接入点连接',
        { type: 'warning' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await setAccessPointTarget(apId, { target_type: '', target_inbound_id: null })
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
  } else if (edge.id.startsWith('tunnel-')) {
    const tunnelInboundId = Number(edge.id.slice(7))
    const inb = props.topology.inbounds.find((i) => i.id === tunnelInboundId)
    if (!inb) return
    try {
      await ElMessageBox.confirm(
        `解除四层直通管道「${inb.tag}」与目标入站的连线？解除后管道将恢复为草稿状态。`,
        '解除直通管道',
        { type: 'warning' },
      )
    } catch {
      deselectEdge(edge.id)
      return
    }
    try {
      const { data } = await updateInbound(tunnelInboundId, { target_inbound_id: 0 })
      if (data.code === 0) {
        ElMessage.success('已解除直通连线')
        emit('changed')
      } else {
        ElMessage.error(data.message)
        deselectEdge(edge.id)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '解除连线失败'))
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
    const w = clampBoxWidth(startWidth + deltaX)
    data.boxWidth = w
    boxWidths.set(nodeId, w)
    localLayout.widths = Object.fromEntries(boxWidths)
    saveLayoutLocal(localLayout)
  }
  const onMouseUp = () => {
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
    layoutDirty.value = true
  }
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

// 双击盒子 → 打开服务器抽屉
function handleNodeDblClick(evt: NodeMouseEvent) {
  const box = evt.node.data as BoxData
  if (box?.server?.id) emit('open-server', box.server.id)
}

// 盒子拖动结束：更新本地布局
function onNodeDragStop(evt: NodeDragEvent) {
  const node = evt.node
  if (node.position) {
    const pos = snapPoint(node.position)
    node.position = pos
    boxPositions.set(node.id, pos)
    localLayout.positions = Object.fromEntries(boxPositions)
    saveLayoutLocal(localLayout)
    layoutDirty.value = true
  }
}

// ---- 交互：一键分层自动排版（DAG 拓扑分层）----
function autoLayout() {
  if (!props.topology || props.topology.servers.length === 0) return
  const data = props.topology
  const srvIds = data.servers.map((s) => s.id)
  const upstream = new Map<number, Set<number>>()
  for (const id of srvIds) upstream.set(id, new Set())

  // InboundRef: out (src) -> inb (tgt)；流量方向 src -> tgt，故 tgt 依赖 src
  for (const out of data.outbounds) {
    if (!out.inbound_ref) continue
    const targetInb = data.inbounds.find((i) => i.id === out.inbound_ref)
    if (!targetInb) continue
    if (out.server_id !== targetInb.server_id && upstream.has(targetInb.server_id)) {
      upstream.get(targetInb.server_id)!.add(out.server_id)
    }
  }

  // 四层直通管道: tunnel (src) -> inb (tgt)；流量方向 src -> tgt，故 tgt 依赖 src
  for (const inb of data.inbounds) {
    if (inb.type !== 'tunnel' || !inb.target_inbound_id) continue
    const targetInb = data.inbounds.find((i) => i.id === inb.target_inbound_id)
    if (!targetInb) continue
    if (inb.server_id !== targetInb.server_id && upstream.has(targetInb.server_id)) {
      upstream.get(targetInb.server_id)!.add(inb.server_id)
    }
  }

  // 最长路径层级计算（拓扑分层）
  const depth = new Map<number, number>()
  function getDepth(id: number, visited = new Set<number>()): number {
    if (visited.has(id)) return 0
    if (depth.has(id)) return depth.get(id)!
    visited.add(id)
    const ups = upstream.get(id) || new Set()
    let d = 0
    for (const upId of ups) {
      d = Math.max(d, getDepth(upId, new Set(visited)) + 1)
    }
    depth.set(id, d)
    return d
  }
  for (const id of srvIds) getDepth(id)

  const layers = new Map<number, number[]>()
  for (const [id, d] of depth) {
    if (!layers.has(d)) layers.set(d, [])
    layers.get(d)!.push(id)
  }

  // AP 节点固定排在最左侧（第 0 列），纵向按实测高度推进并吸附到网格
  const aps = data.access_points || []
  let apY = 24
  for (const ap of aps) {
    boxPositions.set(`ap-${ap.id}`, snapPoint({ x: 40, y: apY }))
    apY += apHeightOf(ap.id) + CARD_GAP_Y
  }

  // 各层 X 起点沿用该层最宽盒子尺寸累计推进，杜绝硬编码 520px 导致拉宽盒子时互相压叠穿模
  let curX = 40 + (aps.length > 0 ? AP_W + CARD_GAP_X : 0)

  const outCounts = new Map<number, number>()
  for (const out of data.outbounds) {
    if (out.protocol !== 'blackhole') outCounts.set(out.server_id, (outCounts.get(out.server_id) || 0) + 1)
  }

  const sortedDepths = Array.from(layers.keys()).sort((a, b) => a - b)
  for (const d of sortedDepths) {
    const srvsInLayer = layers.get(d)!
    let curY = 24
    let maxColWidth = DEFAULT_BOX_W
    for (const sid of srvsInLayer) {
      const w = getBoxWidth(sid)
      maxColWidth = Math.max(maxColWidth, w)
      boxPositions.set(`server-${sid}`, snapPoint({ x: curX, y: curY }))
      const outCount = outCounts.get(sid) || 0
      curY += estBoxHeight(sid, outCount) + CARD_GAP_Y
    }
    curX = snapToGrid(curX + maxColWidth + CARD_GAP_X)
  }

  localLayout.positions = Object.fromEntries(boxPositions)
  saveLayoutLocal(localLayout)
  layoutDirty.value = true

  for (const node of nodes.value) {
    const p = boxPositions.get(node.id)
    if (p) node.position = { ...p }
  }
  ElMessage.success('已完成拓扑分层排版（接入点 → 中转 → 落地）')
}

// 重置为默认一字排列
function resetLayout() {
  if (!props.topology) return
  const aps = props.topology.access_points || []
  let apY = 24
  for (const ap of aps) {
    boxPositions.set(`ap-${ap.id}`, snapPoint({ x: 40, y: apY }))
    apY += apHeightOf(ap.id) + CARD_GAP_Y
  }
  const offsetX = 40 + (aps.length > 0 ? AP_W + CARD_GAP_X : 0)
  props.topology.servers.forEach((s, idx) => {
    boxPositions.set(`server-${s.id}`, snapPoint({ x: offsetX + idx * (DEFAULT_BOX_W + CARD_GAP_X), y: 24 }))
    boxWidths.set(`server-${s.id}`, DEFAULT_BOX_W)
  })
  boxTagOrders.clear()
  localLayout.positions = Object.fromEntries(boxPositions)
  localLayout.widths = Object.fromEntries(boxWidths)
  localLayout.tag_orders = {}
  saveLayoutLocal(localLayout)
  layoutDirty.value = true

  for (const node of nodes.value) {
    const p = boxPositions.get(node.id)
    if (p) node.position = { ...p }
    if (node.data) {
      const b = node.data as BoxData
      b.boxWidth = DEFAULT_BOX_W
      if (props.topology && b.server?.id) {
        const rawOut = (props.topology.outbounds || []).filter((o) => o.server_id === b.server.id && o.protocol !== 'blackhole')
        const isDirect = (o: TopologyData['outbounds'][number]) => (o.tag === 'direct' || o.protocol === 'freedom') && !o.inbound_ref
        b.outbounds = rawOut.map((o) => ({ ...o, id: String(o.id), virtual: isDirect(o) }))
      }
    }
  }
  ElMessage.success('已重置为默认排列')
}

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value
  // 全屏时锁住背后页面的滚动，否则滚轮会滚动被遮住的页面
  document.body.style.overflow = isFullscreen.value ? 'hidden' : ''
  // 进出全屏后容器尺寸变了（640px ↔ 100vh），把视口重新适配一次：
  // 只有模板上的 :fit-view-on-init，调过缩放/拖动后点全屏会停在旧的 pan/zoom，图偏到一角
  nextTick(() => {
    setTimeout(() => flowRef.value?.fitView?.({ padding: 0.15 }), 260)
  })
}

onUnmounted(() => {
  document.body.style.overflow = ''
  clearTimeout(flashTimer)
})

const hasData = computed(() => !!props.topology && props.topology.servers.length > 0)

// 选中卡片的坐标读数（拖拽时 nodes 里的 position 会实时更新）
const selectionInfo = computed(() => {
  const node = (nodes.value as any[]).find((n) => n.selected)
  if (!node) return null
  const isAp = String(node.id).startsWith('ap-')
  return {
    kind: (isAp ? 'ap' : 'server') as 'ap' | 'server',
    name: isAp ? (node.data?.ap?.name ?? '') : (node.data?.server?.name ?? ''),
    x: Math.round(node.position?.x ?? 0),
    y: Math.round(node.position?.y ?? 0),
    w: Math.round(node.dimensions?.width || (isAp ? AP_W : getBoxWidth(node.id))),
    h: Math.round(node.dimensions?.height || (isAp ? AP_EST_H : 0)),
  }
})
</script>

<template>
  <!-- 画布是固定深色工作区：挂 awsui-dark-mode 让画布内的 Cloudscape 控件也走暗色令牌 -->
  <div ref="wrapEl" class="topology-wrap awsui-dark-mode" :class="{ 'is-fullscreen': isFullscreen }">
    <!-- 顶部微型操作栏 -->
    <div class="canvas-top-bar">
      <div class="top-bar-left">
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <CsButton icon-name="gen-ai" aria-label="根据拓扑链路（入口→中转→落地）自动分层排版" @click="autoLayout">
            自动排版
          </CsButton>
          <CsButton icon-name="refresh" aria-label="重置为默认一字排列" @click="resetLayout">
            重置排列
          </CsButton>
        </CsSpaceBetween>
        <CsButton
          v-if="editable"
          icon-name="add-plus"
          variant="primary"
          aria-label="新建接入点（订阅入口）"
          @click="openCreateAccessPoint"
        >
          接入点
        </CsButton>
        <CsButton
          :icon-name="isFullscreen ? 'exit-full-screen' : 'expand'"
          class="fs-btn"
          :aria-label="isFullscreen ? '退出全屏' : '全屏查看'"
          @click="toggleFullscreen"
        >
          {{ isFullscreen ? '退出全屏' : '全屏' }}
        </CsButton>
      </div>

      <!-- 布局未保存提示条 -->
      <div v-if="layoutDirty" class="layout-save-bar">
        <span class="layout-save-tip">布局已修改</span>
        <CsButton variant="primary" @click="saveLayoutToCloud">保存布局</CsButton>
      </div>
    </div>

    <VueFlow
      v-if="hasData"
      ref="flowRef"
      v-model:nodes="nodes"
      v-model:edges="edges"
      class="topology-canvas"
      :nodes-draggable="editable"
      :nodes-connectable="editable"
      :is-valid-connection="isValidConnection"
      :min-zoom="0.15"
      :max-zoom="2"
      :snap-to-grid="!!editable"
      :snap-grid="[GRID, GRID]"
      :only-render-visible-elements="true"
      @nodes-initialized="onNodesInitialized"
      @connect="handleConnect"
      @edge-click="handleEdgeClick"
      @node-double-click="handleNodeDblClick"
      @node-drag-stop="onNodeDragStop"
    >
      <!-- 双层网格：8px 细线 + 80px 主线。offset 必须传 gap/2 —— 库把线画在 tile 的
           正中，不补偿就会与世界坐标错开半格，卡片边界也就对不上线 -->
      <Background
        id="major"
        variant="lines"
        :gap="[GRID * 10, GRID * 10]"
        :offset="[GRID * 5, GRID * 5]"
        pattern-color="rgba(148, 163, 184, 0.14)"
      />
      <Background
        id="minor"
        variant="lines"
        :gap="[GRID, GRID]"
        :offset="[GRID / 2, GRID / 2]"
        :pattern-color="minorGridColor"
      />

      <!-- 盒内路由线：入站内点 → 出站内点的 S 形贝塞尔虚线 -->
      <template #edge-boxrule="e">
        <path
          class="vue-flow__edge-path vue-flow__edge-interaction edge-hit rule-hit"
          :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)"
        />
        <path class="boxrule-glow" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
        <path class="boxrule-path" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
      </template>

      <!-- 跨盒引用线与四层直通线：平滑 S 形贝塞尔 -->
      <template #edge-refedge="e">
        <path
          class="vue-flow__edge-path vue-flow__edge-interaction edge-hit ref-hit"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY)"
        />
        <path
          class="refedge-glow"
          :class="{ 'is-ap': e.data?.isAP, 'is-tunnel': e.data?.isTunnel }"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY)"
        />
        <path
          class="refedge-path"
          :class="{ 'is-ap': e.data?.isAP, 'is-tunnel': e.data?.isTunnel }"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY)"
          :marker-end="e.markerEnd"
        />
      </template>

      <!-- 用户接入点独立小盒（订阅入口；右侧端点拖线指向目标入站） -->
      <template #node-apbox="nodeProps">
        <APBoxNode
          :data="nodeProps.data"
          :editable="!!editable"
          :permission-groups="permissionGroups"
          @edit="openEditAccessPoint"
        />
      </template>

      <template #node-serverbox="nodeProps">
        <ServerBoxNode
          :data="nodeProps.data"
          :selected="!!nodeProps.selected"
          :editable="!!editable"
          :layer-groups="layerGroupsOf(nodeProps.data.server.id)"
          :native-inbounds="nativeInboundsOf(nodeProps.data.server.id)"
          @open-server="emit('open-server', $event)"
          @edit-inbound="openEditInbound"
          @create-inbound="openCreateInbound"
          @edit-outbound="openEditOutbound"
          @create-outbound="openCreateOutbound"
          @create-layer="openCreateLayer"
          @edit-layer="openEditLayer"
          @delete-layer="handleDeleteLayer"
          @bind-layer="openBindInbound"
          @unbind-inbound="unbindInbound"
          @copy-caddyfile="copyLayerCaddyfile"
          @move-inbound-order="moveInboundOrder"
          @move-outbound-order="moveOutboundOrder"
          @auto-align="autoAlignServerTags"
          @resize-start="startBoxResize"
        />
      </template>
    </VueFlow>
    <div v-else class="topology-empty">
      <CsEmpty description="暂无服务器。先到「服务器管理」页新增服务器，再回来拖线连接。" />
    </div>

    <!-- 坐标参照：顶部/左侧标尺 + 世界原点 + 底部选中卡片读数 -->
    <CanvasRulers
      v-if="hasData"
      :viewport="viewport"
      :dimensions="dimensions"
      :selection="selectionInfo"
    />

    <!-- 出站新建/编辑弹窗 -->
    <OutboundConfigEditor
      v-if="outboundEditorOpen"
      :server-id="outboundServerId"
      :outbound="outboundEditing"
      @saved="handleOutboundSaved"
      @close="outboundEditorOpen = false"
    />

    <!-- 用户接入点新建/编辑弹窗 (Consumer Pipeline Model) -->
    <AccessPointDialog
      v-model="apDialogOpen"
      :access-point="apEditingItem"
      :servers="availableXrayServers"
      :inbounds="props.topology?.inbounds"
      :permission-groups="permissionGroups"
      @saved="emit('changed')"
      @deleted="emit('changed')"
    />

    <!-- 入站新建/编辑弹窗 -->
    <InboundDialog
      v-model:visible="inboundCreateOpen"
      :server-id="inboundCreateServerId"
      :inbound="inboundEditingItem"
      @changed="emit('changed')"
    />

    <!-- 路由规则弹窗 -->
    <RuleDialog
      v-model:visible="ruleOpen"
      :server-id="ruleServerId"
      :inbound-tag="ruleInboundTag"
      :outbound-tag="ruleOutboundTag"
      @changed="emit('changed')"
    />

    <!-- 对外接入层新建/编辑弹窗 -->
    <LayerDialog
      v-model:visible="layerDialogOpen"
      :server-id="layerServerId"
      :layer="layerEditing"
      @changed="emit('changed')"
      @delete="handleDeleteLayer"
    />

    <!-- 入站挂载对外层弹窗 -->
    <BindInboundDialog
      v-model:visible="bindDialogOpen"
      :layer-id="bindLayerId"
      :candidates="availableBindInbounds"
      @changed="emit('changed')"
    />
  </div>
</template>

<style scoped lang="scss">
.topology-wrap {
  /* 画布是固定深色工作区，自带一套令牌：两张卡片/连线/工具条共用，不跟随全局亮暗主题 */
  --topo-radius: 16px;
  --topo-radius-sm: 12px;
  --topo-card-bg: rgba(24, 33, 47, 0.95);
  --topo-card-border: rgba(255, 255, 255, 0.1);
  --topo-card-shadow: 0 6px 24px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.1);
  --topo-surface: rgba(15, 23, 42, 0.55);
  --topo-surface-border: rgba(255, 255, 255, 0.05);
  --topo-text-1: #e2e8f0;
  --topo-text-2: #94a3b8;
  --topo-text-3: #64748b;
  --topo-accent: #38bdf8;
  --topo-success: #34d399;
  --topo-warning: #fbbf24;
  --topo-danger: #f87171;
  --topo-teal: #2dd4bf;

  height: var(--topo-fill-h, 640px);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius-sm, 8px);
  overflow: hidden;
  background: #0f172a;
  position: relative;
  /* 不把 height 放进过渡：窗口拉伸时逐帧改高度会拖出 250ms 的滞后 */
  transition: border-radius 0.25s ease-in-out, border-color 0.25s ease-in-out,
    background-color 0.25s ease-in-out;

  &.is-fullscreen {
    position: fixed;
    // inset:0 已经把盒子钉在包含块（视口）上，再写 width:100vw 反而按「视口含
    // 滚动条」的宽度计算，Windows/Linux 传统滚动条下右侧会被裁掉约 15px 并出现横向滚动条
    inset: 0;
    width: 100%;
    height: 100%;
    // 移动端地址栏收缩时 100vh 不会跟着变，底部工具栏会被压住；dvh 跟随动态视口。
    // 不支持 dvh 的浏览器沿用上一行的 100%
    height: 100dvh;
    z-index: var(--x-z-fullscreen, 1500);
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
  /* 让开 20px 标尺 + 8px 间距，避免工具条压住刻度 */
  top: 28px;
  left: 28px;
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
    background: rgba(15, 23, 42, 0.94);
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
  background: rgba(15, 23, 42, 0.94);
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

:deep(.ap-node .ep) {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #0f172a;
  border: 2px solid rgba(255, 255, 255, 0.4);
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
}

:deep(.ap-node .ap-src-handle) {
  right: -6px !important;
  top: 50% !important;
  transform: translateY(-50%) !important;
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
    opacity: 1;
    border-color: rgba(255, 255, 255, 0.35);
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
  &.tunnel-ep-left {
    left: -6px !important;
    top: 50% !important;
    transform: translateY(-50%) !important;
    border-color: #22d3ee !important;
    background: #0f172a !important;
    opacity: 1 !important;
    z-index: 5 !important;
    cursor: crosshair;
    &:hover {
      border-color: #38bdf8 !important;
      background: rgba(56, 189, 248, 0.3) !important;
      box-shadow: 0 0 10px rgba(34, 211, 238, 0.8) !important;
    }
  }
  &.tunnel-ep-right {
    right: -6px !important;
    top: 50% !important;
    transform: translateY(-50%) !important;
    border-color: #22d3ee !important;
    background: #0f172a !important;
    opacity: 1 !important;
    z-index: 5 !important;
    cursor: crosshair;
    &:hover {
      border-color: #38bdf8 !important;
      background: rgba(56, 189, 248, 0.3) !important;
      box-shadow: 0 0 10px rgba(34, 211, 238, 0.8) !important;
    }
  }
}

:deep(.vue-flow__edge-path) {
  stroke-width: 2;
}
:deep(.vue-flow__minimap) {
  background: #0f172a;
}

/* 新卡片定位后的短暂高亮：用 outline 而不是 box-shadow，避免覆盖卡片自身状态阴影 */
:deep(.vue-flow__node.just-created .server-box),
:deep(.vue-flow__node.just-created .ap-node) {
  outline: 2px solid rgba(56, 189, 248, 0.9);
  outline-offset: 3px;
  animation: topo-created-glow 1.1s ease-out 2;
}
@keyframes topo-created-glow {
  0%,
  100% {
    outline-color: rgba(56, 189, 248, 0.9);
  }
  50% {
    outline-color: rgba(56, 189, 248, 0.15);
  }
}
</style>

<style>
/* 连线层必须压在节点层之上，这不是笔误：
   1) type:'boxrule' 的边两端都在同一个节点上（source=server-N/inb-src-X，
      target=server-N/out-tgt-Y），画的就是服务器卡片内部的「入站行 → 出站行」路由虚线。
      一旦把 edges 压到 nodes 之下，这些盒内走线会被卡片背景整片盖住、完全不可见。
   2) 跨盒 refedge 允许直穿中间盒子（见 refEdgePath 上方的取舍说明），
      也是靠这一层实现的。
   因此两者关系保持 edges(100) > nodes(10) 不变；
   真正错的是「选中态」与普通节点同为 10 —— 选中/拖拽不再置顶，重叠卡片顺序错乱。
   把选中态提到 50：高于其它节点、仍低于连线层，选中卡片自己的盒内走线照常可见。 */
.topology-wrap .vue-flow__edges {
  z-index: 100 !important;
  pointer-events: none !important;
}

.topology-wrap .vue-flow__nodes {
  z-index: 10 !important;
}

.topology-wrap .vue-flow__node {
  z-index: 10 !important;
}

.topology-wrap .vue-flow__node.selected,
.topology-wrap .vue-flow__node.dragging {
  z-index: 50 !important;
}

.boxrule-glow {
  stroke: #0284c7;
  stroke-width: 6;
  opacity: 0.22;
  fill: none;
  stroke-linecap: round;
  transition:
    stroke 0.2s ease,
    stroke-width 0.2s ease,
    opacity 0.2s ease;
  pointer-events: none !important;
}
.boxrule-path {
  stroke: #38bdf8;
  stroke-width: 2;
  stroke-dasharray: 6 5;
  fill: none;
  stroke-linecap: round;
  transition:
    stroke 0.2s ease,
    stroke-width 0.2s ease,
    opacity 0.2s ease;
  pointer-events: none !important;
}

.refedge-glow {
  stroke: #d97706;
  stroke-width: 8;
  opacity: 0.25;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
  transition:
    stroke 0.2s ease,
    stroke-width 0.2s ease,
    opacity 0.2s ease;
  pointer-events: none !important;

  &.is-ap {
    stroke: #0284c7;
    opacity: 0.28;
  }
  &.is-tunnel {
    stroke: #0891b2;
    opacity: 0.35;
  }
}
.refedge-path {
  stroke: #fbbf24;
  stroke-width: 2.5;
  stroke-linejoin: round;
  stroke-linecap: round;
  fill: none;
  transition:
    stroke 0.2s ease,
    stroke-width 0.2s ease,
    opacity 0.2s ease;
  pointer-events: none !important;

  &.is-ap {
    stroke: #38bdf8;
    stroke-dasharray: 6 4;
  }
  &.is-tunnel {
    stroke: #06b6d4;
    stroke-dasharray: 8 4;
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
  opacity: 0.45;
  stroke-width: 7;
}
.vue-flow__edge:not(.selected):hover .boxrule-path {
  stroke: #e0f2fe;
  stroke-width: 2.5;
}
.vue-flow__edge:not(.selected):hover .refedge-glow {
  opacity: 0.5;
  stroke-width: 9;
}
.vue-flow__edge:not(.selected):hover .refedge-glow.is-tunnel {
  opacity: 0.6;
  stroke-width: 10;
}
.vue-flow__edge:not(.selected):hover .refedge-path {
  stroke: #fef3c7;
  stroke-width: 3;
}
.vue-flow__edge:not(.selected):hover .refedge-path.is-tunnel {
  stroke: #67e8f9;
  stroke-width: 3.2;
}

@media (max-width: 768px) {
  .server-box,
  .ap-node {
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.35) !important;
  }
}

.custom-resizer-right {
  position: absolute;
  /* 原来是 top: 38px + translateY(-50%)：这个 38px 与抓手自身 36px 高、与 .sb-head 的
     48px 高都没有对应关系（纯反推值），标题栏高度一变抓手就跑到卡片外。
     改成上下贴边 + margin:auto，抓手恒在卡片垂直居中处，与任何高度常量解耦。 */
  top: 0;
  bottom: 0;
  right: -3px;
  margin: auto 0;
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
