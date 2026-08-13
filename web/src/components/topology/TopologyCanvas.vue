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
  createServerRoutingRule,
  deleteServerRoutingRule,
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
interface BoxData {
  server: TopologyData['servers'][number]
  inbounds: TopologyData['inbounds']
  outbounds: TopologyData['outbounds']
}

const nodes = ref<GraphNode[]>([])
const edges = ref<Edge[]>([])

const ROW_H = 30
const HEADER_H = 44
const TITLE_H = 26

function typeInfo(t?: string) {
  if (t === 'relay') return { cls: 'relay', text: '转发' }
  if (t === 'idle') return { cls: 'idle', text: '闲置' }
  return { cls: 'user', text: '用户' }
}

function transportOf(stream: string): string {
  try {
    const s = JSON.parse(stream || '{}')
    return `${s.network || 'tcp'}/${s.security || 'none'}`
  } catch {
    return 'tcp/none'
  }
}

function buildGraph(data: TopologyData) {
  const inbByServer = new Map<number, TopologyData['inbounds']>()
  for (const inb of data.inbounds) {
    if (!inbByServer.has(inb.server_id)) inbByServer.set(inb.server_id, [])
    inbByServer.get(inb.server_id)!.push(inb)
  }
  const outByServer = new Map<number, TopologyData['outbounds']>()
  for (const out of data.outbounds) {
    if (!outByServer.has(out.server_id)) outByServer.set(out.server_id, [])
    outByServer.get(out.server_id)!.push(out)
  }

  nodes.value = data.servers.map((s, idx) => ({
    id: `server-${s.id}`,
    type: 'serverbox',
    position: { x: 40 + idx * 400, y: 24 },
    data: {
      server: s,
      inbounds: inbByServer.get(s.id) ?? [],
      outbounds: outByServer.get(s.id) ?? [],
    } as BoxData,
  })) as unknown as GraphNode[]

  // 入站/出站 id → 节点 id 映射（边定位）
  const inbNode = new Map<number, string>()
  const outNode = new Map<number, string>()
  for (const s of data.servers) {
    for (const inb of inbByServer.get(s.id) ?? []) inbNode.set(inb.id, `server-${s.id}`)
    for (const out of outByServer.get(s.id) ?? []) outNode.set(out.id, `server-${s.id}`)
  }

  const es: Edge[] = []
  // InboundRef 实线（出站 → 落地入站）
  for (const out of data.outbounds) {
    if (!out.inbound_ref || !inbNode.has(out.inbound_ref)) continue
    es.push({
      id: `ref-${out.id}`,
      source: outNode.get(out.id)!,
      sourceHandle: `out-src-${out.id}`,
      target: inbNode.get(out.inbound_ref)!,
      targetHandle: `inb-tgt-${out.inbound_ref}`,
      type: 'smoothstep',
      animated: true,
      markerEnd: { type: MarkerType.ArrowClosed },
      style: { stroke: '#f59e0b', strokeWidth: 2 },
      label: '引用',
    })
  }
  // 路由规则盒内虚线（入站 → 出站，inbound_tag 为空的全匹配规则不画）
  for (const rule of data.routing_rules) {
    if (!rule.inbound_tag || !rule.enabled) continue
    const inb = data.inbounds.find((i) => i.tag === rule.inbound_tag && i.server_id === rule.server_id)
    const out = data.outbounds.find((o) => o.tag === rule.outbound_tag && o.server_id === rule.server_id)
    if (!inb || !out) continue
    es.push({
      id: `rule-${rule.id}`,
      source: `server-${rule.server_id}`,
      sourceHandle: `inb-src-${inb.id}`,
      target: `server-${rule.server_id}`,
      targetHandle: `out-tgt-${out.id}`,
      type: 'smoothstep',
      style: { strokeDasharray: '6 4', stroke: '#94a3b8' },
      label: '路由',
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
async function handleConnect(conn: Connection) {
  if (!props.editable || !props.topology) return
  const src = conn.sourceHandle ?? ''
  const tgt = conn.targetHandle ?? ''
  const outSrc = src.match(/^out-src-(\d+)$/)
  const inbTgt = tgt.match(/^inb-tgt-(\d+)$/)
  const inbSrc = src.match(/^inb-src-(\d+)$/)
  const outTgt = tgt.match(/^out-tgt-(\d+)$/)

  if (outSrc && inbTgt) {
    await createRef(Number(outSrc[1]), Number(inbTgt[1]))
  } else if (inbSrc && outTgt) {
    openRuleDialog(Number(inbSrc[1]), Number(outTgt[1]))
  } else {
    ElMessage.warning('仅支持：出站 → 入站（创建引用）或 入站 → 出站（创建路由规则）')
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
      `将出站「${out.tag}」设为引用落地入站「${inb.tag}」（${inbServer}）？\nvnext 地址/端口/UUID/传输参数由主控自动构造，无需手填。`,
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
async function handleEdgeClick(evt: EdgeMouseEvent) {
  const edge = evt.edge
  if (!props.editable || !props.topology) return
  if (edge.id.startsWith('ref-')) {
    const outboundId = Number(edge.id.slice(4))
    const out = props.topology.outbounds.find((o) => o.id === outboundId)
    if (!out) return
    try {
      await ElMessageBox.confirm(
        `解除出站「${out.tag}」的 InboundRef 引用？目标落地入站在无其他引用时将自动标记回「闲置」。`,
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
    >
      <Background pattern-color="#334155" :gap="18" />
      <Controls position="bottom-left" />
      <template #node-serverbox="{ data }: NodeProps<BoxData>">
        <div class="server-box" :class="{ offline: !data.server.status }">
          <div class="sb-head">
            <span class="status-dot" :class="data.server.status === 1 ? 'online' : 'offline'" />
            <span class="name">{{ data.server.name }}</span>
            <span class="host">{{ data.server.host }}</span>
          </div>

          <div class="sb-title">入站（接入点）</div>
          <template v-if="data.inbounds.length > 0">
            <div v-for="(inb, i) in data.inbounds" :key="inb.id" class="sb-row">
              <Handle
                type="target"
                :id="`inb-tgt-${inb.id}`"
                :position="Position.Left"
                :connectable="editable"
                :style="{ top: `${HEADER_H + TITLE_H + i * ROW_H + ROW_H / 2 - 7}px` }"
              />
              <span class="type-tag" :class="typeInfo(inb.type).cls">{{ typeInfo(inb.type).text }}</span>
              <span class="tag">{{ inb.tag }}</span>
              <span class="meta">{{ inb.port }} · {{ transportOf(inb.stream_settings) }}</span>
              <Handle
                type="source"
                :id="`inb-src-${inb.id}`"
                :position="Position.Right"
                :connectable="editable"
                :style="{ top: `${HEADER_H + TITLE_H + i * ROW_H + ROW_H / 2 - 7}px` }"
              />
            </div>
          </template>
          <div v-else class="sb-empty">暂无入站</div>

          <div class="sb-title">出站</div>
          <template v-if="data.outbounds.length > 0">
            <div v-for="(out, i) in data.outbounds" :key="out.id" class="sb-row">
              <Handle
                type="target"
                :id="`out-tgt-${out.id}`"
                :position="Position.Left"
                :connectable="editable"
                :style="{ top: `${HEADER_H + TITLE_H * 2 + data.inbounds.length * ROW_H + i * ROW_H + ROW_H / 2 - 7}px` }"
              />
              <span class="out-tag" :class="out.inbound_ref ? 'ref' : ''">{{ out.tag }}</span>
              <span class="meta">{{ out.protocol }}{{ out.inbound_ref ? ' · 引用' : '' }}</span>
              <Handle
                type="source"
                :id="`out-src-${out.id}`"
                :position="Position.Right"
                :connectable="editable"
                :style="{ top: `${HEADER_H + TITLE_H * 2 + data.inbounds.length * ROW_H + i * ROW_H + ROW_H / 2 - 7}px` }"
              />
            </div>
          </template>
          <div v-else class="sb-empty">暂无出站</div>

          <div class="sb-default">默认出口：{{ data.server.default_outbound_tag || 'direct' }}</div>
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
  width: 340px;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 10px;
  font-size: 12px;
  color: #e2e8f0;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.35);
  overflow: visible;
  &.offline {
    opacity: 0.75;
    border-color: #475569;
  }
  .sb-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    height: 44px;
    border-bottom: 1px solid #334155;
    background: linear-gradient(180deg, #273449 0%, #1e293b 100%);
    border-radius: 10px 10px 0 0;
    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      flex: none;
      &.online {
        background: #34d399;
        box-shadow: 0 0 6px #34d399;
      }
      &.offline {
        background: #64748b;
      }
    }
    .name {
      font-weight: 600;
      font-size: 13px;
      white-space: nowrap;
    }
    .host {
      margin-left: auto;
      color: #94a3b8;
      font-size: 11px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 140px;
    }
  }
  .sb-title {
    padding: 6px 12px 2px;
    color: #94a3b8;
    font-size: 11px;
    letter-spacing: 0.5px;
    font-weight: 600;
  }
  .sb-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 12px;
    height: 30px;
    line-height: 30px;
    border-bottom: 1px dashed #2b3a52;
    .tag {
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 130px;
    }
    .out-tag {
      font-weight: 600;
      color: #93c5fd;
      &.ref {
        color: #fbbf24;
      }
    }
    .meta {
      color: #94a3b8;
      font-size: 11px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .type-tag {
      flex: none;
      font-size: 10.5px;
      padding: 1px 6px;
      border-radius: 4px;
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
    padding: 4px 12px;
    color: #64748b;
    font-size: 11px;
    height: 30px;
    line-height: 30px;
  }
  .sb-default {
    padding: 6px 12px;
    color: #64748b;
    font-size: 11px;
    border-top: 1px solid #334155;
    border-radius: 0 0 10px 10px;
  }
}

/* @vue-flow Handle 深色适配 */
:deep(.vue-flow__handle) {
  width: 10px;
  height: 10px;
  background: #0f172a;
  border: 2px solid #38bdf8;
}
:deep(.vue-flow__edge-path) {
  stroke-width: 2;
}
:deep(.vue-flow__edge-label) {
  background: rgba(15, 23, 42, 0.8);
  color: #94a3b8;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 4px;
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
