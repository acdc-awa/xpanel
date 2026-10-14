<script setup lang="ts">
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
</script>

<template>
  <div class="x-micro-card">
    <div class="x-micro-card__head">
      <div class="head-title">
        <span class="w-2 h-2 rounded-full bg-cyan-500"></span>
        <span>Socket 底层网络配置 (Sockopt)</span>
      </div>
      <div class="head-meta">
        <el-switch
          :model-value="modelValue.enabled"
          active-text="启用内核调优"
          size="small"
          @update:model-value="(v: any) => updateField('enabled', Boolean(v))"
        />
      </div>
    </div>

    <div v-if="modelValue.enabled" class="x-micro-card__body">
      <div class="form-grid-2">
        <!-- TCP 拥塞控制 -->
        <el-form-item label="TCP 拥塞控制算法 (tcpCongestion)">
          <el-select
            :model-value="modelValue.tcp_congestion"
            filterable
            allow-create
            default-first-option
            placeholder="例如 bbr / cubic"
            style="width: 100%"
            clearable
            @update:model-value="(v: string) => updateField('tcp_congestion', v)"
          >
            <el-option
              v-for="opt in congestionOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
          <div class="field-hint">Linux 建议配置 bbr 获得最佳跨国网络吞吐</div>
        </el-form-item>

        <!-- TCP Fast Open -->
        <el-form-item label="TCP 快速打开 (tcpFastOpen)">
          <div class="inline-row">
            <el-switch
              :model-value="modelValue.tcp_fast_open"
              active-text="开启 TFO"
              @update:model-value="(v: any) => updateField('tcp_fast_open', Boolean(v))"
            />
            <el-input-number
              v-if="modelValue.tcp_fast_open"
              :model-value="modelValue.tcp_fast_open_qlen"
              :min="1"
              :max="2147483647"
              :step="64"
              placeholder="队列上限(默认256)"
              size="small"
              style="width: 160px"
              class="font-mono-num"
              @update:model-value="(v: any) => updateField('tcp_fast_open_qlen', v ? Number(v) : undefined)"
            />
          </div>
          <div class="field-hint">减少 1 个 RTT 握手时延；可选指定连接排队队列上限（默认 256）</div>
        </el-form-item>

        <!-- 多路径 TCP (MPTCP) -->
        <el-form-item label="多路径 TCP (tcpMptcp)">
          <div style="padding-top: 4px">
            <el-switch
              :model-value="modelValue.tcp_mptcp"
              active-text="启用 MPTCP (需 Linux 5.6+)"
              @update:model-value="(v: any) => updateField('tcp_mptcp', Boolean(v))"
            />
          </div>
          <div class="field-hint">允许跨 Wi-Fi 与蜂窝数据多链路聚合传输</div>
        </el-form-item>

        <!-- 接收 PROXY Protocol -->
        <el-form-item label="接收 PROXY Protocol">
          <div style="padding-top: 4px">
            <el-switch
              :model-value="modelValue.accept_proxy_protocol"
              active-text="开启 PROXY Protocol 接收"
              @update:model-value="(v: any) => updateField('accept_proxy_protocol', Boolean(v))"
            />
          </div>
          <div class="field-hint">前置 HAProxy/Nginx 反向代理时获取真实客户端来源 IP</div>
        </el-form-item>

        <!-- TCP Keep-Alive 空闲时间 -->
        <el-form-item label="Keep-Alive 空闲等待 (秒)">
          <el-input-number
            :model-value="modelValue.tcp_keep_alive_idle"
            :min="0"
            :max="86400"
            :step="10"
            placeholder="留空为系统默认"
            style="width: 100%"
            class="font-mono-num"
            @update:model-value="(v: any) => updateField('tcp_keep_alive_idle', v ? Number(v) : undefined)"
          />
          <div class="field-hint">TCP 连接空闲达此阈值后启动保活探测</div>
        </el-form-item>

        <!-- TCP Keep-Alive 探测间隔 -->
        <el-form-item label="Keep-Alive 探测间隔 (秒)">
          <el-input-number
            :model-value="modelValue.tcp_keep_alive_interval"
            :min="0"
            :max="86400"
            :step="5"
            placeholder="留空为系统默认"
            style="width: 100%"
            class="font-mono-num"
            @update:model-value="(v: any) => updateField('tcp_keep_alive_interval', v ? Number(v) : undefined)"
          />
          <div class="field-hint">发送保活探测报文的周期</div>
        </el-form-item>

        <!-- TCP User Timeout -->
        <el-form-item label="TCP 用户超时 (tcpUserTimeout)">
          <el-input-number
            :model-value="modelValue.tcp_user_timeout"
            :min="0"
            :max="3600000"
            :step="1000"
            placeholder="单位毫秒，如 10000"
            style="width: 100%"
            class="font-mono-num"
            @update:model-value="(v: any) => updateField('tcp_user_timeout', v ? Number(v) : undefined)"
          />
          <div class="field-hint">未确认数据被放弃前的最大超时时间（毫秒）</div>
        </el-form-item>

        <!-- SO_MARK 标记 -->
        <el-form-item label="路由网络标记 (SO_MARK)">
          <el-input-number
            :model-value="modelValue.mark"
            :min="0"
            :max="2147483647"
            placeholder="SO_MARK 整数"
            style="width: 100%"
            class="font-mono-num"
            @update:model-value="(v: any) => updateField('mark', v ? Number(v) : undefined)"
          />
          <div class="field-hint">仅适用于 Linux，配合 iptables / nftables 策略路由使用</div>
        </el-form-item>

        <!-- 透明代理 tproxy -->
        <el-form-item label="透明代理模式 (tproxy)">
          <el-select
            :model-value="modelValue.tproxy"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('tproxy', v)"
          >
            <el-option label="关闭 (off)" value="off" />
            <el-option label="TProxy 模式 (支持 TCP/UDP)" value="tproxy" />
            <el-option label="Redirect 模式 (仅 TCP)" value="redirect" />
          </el-select>
          <div class="field-hint">透明代理拦截流量，需内核 CAP_NET_ADMIN 权限</div>
        </el-form-item>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.inline-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 4px;
  line-height: 1.35;
}
</style>
