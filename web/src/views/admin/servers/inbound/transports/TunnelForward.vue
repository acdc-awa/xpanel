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
import { protocolDisplayName } from '@/utils/protocol'
import type { TunnelConfig } from '../types'

const props = defineProps<{
  modelValue: TunnelConfig
  currentInboundId?: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TunnelConfig): void
}>()

const candidateInbounds = ref<InboundItem[]>([])
const serverMap = ref<Record<number, string>>({})

// 草稿不是独立的模式：引用入站但未选择目标即草稿，画布上连接落地入站后自动补全。
const targetModeOptions = [
  { id: 'inbound', text: '引用入站' },
  { id: 'manual', text: '自定义目标' },
]

const forwardNetworkOptions = [
  { id: 'tcp,udp', text: 'TCP + UDP' },
  { id: 'tcp', text: '仅 TCP' },
  { id: 'udp', text: '仅 UDP' },
]

// 目标候选：全部能承接流量的入站（排除自身与已退役的 forward 行），
// 跨服务器目标在标签里带服务器名，不再用开关分层展示。
function isUserInbound(item: InboundItem) {
  // 历史行 type 可能为空，按 user 处理（与列表页口径一致）。
  return (item.type || 'user') === 'user'
}

const allCandidates = computed(() =>
  candidateInbounds.value.filter((item) => isUserInbound(item) || item.type === 'tunnel'),
)

const targetInboundOptions = computed(() =>
  allCandidates.value.map((item) => ({
    value: item.id,
    label: `[${serverMap.value[item.server_id] || '服务器#' + item.server_id}] ${item.tag} (:${item.port} ${protocolDisplayName(item.protocol)})`,
  })),
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
    }
  } catch {
    /* ignore */
  }
}

onMounted(loadCandidateInbounds)

function updateField<K extends keyof TunnelConfig>(key: K, val: TunnelConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function onModeChange(detail: { selectedId: string | number }) {
  const m = String(detail?.selectedId || 'inbound') as 'inbound' | 'manual'
  if (m === 'inbound') {
    emit('update:modelValue', {
      ...props.modelValue,
      target_mode: m,
      target_address: '',
      target_port: 0,
    })
  } else {
    emit('update:modelValue', {
      ...props.modelValue,
      target_mode: m,
      target_inbound_id: undefined,
    })
  }
}

function onNetworkChange(detail: { selectedId: string | number }) {
  updateField('network', String(detail?.selectedId || 'tcp,udp'))
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

const referenceHint = computed(() => {
  if (!allCandidates.value.length) {
    return '暂无可用目标入站；也可以留空保存为草稿，稍后在拓扑画布连线绑定。'
  }
  if (!props.modelValue.target_inbound_id) {
    return '未选择目标时按草稿保存，可在拓扑画布上把 out-<tag> 连到落地入站。'
  }
  return ''
})
</script>

<template>
  <div class="tunnel-forward">
    <CsContainer header="出口设置">
      <template #info>
        <TipIcon content="引用入站时，目标由派生的 freedom 出站（redirect）承担；自定义目标直接写目标地址与端口。未选择目标时按草稿保存。" />
      </template>
      <template #actions>
        <CsSegmentedControl
          :model-value="modelValue.target_mode"
          :options="targetModeOptions"
          @change="onModeChange"
        />
      </template>

      <div v-if="modelValue.target_mode === 'inbound'" class="form-grid-2">
        <CsFormItem
          label="目标入站"
          style="grid-column: 1 / -1"
          :constraint-text="referenceHint"
        >
          <CsSelect
            :model-value="modelValue.target_inbound_id || undefined"
            :options="targetInboundOptions"
            filterable
            placeholder="选择要直通的目标入站"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('target_inbound_id', Number(v || 0))"
          />
        </CsFormItem>

        <div v-if="targetNeedsPPWarning" style="grid-column: 1 / -1">
          <CsAlert type="warning" header="所选目标入站未启用 Proxy Protocol，无法透传真实客户端 IP。" />
        </div>
      </div>

      <div v-else class="form-grid-2">
        <CsFormItem label="目标地址">
          <CsInput
            :model-value="modelValue.target_address"
            placeholder="远程 IP 或域名，如 1.2.3.4"
            class="mono"
            @update:model-value="(v: string) => updateField('target_address', v)"
          />
        </CsFormItem>
        <CsFormItem label="目标端口">
          <CsInputNumber
            :model-value="modelValue.target_port || undefined"
            :min="1"
            :max="65535"
            placeholder="1-65535"
            style="width: 100%"
            class="mono"
            @update:model-value="(v: number | null) => updateField('target_port', Number(v || 0))"
          />
        </CsFormItem>
      </div>
    </CsContainer>

    <CsContainer header="转发与透传" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="转发网络" style="grid-column: 1 / -1">
          <template #info>
            <TipIcon content="写入 settings.network：入站监听侧接受的转发网络（TCP / UDP / 两者）。" />
          </template>
          <CsSegmentedControl
            :model-value="modelValue.network || 'tcp,udp'"
            :options="forwardNetworkOptions"
            @change="onNetworkChange"
          />
        </CsFormItem>

        <CsFormItem label="透传真实客户端 IP" style="grid-column: 1 / -1">
          <template #info>
            <TipIcon content="开启后由派生的 freedom 出站向目标发送 PROXY Protocol v2 报文。目标为面板内入站时，其接收侧会随本开关自动同步；目标为外部地址时需在目标侧自行开启接收。该值是面板私有键 streamSettings.proxy_protocol，不会写进节点配置的 xray 字段。" />
          </template>
          <CsToggle
            :model-value="modelValue.proxy_protocol !== false"
            label="向目标发送 PROXY Protocol v2 报文"
            @update:model-value="(v: boolean) => updateField('proxy_protocol', v)"
          />
        </CsFormItem>
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.tunnel-forward {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
