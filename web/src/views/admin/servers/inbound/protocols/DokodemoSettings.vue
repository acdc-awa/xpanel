<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getInbounds, getServers, type InboundItem } from '@/api/admin'
import TipIcon from '@/components/base/TipIcon.vue'
import type { DokodemoConfig } from '../types'

const props = defineProps<{
  modelValue: DokodemoConfig
  currentInboundId?: number
  listenPort?: number
  listenAddress?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: DokodemoConfig): void
}>()

const candidateInbounds = ref<InboundItem[]>([])
const serverMap = ref<Record<number, string>>({})

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
      candidateInbounds.value = inbRes.data.data.items.filter((i) => i.id !== props.currentInboundId)
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

function onModeChange(mode: any) {
  const m = String(mode) as 'inbound' | 'manual' | 'draft'
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
    const ap = ss.acceptProxyProtocol === true || ss.tcpSettings?.acceptProxyProtocol === true || ss.xhttpSettings?.acceptProxyProtocol === true
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
          <span class="node-val font-mono-num" :title="`监听 ${listenAddress || '0.0.0.0'}:${listenPort || 443}`">
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
          <span class="node-val font-mono-num" :title="targetSummaryText">{{ targetSummaryText }}</span>
        </div>
      </div>
      <div class="pipeline-hint">
        四层直通管道：入站监听本地流量，出站通过专属路由通道转发至目标。
      </div>
    </div>

    <!-- 微卡片 1: 转发目标定义 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
          <span>转发目标出口 (Target Destination)</span>
        </div>
        <div class="head-meta">
          <el-radio-group :model-value="modelValue.target_mode" size="small" @change="onModeChange">
            <el-radio-button value="inbound">指定目标入站</el-radio-button>
            <el-radio-button value="manual">手动指定远程目标</el-radio-button>
            <el-radio-button value="draft">草稿留空</el-radio-button>
          </el-radio-group>
        </div>
      </div>
      <div class="x-micro-card__body">
        <!-- 模式一：指定入站 -->
        <div v-if="modelValue.target_mode === 'inbound'" class="form-grid-2">
          <el-form-item label="直通目标入站" style="grid-column: 1 / -1">
            <el-select
              :model-value="modelValue.target_inbound_id || undefined"
              filterable
              placeholder="选择要直通的目标入站"
              style="width: 100%"
              @update:model-value="(v: any) => updateField('target_inbound_id', Number(v || 0))"
            >
              <el-option
                v-for="item in candidateInbounds"
                :key="item.id"
                :label="`[${serverMap[item.server_id] || '服务器#' + item.server_id}] ${item.tag} (:${item.port} ${item.protocol})`"
                :value="item.id"
              />
            </el-select>
          </el-form-item>
          <div v-if="targetNeedsPPWarning" style="grid-column: 1 / -1">
            <el-alert
              type="warning"
              :closable="false"
              show-icon
              title="所选目标入站未启用 Proxy Protocol，无法透传真实客户端 IP。"
            />
          </div>
        </div>

        <!-- 模式二：手动指定 -->
        <div v-else-if="modelValue.target_mode === 'manual'" class="form-grid-2">
          <el-form-item label="转发目标地址 (Target Address)">
            <el-input
              :model-value="modelValue.target_address"
              placeholder="远程 IP 或域名，如 1.2.3.4"
              class="font-mono-num"
              @input="(v: string) => updateField('target_address', v)"
            />
          </el-form-item>
          <el-form-item label="转发目标端口 (Target Port)">
            <el-input-number
              :model-value="modelValue.target_port"
              :min="1"
              :max="65535"
              style="width: 100%"
              class="font-mono-num"
              @update:model-value="(v: any) => updateField('target_port', Number(v || 0))"
            />
          </el-form-item>
        </div>

        <!-- 模式三：草稿留空 -->
        <div v-else class="draft-tip-box">
          <el-alert
            type="info"
            :closable="false"
            show-icon
            title="草稿状态：未指定目标地址与端口"
            description="可在拓扑画布中连线绑定目标入站。"
          />
        </div>
      </div>
    </div>

    <!-- 微卡片 2: 传输协议与 Proxy Protocol -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span>四层传输协议与透传策略</span>
        </div>
        <div class="head-meta">
          <span>直通网络层</span>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="转发传输协议">
            <el-select
              :model-value="modelValue.network || 'tcp,udp'"
              style="width: 100%"
              @update:model-value="(v: string) => updateField('network', v)"
            >
              <el-option label="TCP + UDP" value="tcp,udp" />
              <el-option label="仅 TCP" value="tcp" />
              <el-option label="仅 UDP" value="udp" />
            </el-select>
          </el-form-item>

          <el-form-item>
            <template #label>
              <span style="display: inline-flex; align-items: center; gap: 4px">
                透传真实客户端 IP (Proxy Protocol v2)
                <TipIcon content="开启后向落地端发送 PROXY Protocol v2 报文。落地端须开启「接收 Proxy Protocol」；若落地为普通外部 Web 服务请关闭此项。" />
              </span>
            </template>
            <div style="padding-top: 4px">
              <el-switch
                :model-value="modelValue.proxy_protocol !== false"
                active-text="发送 PPv2 报文"
                @update:model-value="(v: any) => updateField('proxy_protocol', Boolean(v))"
              />
            </div>
          </el-form-item>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.dokodemo-settings-box {
  display: flex;
  flex-direction: column;
}

:deep(.el-form-item) {
  margin-bottom: 0;
}

.pipeline-banner {
  background: var(--x-card-soft, #f8fafc);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: var(--x-radius, 8px);
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
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 6px;
  padding: 6px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;

  .node-tag {
    font-size: 10px;
    font-weight: 600;
    color: var(--x-text-3, #94a3b8);
    text-transform: uppercase;
  }
  .node-val {
    font-size: 11.5px;
    font-weight: 500;
    color: var(--x-text, #1e293b);
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
  color: var(--x-text-3, #94a3b8);
  font-size: 12px;
  flex-shrink: 0;
}

.pipeline-hint {
  font-size: 11px;
  color: var(--x-text-3, #64748b);
  margin-top: 8px;
  line-height: 1.4;
}

.draft-tip-box {
  margin-bottom: 0;
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
