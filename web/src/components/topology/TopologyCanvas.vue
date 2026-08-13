<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
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
  type NodeProps,
} from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Delete } from '@element-plus/icons-vue'
import {
  createServerOutbound,
  createServerRoutingRule,
  deleteServerOutbound,
  deleteServerRoutingRule,
  updateInbound,
  updateServerOutbound,
  type TopologyData,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const props = defineProps<{
  topology: TopologyData | null
  editable?: boolean
}>()

const emit = defineEmits<{
  (e: 'changed'): void
  (e: 'open-server', serverId: number): void
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
  outbounds: BoxOutbound[]
}

const nodes = ref<GraphNode[]>([])
const edges = ref<Edge[]>([])

// 模块级：会话内记住盒子拖动位置——连线/删线/视图切换后 buildGraph 重建节点时保持，
// 不回到默认布局（仅新服务器用默认位置）
const boxPositions = new Map<string, { x: number; y: number }>()

const ROW_H = 40 // 增加行高以适配药丸样式
const HEADER_H = 48
const TITLE_H = 30
const BOX_W = 440 // 与 .server-box 宽度一致（CSS），端点坐标计算用

// 盒子渲染高度（与模板布局一致）：头部 + 标题 + 行数 + 底部边距
function boxHeight(inbCount: number, outCount: number) {
  return HEADER_H + TITLE_H + Math.max(Math.max(inbCount, outCount), 1) * ROW_H + 16
}

// 节点当前位置（记住的拖动位置或默认布局）
function nodePosOf(s: TopologyData['servers'][number], idx: number) {
  return boxPositions.get(`server-${s.id}`) ?? { x: 40 + idx * 520, y: 24 }
}

function typeInfo(t?: string) {
  if (t === 'relay') return { cls: 'relay', text: '转发' }
  if (t === 'idle') return { cls: 'idle', text: '闲置' }
  return { cls: 'user', text: '用户' }
}



// 盒内路由线：S 形贝塞尔——入站内点（盒中线左）→ 出站内点（盒中线右），横穿两列之间的
// 走线走廊（标签都靠边，走廊空旷），无论行差多少都不经过任何标签
function boxRulePath(sx: number, sy: number, tx: number, ty: number) {
  const mx = (sx + tx) / 2
  return `M ${sx} ${sy} C ${mx} ${sy}, ${mx} ${ty}, ${tx} ${ty}`
}

// 跨盒引用线（方案④ 直-弧-直）：水平直出/直入 30px（垂直盒缘）→ 贝塞尔弧过渡 → 水平直入，
// 电路板走线式；被其他盒子阻挡时改下方 U 形贝塞尔绕行（drop 到被挡盒之下）
function refEdgePath(sx: number, sy: number, tx: number, ty: number, detour: boolean, drop: number) {
  const dir = tx >= sx ? 1 : -1
  if (detour) {
    return `M ${sx} ${sy} C ${sx} ${drop}, ${tx} ${drop}, ${tx} ${ty}`
  }
  const span = Math.abs(tx - sx)
  const seg = Math.min(30, span * 0.15)
  const ax = sx + seg * dir
  const bx = tx - seg * dir
  const arc = Math.abs(bx - ax) * 0.25
  return `M ${sx} ${sy} L ${ax} ${sy} C ${ax + arc * dir} ${sy}, ${bx - arc * dir} ${ty}, ${bx} ${ty} L ${tx} ${ty}`
}

function buildGraph(data: TopologyData) {
  const inbByServer = new Map<number, TopologyData['inbounds']>()
  for (const inb of data.inbounds) {
    if (!inbByServer.has(inb.server_id)) inbByServer.set(inb.server_id, [])
    inbByServer.get(inb.server_id)!.push(inb)
  }
  // 出站按服务器分组；blackhole/blocked 不参与拓扑（服务器表单单独管理），
  // 每盒固定追加一行虚拟「direct」直连出站（模板 freedom，tag=direct）
  const outByServer = new Map<number, BoxOutbound[]>()
  for (const out of data.outbounds) {
    if (out.protocol === 'blackhole') continue
    if (!outByServer.has(out.server_id)) outByServer.set(out.server_id, [])
    outByServer.get(out.server_id)!.push({ ...out, id: String(out.id) })
  }
  for (const s of data.servers) {
    const list = outByServer.get(s.id) ?? []
    list.push({
      id: `direct-${s.id}`,
      tag: s.default_outbound_tag || 'direct',
      protocol: 'freedom',
      inbound_ref: null,
      virtual: true,
    })
    outByServer.set(s.id, list)
  }

  nodes.value = data.servers.map((s, idx) => ({
    id: `server-${s.id}`,
    type: 'serverbox',
    position: boxPositions.get(`server-${s.id}`) ?? { x: 40 + idx * 520, y: 24 },
    data: {
      server: s,
      inbounds: inbByServer.get(s.id) ?? [],
      outbounds: outByServer.get(s.id) ?? [],
    } as BoxData,
  })) as unknown as GraphNode[]

  // 入站/出站 id → 节点 id 映射（边定位；虚拟 direct 无 DB id，不参与引用边）
  const inbNode = new Map<number, string>()
  const outNode = new Map<number, string>()
  for (const s of data.servers) {
    for (const inb of inbByServer.get(s.id) ?? []) inbNode.set(inb.id, `server-${s.id}`)
    for (const out of data.outbounds) {
      if (out.server_id === s.id) outNode.set(out.id, `server-${s.id}`)
    }
  }

  const idxByServer = new Map(data.servers.map((s, i) => [s.id, i]))

  const es: Edge[] = []
  // InboundRef 实线（出站 → 落地入站；方案④ 直-弧-直；线路径被其他盒子阻挡 → 下方 U 形绕行）
  for (const out of data.outbounds) {
    if (out.protocol === 'blackhole' || !out.inbound_ref || !inbNode.has(out.inbound_ref)) continue
    const inb = data.inbounds.find((i) => i.id === out.inbound_ref)
    if (!inb) continue
    // 端点绝对坐标：出站右缘 / 入站左缘，行中心
    const srcPos = nodePosOf(data.servers.find((s) => s.id === out.server_id)!, idxByServer.get(out.server_id) ?? 0)
    const outList = outByServer.get(out.server_id) ?? []
    const srcRow = outList.findIndex((o) => o.id === String(out.id))
    const sx = srcPos.x + BOX_W
    const sy = srcPos.y + HEADER_H + TITLE_H + srcRow * ROW_H + ROW_H / 2
    const tgtPos = nodePosOf(data.servers.find((s) => s.id === inb.server_id)!, idxByServer.get(inb.server_id) ?? 0)
    const inbList = inbByServer.get(inb.server_id) ?? []
    const tgtRow = inbList.findIndex((i) => i.id === inb.id)
    const tx = tgtPos.x
    const ty = tgtPos.y + HEADER_H + TITLE_H + tgtRow * ROW_H + ROW_H / 2
    // 阻挡检测：线的 x 全程、y 在两端点之间，与任一其他盒子矩形相交 → 下方绕行
    let detour = false
    let blockBottom = 0
    for (const s2 of data.servers) {
      if (s2.id === out.server_id || s2.id === inb.server_id) continue
      const p2 = nodePosOf(s2, idxByServer.get(s2.id) ?? 0)
      const h2 = boxHeight(inbByServer.get(s2.id)?.length ?? 0, outByServer.get(s2.id)?.length ?? 0)
      if (
        p2.x < Math.max(sx, tx) &&
        p2.x + BOX_W > Math.min(sx, tx) &&
        p2.y < Math.max(sy, ty) &&
        p2.y + h2 > Math.min(sy, ty)
      ) {
        detour = true
        blockBottom = Math.max(blockBottom, p2.y + h2)
      }
    }
    const drop = detour ? Math.max(blockBottom + 50, Math.max(sy, ty) + 140) : 0
    es.push({
      id: `ref-${out.id}`,
      source: outNode.get(out.id)!,
      sourceHandle: `out-src-${out.id}`,
      target: inbNode.get(out.inbound_ref)!,
      targetHandle: `inb-tgt-${out.inbound_ref}`,
      type: 'refedge',
      animated: true,
      markerEnd: { type: MarkerType.ArrowClosed },
      data: { detour, drop },
    })
  }
  // 路由规则盒内虚线（入站 → 出站；目标出站匹配 DB 出站或虚拟 direct；inbound_tag 为空的全匹配规则不画）
  for (const rule of data.routing_rules) {
    if (!rule.inbound_tag || !rule.enabled) continue
    const inb = data.inbounds.find((i) => i.tag === rule.inbound_tag && i.server_id === rule.server_id)
    if (!inb) continue
    const srv = data.servers.find((s) => s.id === rule.server_id)
    const out = data.outbounds.find((o) => o.tag === rule.outbound_tag && o.server_id === rule.server_id && o.protocol !== 'blackhole')
    let targetHandle: string
    if (out) {
      targetHandle = `out-tgt-${out.id}`
    } else if (srv && rule.outbound_tag === (srv.default_outbound_tag || 'direct')) {
      targetHandle = `out-tgt-direct-${rule.server_id}`
    } else {
      continue
    }
    es.push({
      id: `rule-${rule.id}`,
      source: `server-${rule.server_id}`,
      sourceHandle: `inb-src-${inb.id}`,
      target: `server-${rule.server_id}`,
      targetHandle,
      type: 'boxrule',
      markerEnd: { type: MarkerType.ArrowClosed },
    })
  }
  edges.value = es
}

watch(
  () => props.topology,
  (t) => {
    if (t) buildGraph(t)
  },
  { immediate: true },
)

// ---- 交互：拖线 ----
// 双点语义：盒内点（中线两侧）= 仅服务器内连接（入站内点 → 出站内点 = 路由规则）；
//           边缘点（盒缘）= 对外接口（入站外点收引用线/发跨盒中转，出站外点发引用线，
//           direct 绿点收本盒规则线）。入站外点为重叠双 handle（source+target 同位置，
//           @vue-flow 落点按几何最近命中任意一个 → 等价匹配）。
async function handleConnect(conn: Connection) {
  if (!props.editable || !props.topology) return
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const outSrc = src.match(/^out-src-(\d+)$/) // 出站外点：发引用
  const inbSrcExt = src.match(/^inb-src-ext-(\d+)$/) // 入站外点：发跨盒中转
  const inbSrc = src.match(/^inb-src-(\d+)$/) // 入站内点：发盒内规则
  const inbAny = tgt.match(/^(?:inb-src-ext|inb-tgt)-(\d+)$/) // 入站外点落点（引用/中转）
  const outAny = tgt.match(/^(?:out-src|out-tgt)-(?:(\d+)|direct-(\d+))$/) // 出站落点（含 direct 绿点）

  if (outSrc && inbAny) {
    await createRef(Number(outSrc[1]), Number(inbAny[1]))
  } else if (inbSrcExt && inbAny) {
    await createViaOutbound(Number(inbSrcExt[1]), Number(inbAny[1]))
  } else if (inbSrc && outAny) {
    // outAny[1] = DB 出站 id；outAny[2] = 虚拟 direct（direct-<serverId>）
    if (outAny[2]) {
      openDirectRuleDialog(Number(inbSrc[1]), Number(outAny[2]))
    } else {
      openRuleDialog(Number(inbSrc[1]), Number(outAny[1]))
    }
  } else if (inbSrc && inbAny) {
    ElMessage.warning('盒内端点仅限服务器内连接（入站 → 出站）；跨服务器请从盒子边缘端点拖出')
  } else {
    ElMessage.warning('仅支持：入站内点 → 出站（盒内规则）、入站边缘点 → 他服务器入站（自动建中转出站）、出站边缘点 → 入站（设置引用）')
  }
}

// 入站 → 他服务器入站：自动创建「via 出站」（vless 引用目标落地）+ 自动创建路由规则（源入站 → 新出站）
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
  // 出站 tag 自动命名：via-<目标入站 tag>，重名加序号
  let tag = `via-${tgtInb.tag}`
  const sameServerOuts = data.outbounds.filter((o) => o.server_id === srcInb.server_id)
  let n = 2
  while (sameServerOuts.some((o) => o.tag === tag)) tag = `via-${tgtInb.tag}-${n++}`
  // 目标闲置：自动升级为 relay（落地转发），弹窗提示
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
  // 目标闲置：自动升级为 relay（落地转发），弹窗提示
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

// 入站 → 虚拟 direct 出站（目标 tag = 服务器默认出口）
function openDirectRuleDialog(inboundId: number, serverId: number) {
  const data = props.topology!
  const inb = data.inbounds.find((i) => i.id === inboundId)
  const srv = data.servers.find((s) => s.id === serverId)
  if (!inb || !srv) return
  if (inb.server_id !== serverId) {
    ElMessage.warning('路由规则只能在同一服务器内（入站 → 出站）')
    return
  }
  ruleForm.serverId = inb.server_id
  ruleForm.inboundTag = inb.tag
  ruleForm.outboundTag = srv.default_outbound_tag || 'direct'
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
async function handleEdgeClick(evt: EdgeMouseEvent) {
  const edge = evt.edge
  if (!props.editable || !props.topology) return
  if (edge.id.startsWith('ref-')) {
    const outboundId = Number(edge.id.slice(4))
    const out = props.topology.outbounds.find((o) => o.id === outboundId)
    if (!out) return
    // 拖线自动创建的 via 出站：删引用 = 连同出站与路由规则一起清理（后端级联删规则 + 目标回退原类型）
    if (out.tag.startsWith('via-')) {
      try {
        await ElMessageBox.confirm(
          `删除自动创建的出站「${out.tag}」及其路由规则？目标落地入站将回退到引用前类型。`,
          '删除中转出站',
          { type: 'warning' },
        )
      } catch {
        return
      }
      try {
        const { data } = await deleteServerOutbound(out.server_id, out.id)
        if (data.code === 0) {
          ElMessage.success('已删除中转出站与路由规则')
          emit('changed')
        } else {
          ElMessage.error(data.message)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '删除失败'))
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
      return
    }
    try {
      const { data } = await updateServerOutbound(out.server_id, out.id, { inbound_ref: 0 })
      if (data.code === 0) {
        ElMessage.success('已解除引用')
        emit('changed')
      } else {
        ElMessage.error(data.message)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '解除引用失败'))
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
      return
    }
    try {
      const { data } = await deleteServerRoutingRule(rule.server_id, ruleId)
      if (data.code === 0) {
        ElMessage.success('已删除')
        emit('changed')
      } else {
        ElMessage.error(data.message)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '删除规则失败'))
    }
  }
}

// ---- 交互：双击盒子 → 服务器抽屉 ----
function handleNodeDblClick(evt: NodeMouseEvent) {
  const box = evt.node.data as BoxData
  if (box?.server?.id) emit('open-server', box.server.id)
}

// 盒子拖动结束：记住位置（连线/删线重建节点后保持）
function onNodeDragStop(evt: NodeDragEvent) {
  const node = evt.node
  if (node.position) boxPositions.set(node.id, { x: node.position.x, y: node.position.y })
}

const hasData = computed(() => !!props.topology && props.topology.servers.length > 0)
</script>

<template>
  <div class="topology-wrap">
    <VueFlow
      v-if="hasData"
      v-model:nodes="nodes"
      v-model:edges="edges"
      class="topology-canvas"
      :nodes-draggable="editable"
      :nodes-connectable="editable"
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
      <!-- 盒内路由线：入站内点 → 出站内点的 S 形贝塞尔虚线（宽透明 hit path 保证可点删；同色光晕底增强显眼度） -->
      <template #edge-boxrule="e">
        <path class="edge-hit" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
        <path class="boxrule-glow" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" />
        <path class="boxrule-path" :d="boxRulePath(e.sourceX, e.sourceY, e.targetX, e.targetY)" :marker-end="e.markerEnd" />
      </template>
      <!-- 跨盒引用线：方案④ 直-弧-直（被盒子阻挡时下方 U 形绕行），流动虚线+箭头 -->
      <template #edge-refedge="e">
        <path
          class="edge-hit"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
        />
        <path
          class="refedge-path vue-flow__edge-path"
          :d="refEdgePath(e.sourceX, e.sourceY, e.targetX, e.targetY, !!e.data?.detour, e.data?.drop ?? 0)"
          :marker-end="e.markerEnd"
        />
      </template>
      <template #node-serverbox="{ data }: NodeProps<BoxData>">
        <div class="server-box" :class="{ offline: !data.server.status }">
          <div class="sb-head">
            <span class="status-dot" :class="data.server.status === 1 ? 'online' : 'offline'" />
            <span class="name">{{ data.server.name }}</span>
            <span class="host">{{ data.server.host }}</span>
          </div>

          <div class="sb-cols">
            <!-- 入站列：边缘外点（重叠双 handle）= 对外接口（收引用线 + 发跨盒中转）；标签贴左；
                 盒内点（中线左）= 发盒内路由线 -->
            <div class="sb-col">
              <div class="sb-title">入站</div>
              <template v-if="data.inbounds.length > 0">
                <div v-for="(inb, i) in data.inbounds" :key="inb.id" class="sb-row">
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
                  <span class="type-tag" :class="typeInfo(inb.type).cls">{{ typeInfo(inb.type).text }}</span>
                  <span class="tag">{{ inb.tag }}</span>
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

            <!-- 出站列：盒内点（中线右）= 收盒内路由线；边缘外点（右缘）= 发引用线；
                 direct 虚拟行：绿点（右缘）收本盒规则线，不参与引用拖线；标签贴右 -->
            <div class="sb-col">
              <div class="sb-title">出站</div>
              <template v-if="data.outbounds.length > 0">
                <div v-for="(out, i) in data.outbounds" :key="out.id" class="sb-row" :class="{ 'direct-row': out.virtual }">
                  <Handle
                    type="target"
                    :id="`out-tgt-${out.id}`"
                    :position="Position.Left"
                    :connectable="editable"
                    class="ep in-ep"
                  />
                  <span class="tag out-tag" :class="out.inbound_ref ? 'ref' : out.virtual ? 'direct' : ''">{{ out.tag }}</span>
                  <Handle
                    v-if="!out.virtual"
                    type="source"
                    :id="`out-src-${out.id}`"
                    :position="Position.Right"
                    :connectable="editable"
                    class="ep ext-src"
                  />
                  <Handle
                    v-else
                    type="target"
                    :id="`out-tgt-${out.id}`"
                    :position="Position.Right"
                    :connectable="editable"
                    class="ep direct-ep"
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

    <!-- 路由规则弹窗 -->
    <el-dialog v-model="ruleOpen" title="新建路由规则（拖线）" width="520px" append-to-body>
      <el-form label-position="top">
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="入站（源）">
            <el-input :model-value="ruleForm.inboundTag" disabled />
          </el-form-item>
          <el-form-item label="出站（目标）">
            <el-input :model-value="ruleForm.outboundTag" disabled />
          </el-form-item>
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
}
.topology-canvas {
  width: 100%;
  height: 100%;
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
  margin: 0 0 10px;
}

/* ---- ServerBox 自定义节点 ---- */
.server-box {
  width: 440px;
  background: rgba(30, 41, 59, 0.7);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 16px;
  font-size: 12px;
  color: #e2e8f0;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3), inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: visible;
  transition: all 0.3s ease;
  &.offline {
    opacity: 0.6;
    border-color: rgba(255, 255, 255, 0.05);
  }
  .sb-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 16px;
    height: 48px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    background: linear-gradient(180deg, rgba(255,255,255,0.06) 0%, transparent 100%);
    border-radius: 16px 16px 0 0;
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
        background: #64748b;
      }
    }
    .name {
      font-weight: 600;
      font-size: 14px;
      white-space: nowrap;
      text-shadow: 0 2px 4px rgba(0,0,0,0.5);
    }
    .host {
      margin-left: auto;
      color: #94a3b8;
      font-size: 11px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 130px;
    }
  }
  .sb-cols {
    display: flex;
    gap: 80px; /* 拉大中央间距，留出连线空间 */
    justify-content: space-between;
    padding: 0 16px 16px;
    width: 440px;
    box-sizing: border-box;
    .sb-col {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 6px;
      align-items: flex-start; /* 自动收缩适应标签宽度 */
    }
    .sb-col:last-child {
      align-items: flex-end;
      .sb-title {
        text-align: right;
      }
    }
    .sb-title {
      height: 30px;
      width: 100%;
      box-sizing: border-box;
      padding: 8px 4px 0;
      color: #94a3b8;
      font-size: 12px;
      letter-spacing: 0.5px;
      font-weight: 600;
      text-transform: uppercase;
    }
  }
  .sb-row {
    position: relative; /* 关键：让绝对定位的 Handle 吸附在药丸边缘 */
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px;
    height: 34px;
    max-width: 100%;
    box-sizing: border-box;
    background: rgba(15, 23, 42, 0.5);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 17px; /* Pill shape */
    transition: all 0.2s;
    &:hover {
      background: rgba(15, 23, 42, 0.8);
      border-color: rgba(255, 255, 255, 0.15);
    }

    .tag {
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100px;
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
    .type-tag {
      flex: none;
      font-size: 10px;
      padding: 2px 6px;
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

/* 端点（双点结构）：
   边缘点（盒缘）= 对外接口——ext-src 常显（发线，hover 蓝环）；ext-tgt 透明纯落点（与 ext-src 重叠，
   @vue-flow 落点按几何最近命中任意一个），引用线/跨盒中转线收在此处
   内点（盒中线两侧）= 盒内路由连接点，边框稍弱区分；direct 绿点（右缘）= 收本盒规则线 */
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
  /* 内点（盒内路由连接点）：边框稍弱，视觉上区分于外点 */
  &.in-ep {
    border-color: rgba(255, 255, 255, 0.15);
  }
  /* direct 虚拟行绿点（右缘，收本盒规则线） */
  &.direct-ep {
    border-color: #34d399;
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

<!-- 全局样式：
     1) edges 层提升到 nodes 之上（z-index 1000，压过一切）：盒内路由线/引用线始终可点可删，
        拖动盒子后也不会被节点层盖住（每条边是独立 <svg class="vue-flow__edges"> 带内联 z-index: 0，
        需 !important 压过；节点内联 z-index 恒为 0）
     2) 自定义边 path 的样式不能放 scoped :deep——slot 内容的祖先链全是 vue-flow 内部
        svg/g，无 data-v 祖先，:deep 选择器匹配不到 -->
<style>
.vue-flow__edges {
  z-index: 9999 !important;
}
/* 盒内路由线（自定义 boxrule 边）：亮蓝虚线 + 同色光晕底，深色背景上清晰显眼 */
.boxrule-glow {
  stroke: #38bdf8;
  stroke-width: 6;
  opacity: 0.2;
  fill: none;
  filter: drop-shadow(0 0 4px rgba(56, 189, 248, 0.4));
}
.boxrule-path {
  stroke: #bae6fd;
  stroke-width: 2;
  stroke-dasharray: 6 4;
  fill: none;
}
/* 跨盒引用线（自定义 refedge 边；animated 类驱动流动虚线） */
.refedge-path {
  stroke: #fbbf24;
  stroke-width: 3;
  stroke-linejoin: round;
  fill: none;
  filter: drop-shadow(0 0 6px rgba(251, 191, 36, 0.5));
}
/* 宽透明命中路径：细线也易于点选删除 */
.edge-hit {
  stroke: transparent;
  stroke-width: 14;
  fill: none;
}
</style>
