<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsContainer,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsSelect,
} from '@/components/cloudscape'
import type { HysteriaConfig } from '../types'
import { validateHysteriaFinalMask } from '../validate'

const props = defineProps<{
  modelValue: HysteriaConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: HysteriaConfig): void
}>()

function updateField<K extends keyof HysteriaConfig>(key: K, val: HysteriaConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

const errors = computed(() => validateHysteriaFinalMask(props.modelValue))

const obfsOptions = [
  { label: '不启用', value: 'none' },
  { label: 'salamander（固定长度填充）', value: 'salamander' },
  { label: 'gecko（随机长度分片）', value: 'gecko' },
]

const portHopModeOptions = [
  { label: 'intervalRemote（定时换目标端口）', value: 'intervalRemote' },
  { label: 'perConnRemote（每个连接随机端口）', value: 'perConnRemote' },
]
</script>

<template>
  <div class="hysteria-finalmask-box">
    <CsAlert type="info">
      finalmask 不属于 hysteria 协议本身：它在协议组装与 TLS 加密完成后作用于外层字节 /
      数据包。salamander 与 gecko 是外层混淆，quicParams 是 QUIC 调参，udphop 只改客户端目标端口。
    </CsAlert>

    <!-- 混淆（finalmask.udp[type=salamander]） -->
    <CsContainer header="混淆 (finalmask.udp[type=salamander])" class="section-gap">
      <div class="form-grid">
        <CsFormItem label="混淆方式 (obfs)">
          <CsSelect
            :model-value="modelValue.obfs_type"
            :options="obfsOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('obfs_type', String(v) as HysteriaConfig['obfs_type'])"
          />
          <div class="field-hint cs-text-secondary">
            xray 服务端写 salamander + packetSize 即等价于 mihomo 的 gecko；开启后订阅必须同源下发，否则客户端连不上。
          </div>
        </CsFormItem>

        <CsFormItem v-if="modelValue.obfs_type !== 'none'" label="混淆密码 (obfs-password)">
          <CsInput
            :model-value="modelValue.obfs_password"
            placeholder="必填，客户端需完全一致"
            class="mono"
            @update:model-value="(v: string) => updateField('obfs_password', v)"
          />
        </CsFormItem>

        <template v-if="modelValue.obfs_type === 'gecko'">
          <CsFormItem label="最小分片字节">
            <CsInputNumber
              :model-value="modelValue.obfs_min_packet"
              :min="1"
              :max="2048"
              :step="64"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('obfs_min_packet', Number(v) || 512)"
            />
          </CsFormItem>

          <CsFormItem label="最大分片字节">
            <CsInputNumber
              :model-value="modelValue.obfs_max_packet"
              :min="1"
              :max="2048"
              :step="64"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('obfs_max_packet', Number(v) || 1200)"
            />
            <div class="field-hint cs-text-secondary">默认 512–1200，上限 2048；最小不得大于最大</div>
          </CsFormItem>
        </template>
      </div>
    </CsContainer>

    <!-- 带宽（finalmask.quicParams.brutal*） -->
    <CsContainer header="带宽限制 (finalmask.quicParams.brutal)" class="section-gap">
      <div class="form-grid">
        <CsFormItem label="服务端上行 (brutalUp)">
          <CsInput
            :model-value="modelValue.bw_up"
            placeholder="如 100 mbps，留空不限制"
            class="mono"
            @update:model-value="(v: string) => updateField('bw_up', v)"
          />
          <div class="field-hint cs-text-secondary">服务端发出方向 = 客户端下行；单位支持 k/m/g/t 与 mbps/gbps</div>
        </CsFormItem>

        <CsFormItem label="服务端下行 (brutalDown)">
          <CsInput
            :model-value="modelValue.bw_down"
            placeholder="如 100 mbps，留空不限制"
            class="mono"
            @update:model-value="(v: string) => updateField('bw_down', v)"
          />
          <div class="field-hint cs-text-secondary">服务端接收方向 = 客户端上行；上下行都填写才会启用 Brutal，否则退回 BBR</div>
        </CsFormItem>
      </div>
      <CsAlert type="info" class="notice-gap">
        服务端声明的是「允许客户端选用的最大 Brutal 速率」；两侧数值需同源下发到订阅，客户端带宽以最小值为准。
      </CsAlert>
    </CsContainer>

    <!-- 端口跳跃（finalmask.udp[type=udphop]，仅客户端消费） -->
    <CsContainer header="端口跳跃 (finalmask.udp[type=udphop]，仅下发客户端)" class="section-gap">
      <div class="form-grid">
        <CsFormItem label="跳跃方式 (mode)">
          <CsSelect
            :model-value="modelValue.port_hop_mode"
            :options="portHopModeOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('port_hop_mode', String(v) as HysteriaConfig['port_hop_mode'])"
          />
          <div class="field-hint cs-text-secondary">两种都能映射到 mihomo 的 ports；intervalLocal 只跳源端口，面板不提供</div>
        </CsFormItem>

        <CsFormItem label="目标端口集合">
          <CsInput
            :model-value="modelValue.port_hop_ports"
            placeholder="如 20000-30000，留空不启用"
            class="mono"
            @update:model-value="(v: string) => updateField('port_hop_ports', v)"
          />
          <div class="field-hint cs-text-secondary">支持 "20000-30000" 或 "20000-30000,40000"；节点本身仍只监听配置的端口</div>
        </CsFormItem>

        <CsFormItem v-if="modelValue.port_hop_mode === 'intervalRemote'" label="跳跃间隔 (秒)">
          <CsInput
            :model-value="modelValue.port_hop_interval"
            placeholder="默认 30，支持如 10-20"
            class="mono"
            @update:model-value="(v: string) => updateField('port_hop_interval', v)"
          />
          <div class="field-hint cs-text-secondary">最小 5 秒；mihomo 与 xray 客户端都接受区间写法</div>
        </CsFormItem>
      </div>
      <CsAlert type="info" class="notice-gap">
        端口跳跃只改变客户端发往的目标端口，xray 服务端不支持 udphop（写入节点配置会导致监听失败）。
        面板会把它从生成的服务端配置中剥离，仅保留给订阅与机间出站。
        请先在服务器防火墙 / NAT 把该端口段转发到本入站监听端口。
      </CsAlert>
    </CsContainer>

    <CsAlert v-if="errors.length" type="error" class="errors-gap">
      <div v-for="e in errors" :key="e">{{ e }}</div>
    </CsAlert>
  </div>
</template>

<style scoped lang="scss">
.hysteria-finalmask-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.notice-gap {
  margin-top: 4px;
}

.errors-gap {
  margin-top: 10px;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.4;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
