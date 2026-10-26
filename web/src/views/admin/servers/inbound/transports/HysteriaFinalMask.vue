<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsContainer,
  CsExpandableSection,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsSelect,
  CsToggle,
} from '@/components/cloudscape'
import type { HysteriaConfig } from '../types'
import { validateHysteriaFinalMask } from '../validate'
import TipIcon from '@/components/base/TipIcon.vue'

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

const congestionOptions = [
  { label: '自动（按带宽决定）', value: '' },
  { label: 'brutal（需带宽协商）', value: 'brutal' },
  { label: 'force-brutal（强制，不协商）', value: 'force-brutal' },
  { label: 'bbr', value: 'bbr' },
  { label: 'reno', value: 'reno' },
]

const bbrProfileOptions = [
  { label: '自动（standard）', value: '' },
  { label: 'conservative（保守）', value: 'conservative' },
  { label: 'standard（标准）', value: 'standard' },
  { label: 'aggressive（激进）', value: 'aggressive' },
]
</script>

<template>
  <div class="hysteria-finalmask-box">
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
            开启后订阅必须同源下发；写 packetSize 即等价于 mihomo 的 gecko。
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
      <template #info>
        <TipIcon content="服务端声明的是「允许客户端选用的最大 Brutal 速率」；两侧数值需同源下发到订阅，客户端带宽以最小值为准。" />
      </template>
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
    </CsContainer>

    <!-- QUIC 调参（finalmask.quicParams），与带宽同属一个对象 -->
    <CsContainer class="section-gap" :disable-content-paddings="true">
      <CsExpandableSection
        header="QUIC 参数（高级，finalmask.quicParams）"
        header-description="仅作用于本机服务端 QUIC 栈，留空使用 xray 默认值"
        variant="container"
      >
        <div class="form-grid">
          <CsFormItem label="拥塞控制 (congestion)">
            <CsSelect
              :model-value="modelValue.congestion"
              :options="congestionOptions"
              style="width: 100%"
              @update:model-value="(v: any) => updateField('congestion', String(v ?? '') as HysteriaConfig['congestion'])"
            />
            <div class="field-hint cs-text-secondary">
              自动：上下行带宽都填了走 brutal，否则 BBR；force-brutal 需要上行带宽且不与客户端协商。
            </div>
          </CsFormItem>

          <CsFormItem label="BBR 档位 (bbrProfile)">
            <CsSelect
              :model-value="modelValue.bbr_profile"
              :options="bbrProfileOptions"
              style="width: 100%"
              @update:model-value="(v: any) => updateField('bbr_profile', String(v ?? '') as HysteriaConfig['bbr_profile'])"
            />
            <div class="field-hint cs-text-secondary">仅在拥塞控制为 bbr（或自动回落到 BBR）时生效。</div>
          </CsFormItem>

          <CsFormItem label="最大空闲超时 (maxIdleTimeout，秒)">
            <CsInputNumber
              :model-value="modelValue.max_idle_timeout"
              :min="4"
              :max="120"
              :step="1"
              placeholder="默认 30"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('max_idle_timeout', v == null ? undefined : v)"
            />
          </CsFormItem>

          <CsFormItem label="对端入流上限 (maxIncomingStreams)">
            <CsInputNumber
              :model-value="modelValue.max_incoming_streams"
              :min="8"
              :step="128"
              placeholder="默认 1024"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('max_incoming_streams', v == null ? undefined : v)"
            />
          </CsFormItem>

          <CsFormItem label="初始流接收窗口 (initStreamReceiveWindow)">
            <CsInputNumber
              :model-value="modelValue.init_stream_receive_window"
              :min="16384"
              :step="1048576"
              placeholder="默认 8388608"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('init_stream_receive_window', v == null ? undefined : v)"
            />
          </CsFormItem>

          <CsFormItem label="最大流接收窗口 (maxStreamReceiveWindow)">
            <CsInputNumber
              :model-value="modelValue.max_stream_receive_window"
              :min="16384"
              :step="1048576"
              placeholder="默认 8388608"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('max_stream_receive_window', v == null ? undefined : v)"
            />
          </CsFormItem>

          <CsFormItem label="初始连接接收窗口 (initConnectionReceiveWindow)">
            <CsInputNumber
              :model-value="modelValue.init_connection_receive_window"
              :min="16384"
              :step="1048576"
              placeholder="默认 20971520"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('init_connection_receive_window', v == null ? undefined : v)"
            />
          </CsFormItem>

          <CsFormItem label="最大连接接收窗口 (maxConnectionReceiveWindow)">
            <CsInputNumber
              :model-value="modelValue.max_connection_receive_window"
              :min="16384"
              :step="1048576"
              placeholder="默认 20971520"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateField('max_connection_receive_window', v == null ? undefined : v)"
            />
          </CsFormItem>
        </div>

        <div class="toggle-grid">
          <CsToggle
            :model-value="modelValue.disable_path_mtu_discovery"
            label="禁用路径 MTU 探测"
            @update:model-value="(v: boolean) => updateField('disable_path_mtu_discovery', v)"
          />
          <CsToggle
            :model-value="modelValue.brutal_disable_loss_compensation"
            label="关闭 Brutal 丢包补偿"
            @update:model-value="(v: boolean) => updateField('brutal_disable_loss_compensation', v)"
          />
          <CsToggle
            :model-value="modelValue.disable_gso"
            label="关闭 UDP GSO"
            @update:model-value="(v: boolean) => updateField('disable_gso', v)"
          />
          <CsToggle
            :model-value="modelValue.disable_stateless_reset"
            label="关闭 QUIC Stateless Reset"
            @update:model-value="(v: boolean) => updateField('disable_stateless_reset', v)"
          />
        </div>
      </CsExpandableSection>
    </CsContainer>

    <!-- 端口跳跃（finalmask.udp[type=udphop]，仅客户端消费） -->
    <CsContainer header="端口跳跃 (finalmask.udp[type=udphop]，仅下发客户端)" class="section-gap">
      <template #info>
        <TipIcon content="仅客户端消费，生成服务端配置时自动剥离；需先在服务器防火墙 / NAT 把端口段转发到本入站监听端口。" />
      </template>
      <div class="form-grid">
        <CsFormItem label="跳跃方式 (mode)">
          <CsSelect
            :model-value="modelValue.port_hop_mode"
            :options="portHopModeOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('port_hop_mode', String(v) as HysteriaConfig['port_hop_mode'])"
          />
          <div class="field-hint cs-text-secondary">映射到 mihomo 的 ports；intervalLocal（只跳源端口）面板不提供</div>
        </CsFormItem>

        <CsFormItem label="目标端口集合">
          <CsInput
            :model-value="modelValue.port_hop_ports"
            placeholder="如 20000-30000，留空不启用"
            class="mono"
            @update:model-value="(v: string) => updateField('port_hop_ports', v)"
          />
          <div class="field-hint cs-text-secondary">支持 20000-30000 或 20000-30000,40000；节点仍只监听配置端口</div>
        </CsFormItem>

        <CsFormItem v-if="modelValue.port_hop_mode === 'intervalRemote'" label="跳跃间隔 (秒)">
          <CsInput
            :model-value="modelValue.port_hop_interval"
            placeholder="默认 30，支持如 10-20"
            class="mono"
            @update:model-value="(v: string) => updateField('port_hop_interval', v)"
          />
          <div class="field-hint cs-text-secondary">最小 5 秒，支持 10-20 区间写法</div>
        </CsFormItem>
      </div>
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

.toggle-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 18px;
  margin-top: 4px;
}

@media (max-width: 640px) {
  .form-grid,
  .toggle-grid {
    grid-template-columns: 1fr;
  }
}
</style>
