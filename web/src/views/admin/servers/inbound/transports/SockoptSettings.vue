<script setup lang="ts">
import {
  CsContainer,
  CsFormItem,
  CsInputNumber,
  CsSelect,
  CsToggle,
} from '@/components/cloudscape'
import type { SockoptConfig } from '../types'

const props = defineProps<{
  modelValue: SockoptConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: SockoptConfig): void
}>()

function updateField<K extends keyof SockoptConfig>(key: K, val: SockoptConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

const congestionOptions = [
  { label: 'bbr (推荐高吞吐/抗丢包)', value: 'bbr' },
  { label: 'cubic (标准 Linux 默认)', value: 'cubic' },
  { label: 'reno (经典算法)', value: 'reno' },
]

const tproxyOptions = [
  { label: '关闭 (off)', value: 'off' },
  { label: 'TProxy 模式 (支持 TCP/UDP)', value: 'tproxy' },
  { label: 'Redirect 模式 (仅 TCP)', value: 'redirect' },
]
</script>

<template>
  <CsContainer header="Socket 底层网络配置 (Sockopt)" class="sockopt-card">
    <template #actions>
      <CsToggle
        :model-value="modelValue.enabled"
        label="启用内核调优"
        @update:model-value="(v: boolean) => updateField('enabled', v)"
      />
    </template>

    <div v-if="modelValue.enabled" class="form-grid-2">
      <!-- TCP 拥塞控制 -->
      <CsFormItem label="TCP 拥塞控制算法 (tcpCongestion)">
        <CsSelect
          :model-value="modelValue.tcp_congestion"
          :options="congestionOptions"
          filterable
          allow-create
          placeholder="例如 bbr / cubic"
          style="width: 100%"
          clearable
          @update:model-value="(v: any) => updateField('tcp_congestion', String(v))"
        />
        <div class="field-hint cs-text-secondary">Linux 建议配置 bbr 获得最佳跨国网络吞吐</div>
      </CsFormItem>

      <!-- TCP Fast Open -->
      <CsFormItem label="TCP 快速打开 (tcpFastOpen)">
        <div class="inline-row">
          <CsToggle
            :model-value="modelValue.tcp_fast_open"
            label="开启 TFO"
            @update:model-value="(v: boolean) => updateField('tcp_fast_open', v)"
          />
          <CsInputNumber
            v-if="modelValue.tcp_fast_open"
            :model-value="modelValue.tcp_fast_open_qlen"
            :min="1"
            :max="2147483647"
            :step="64"
            placeholder="队列上限(默认256)"
            style="width: 160px"
            class="mono"
            @update:model-value="(v: number | null) => updateField('tcp_fast_open_qlen', v ? v : undefined)"
          />
        </div>
        <div class="field-hint cs-text-secondary">减少 1 个 RTT 握手时延；可选指定连接排队队列上限（默认 256）</div>
      </CsFormItem>

      <!-- 多路径 TCP (MPTCP) -->
      <CsFormItem label="多路径 TCP (tcpMptcp)">
        <div class="toggle-pad">
          <CsToggle
            :model-value="modelValue.tcp_mptcp"
            label="启用 MPTCP (需 Linux 5.6+)"
            @update:model-value="(v: boolean) => updateField('tcp_mptcp', v)"
          />
        </div>
        <div class="field-hint cs-text-secondary">允许跨 Wi-Fi 与蜂窝数据多链路聚合传输</div>
      </CsFormItem>

      <!-- 接收 PROXY Protocol -->
      <CsFormItem label="接收 PROXY Protocol">
        <div class="toggle-pad">
          <CsToggle
            :model-value="modelValue.accept_proxy_protocol"
            label="开启 PROXY Protocol 接收"
            @update:model-value="(v: boolean) => updateField('accept_proxy_protocol', v)"
          />
        </div>
        <div class="field-hint cs-text-secondary">前置 HAProxy/Nginx 反向代理时获取真实客户端来源 IP</div>
      </CsFormItem>

      <!-- TCP Keep-Alive 空闲时间 -->
      <CsFormItem label="Keep-Alive 空闲等待 (秒)">
        <CsInputNumber
          :model-value="modelValue.tcp_keep_alive_idle"
          :min="0"
          :max="86400"
          :step="10"
          placeholder="留空为系统默认"
          style="width: 100%"
          class="mono"
          @update:model-value="(v: number | null) => updateField('tcp_keep_alive_idle', v ? v : undefined)"
        />
        <div class="field-hint cs-text-secondary">TCP 连接空闲达此阈值后启动保活探测</div>
      </CsFormItem>

      <!-- TCP Keep-Alive 探测间隔 -->
      <CsFormItem label="Keep-Alive 探测间隔 (秒)">
        <CsInputNumber
          :model-value="modelValue.tcp_keep_alive_interval"
          :min="0"
          :max="86400"
          :step="5"
          placeholder="留空为系统默认"
          style="width: 100%"
          class="mono"
          @update:model-value="(v: number | null) => updateField('tcp_keep_alive_interval', v ? v : undefined)"
        />
        <div class="field-hint cs-text-secondary">发送保活探测报文的周期</div>
      </CsFormItem>

      <!-- TCP User Timeout -->
      <CsFormItem label="TCP 用户超时 (tcpUserTimeout)">
        <CsInputNumber
          :model-value="modelValue.tcp_user_timeout"
          :min="0"
          :max="3600000"
          :step="1000"
          placeholder="单位毫秒，如 10000"
          style="width: 100%"
          class="mono"
          @update:model-value="(v: number | null) => updateField('tcp_user_timeout', v ? v : undefined)"
        />
        <div class="field-hint cs-text-secondary">未确认数据被放弃前的最大超时时间（毫秒）</div>
      </CsFormItem>

      <!-- SO_MARK 标记 -->
      <CsFormItem label="路由网络标记 (SO_MARK)">
        <CsInputNumber
          :model-value="modelValue.mark"
          :min="0"
          :max="2147483647"
          placeholder="SO_MARK 整数"
          style="width: 100%"
          class="mono"
          @update:model-value="(v: number | null) => updateField('mark', v ? v : undefined)"
        />
        <div class="field-hint cs-text-secondary">仅适用于 Linux，配合 iptables / nftables 策略路由使用</div>
      </CsFormItem>

      <!-- 透明代理 tproxy -->
      <CsFormItem label="透明代理模式 (tproxy)">
        <CsSelect
          :model-value="modelValue.tproxy"
          :options="tproxyOptions"
          style="width: 100%"
          @update:model-value="(v: any) => updateField('tproxy', String(v) as SockoptConfig['tproxy'])"
        />
        <div class="field-hint cs-text-secondary">透明代理拦截流量，需内核 CAP_NET_ADMIN 权限</div>
      </CsFormItem>
    </div>
  </CsContainer>
</template>

<style scoped lang="scss">
.sockopt-card {
  margin-top: 16px;
}

.inline-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.toggle-pad {
  padding-top: 4px;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.35;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
