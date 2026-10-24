<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  CsAlert,
  CsContainer,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsSegmentedControl,
  CsSelect,
  CsToggle,
} from '@/components/cloudscape'
import { getInbounds, getServers, type InboundItem } from '@/api/admin'
import TipIcon from '@/components/base/TipIcon.vue'
import type { DokodemoConfig } from '../types'

const props = defineProps<{
  modelValue: DokodemoConfig
  currentInboundId?: number
  /** 当前入站所属服务器：目标候选默认只列本机（同服务器）的用户入站。 */
  serverId?: number
  listenPort?: number
  listenAddress?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: DokodemoConfig): void
}>()

const candidateInbounds = ref<InboundItem[]>([])
const serverMap = ref<Record<number, string>>({})

const targetModeOptions = [
  { id: 'inbound', text: '指定目标入站' },
  { id: 'manual', text: '手动指定远程目标' },
  { id: 'draft', text: '草稿留空' },
]

const forwardNetworkOptions = [
  { label: 'TCP + UDP', value: 'tcp,udp' },
  { label: '仅 TCP', value: 'tcp' },
  { label: '仅 UDP', value: 'udp' },
]

// 目标候选默认只列「同服务器 + type=user」的入站（最常见的落地形态）。
// 跨服务器落地与 tunnel 链式目标都是后端真实支持的路径（TunnelTarget 用目标服务器 Host），
// 默认隐藏、需要显式打开开关，避免把一台机器上的全部入站塞进下拉。
const showAllTargets = ref(false)

function isUserInbound(item: InboundItem) {
  // 历史行 type 可能为空，按 user 处理（与列表页口径一致）。
  return (item.type || 'user') === 'user'
}

const allCandidates = computed(() =>
  candidateInbounds.value.filter((item) => isUserInbound(item) || item.type === 'tunnel'),
)

const defaultCandidates = computed(() => {
  // 服务器未知（新建入站尚未选服务器）时无法按同服务器过滤，退回全部合法候选。
  if (!props.serverId) return allCandidates.value
  return allCandidates.value.filter((item) => item.server_id === props.serverId && isUserInbound(item))
})

const restrictedCandidates = computed(() => {
  const defaults = new Set(defaultCandidates.value.map((item) => item.id))
  return allCandidates.value.filter((item) => !defaults.has(item.id))
})

const visibleCandidates = computed(() =>
  showAllTargets.value ? allCandidates.value : defaultCandidates.value,
)

const targetInboundOptions = computed(() =>
  visibleCandidates.value.map((item) => ({
    value: item.id,
    label: `[${serverMap.value[item.server_id] || '服务器#' + item.server_id}] ${item.tag} (:${item.port} ${item.protocol})`,
  }))
)

async function loadCandidateInbounds() {
  try {
    const [inbRes, srvRes] = await Promise.all([getInbounds(), getServers()])
    if (srvRes.data.code === 0) {
      const sm: Record<number, string> = {}
      for (const s of srvRes.data.data.items) {
        sm[s.id] = s.name
      }
      serverMap.value = sm
    }
    if (inbRes.data.code === 0) {
      // 目标只能是能承接流量的入站：排除自身与已退役的 forward（旧代理通道）行。
      candidateInbounds.value = inbRes.data.data.items.filter(
        (i) => i.id !== props.currentInboundId && i.type !== 'forward',
      )
      // 存量目标若落在默认范围外（跨服务器 / tunnel），自动展开，否则下拉回显不出当前值。
      const current = props.modelValue.target_inbound_id
      if (current && !defaultCandidates.value.some((i) => i.id === current)) {
        showAllTargets.value = true
      }
    }
  } catch {
    /* ignore */
  }
}

onMounted(loadCandidateInbounds)

function updateField<K extends keyof DokodemoConfig>(key: K, val: DokodemoConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function onModeChange(detail: { selectedId: string | number }) {
  const m = String(detail?.selectedId || 'draft') as 'inbound' | 'manual' | 'draft'
  if (m === 'inbound') {
    emit('update:modelValue', {
      ...props.modelValue,
      target_mode: m,
      target_address: '',
      target_port: 0,
    })
  } else if (m === 'manual') {
    emit('update:modelValue', {
      ...props.modelValue,
      target_mode: m,
      target_inbound_id: undefined,
    })
  } else {
    emit('update:modelValue', {
      ...props.modelValue,
      target_mode: m,
      target_inbound_id: undefined,
      target_address: '',
      target_port: 0,
    })
  }
}

const targetNeedsPPWarning = computed(() => {
  if (props.modelValue.target_mode !== 'inbound' || !props.modelValue.target_inbound_id) return false
  if (props.modelValue.proxy_protocol === false) return false
  const target = candidateInbounds.value.find((i) => i.id === props.modelValue.target_inbound_id)
  if (!target) return false
  try {
    const ss = JSON.parse(target.stream_settings || '{}')
    const ap = ss.acceptProxyProtocol === true || ss.tcpSettings?.acceptProxyProtocol === true
    return !ap
  } catch {
    return false
  }
})

const targetSummaryText = computed(() => {
  if (props.modelValue.target_mode === 'inbound') {
    if (!props.modelValue.target_inbound_id) return '未选入站'
    const target = candidateInbounds.value.find((i) => i.id === props.modelValue.target_inbound_id)
    if (!target) return `入站 #${props.modelValue.target_inbound_id}`
    const sName = serverMap.value[target.server_id] || `#${target.server_id}`
    return `${sName}/${target.tag} (:${target.port})`
  }
  if (props.modelValue.target_mode === 'manual') {
    if (!props.modelValue.target_address && !props.modelValue.target_port) return '未指定目标'
    return `${props.modelValue.target_address || '—'}:${props.modelValue.target_port || '—'}`
  }
  return '待连线绑定 (草稿)'
})
</script>

<template>
  <div class="dokodemo-settings-box">
    <!-- 四层直通管道白盒化流向展示 -->
    <div class="pipeline-banner">
      <div class="pipeline-flow">
        <div class="pipeline-node inbound">
          <span class="node-tag">入站监听</span>
          <span class="node-val mono" :title="`监听 ${listenAddress || '0.0.0.0'}:${listenPort || 443}`">
            {{ listenAddress || '0.0.0.0' }}:{{ listenPort || 443 }}
          </span>
        </div>
        <span class="pipeline-arrow">➔</span>
        <div class="pipeline-node rule">
          <span class="node-tag">直达分流</span>
          <span class="node-val">Tag 独占通道</span>
        </div>
        <span class="pipeline-arrow">➔</span>
        <div
          class="pipeline-node outbound"
          :class="{ empty: modelValue.target_mode === 'draft' || (!modelValue.target_inbound_id && !modelValue.target_address) }"
        >
          <span class="node-tag">目标出口</span>
          <span class="node-val mono" :title="targetSummaryText">{{ targetSummaryText }}</span>
        </div>
      </div>
      <div class="pipeline-hint">
        四层直通管道：入站监听本地流量，出站通过专属路由通道转发至目标。
      </div>
    </div>

    <!-- 微卡片 1: 转发目标定义 -->
    <CsContainer header="转发目标出口 (Target Destination)">
      <template #actions>
        <CsSegmentedControl
          :model-value="modelValue.target_mode"
          :options="targetModeOptions"
          @change="onModeChange"
        />
      </template>
      <!-- 模式一：指定入站 -->
      <div v-if="modelValue.target_mode === 'inbound'" class="form-grid-2">
        <CsFormItem label="直通目标入站" style="grid-column: 1 / -1">
          <CsSelect
            :model-value="modelValue.target_inbound_id || undefined"
            :options="targetInboundOptions"
            filterable
            placeholder="选择要直通的目标入站"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('target_inbound_id', Number(v || 0))"
          />
          <div v-if="!targetInboundOptions.length" class="field-hint cs-text-secondary">
            本服务器暂无可用目标入站；跨服务器目标需要先打开下方开关。
          </div>
        </CsFormItem>
        <CsFormItem
          v-if="restrictedCandidates.length"
          label="候选范围"
          style="grid-column: 1 / -1"
        >
          <div class="toggle-pad">
            <CsToggle
              :model-value="showAllTargets"
              :label="`显示跨服务器 / 链式目标（另有 ${restrictedCandidates.length} 个）`"
              @update:model-value="(v: boolean) => (showAllTargets = v)"
            />
          </div>
          <div class="field-hint cs-text-secondary">
            默认只列同服务器的用户入站；跨服务器落地与 tunnel 链式目标是真实支持的路径，打开后一并显示。
          </div>
        </CsFormItem>
        <div v-if="targetNeedsPPWarning" style="grid-column: 1 / -1">
          <CsAlert type="warning" header="所选目标入站未启用 Proxy Protocol，无法透传真实客户端 IP。" />
        </div>
      </div>

      <!-- 模式二：手动指定 -->
      <div v-else-if="modelValue.target_mode === 'manual'" class="form-grid-2">
        <CsFormItem label="转发目标地址 (Target Address)">
          <CsInput
            :model-value="modelValue.target_address"
            placeholder="远程 IP 或域名，如 1.2.3.4"
            class="mono"
            @update:model-value="(v: string) => updateField('target_address', v)"
          />
        </CsFormItem>
        <CsFormItem label="转发目标端口 (Target Port)">
          <CsInputNumber
            :model-value="modelValue.target_port"
            :min="1"
            :max="65535"
            style="width: 100%"
            class="mono"
            @update:model-value="(v: number | null) => updateField('target_port', Number(v || 0))"
          />
        </CsFormItem>
      </div>

      <!-- 模式三：草稿留空 -->
      <div v-else>
        <CsAlert type="info" header="草稿状态：未指定目标地址与端口">
          可在拓扑画布中连线绑定目标入站。
        </CsAlert>
      </div>
    </CsContainer>

    <!-- 微卡片 2: 传输协议与 Proxy Protocol -->
    <CsContainer header="四层传输协议与透传策略" class="section-gap">
      <template #actions>
        <span class="cs-text-secondary">直通网络层</span>
      </template>
      <div class="form-grid-2">
        <CsFormItem label="转发传输协议">
          <CsSelect
            :model-value="modelValue.network || 'tcp,udp'"
            :options="forwardNetworkOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('network', String(v))"
          />
        </CsFormItem>

        <CsFormItem>
          <template #label>
            <span class="label-with-tip">
              透传真实客户端 IP (Proxy Protocol v2)
              <TipIcon content="开启后向落地端发送 PROXY Protocol v2 报文。落地端须开启「接收 Proxy Protocol」；若落地为普通外部 Web 服务请关闭此项。" />
            </span>
          </template>
          <div class="toggle-pad">
            <CsToggle
              :model-value="modelValue.proxy_protocol !== false"
              label="发送 PPv2 报文"
              @update:model-value="(v: boolean) => updateField('proxy_protocol', v)"
            />
          </div>
        </CsFormItem>
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.dokodemo-settings-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.toggle-pad {
  padding-top: 4px;
}

.label-with-tip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.pipeline-banner {
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-card-btvpmf, #e9ebed);
  border-radius: 8px;
  padding: 12px 14px;
  margin-bottom: 14px;
}

.pipeline-flow {
  display: flex;
  align-items: center;
  gap: 8px;
  justify-content: space-between;
}

.pipeline-node {
  flex: 1;
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-card-btvpmf, #e9ebed);
  border-radius: 6px;
  padding: 6px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;

  .node-tag {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-text-body-secondary-66ygbb, #656871);
    text-transform: uppercase;
  }
  .node-val {
    font-size: 11.5px;
    font-weight: 500;
    color: var(--color-text-body-default-b2us04, #0f141a);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  &.inbound {
    border-left: 3px solid #0284c7;
  }
  &.rule {
    border-left: 3px solid #8b5cf6;
  }
  &.outbound {
    border-left: 3px solid #10b981;
    &.empty {
      border-left-color: #f59e0b;
      .node-val {
        color: #f59e0b;
      }
    }
  }
}

.pipeline-arrow {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: 12px;
  flex-shrink: 0;
}

.pipeline-hint {
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-top: 8px;
  line-height: 1.4;
}

@media (max-width: 640px) {
  .pipeline-flow {
    flex-direction: column;
    align-items: stretch;
  }
  .pipeline-arrow {
    text-align: center;
    transform: rotate(90deg);
  }
}
</style>
