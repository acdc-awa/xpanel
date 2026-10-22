<script setup lang="ts">
import { computed } from 'vue'
import { InfoFilled, WarningFilled } from '@element-plus/icons-vue'
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
</script>

<template>
  <div class="hysteria-finalmask-box">
    <div class="compact-tip gray">
      <el-icon><InfoFilled /></el-icon>
      <span>
        finalmask 不属于 hysteria 协议本身：它在协议组装与 TLS 加密完成后作用于外层字节 /
        数据包。salamander 与 gecko 是外层混淆，quicParams 是 QUIC 调参，udphop 只改客户端目标端口。
      </span>
    </div>

    <!-- 混淆（finalmask.udp[type=salamander]） -->
    <div class="sub-card">
      <div class="sub-card__title">
        <span class="dot bg-indigo-500"></span>
        <span>混淆 (finalmask.udp[type=salamander])</span>
      </div>
      <div class="form-grid">
        <el-form-item label="混淆方式 (obfs)">
          <el-select
            :model-value="modelValue.obfs_type"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('obfs_type', v)"
          >
            <el-option v-for="o in obfsOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
          <div class="field-hint">
            xray 服务端写 salamander + packetSize 即等价于 mihomo 的 gecko；开启后订阅必须同源下发，否则客户端连不上。
          </div>
        </el-form-item>

        <el-form-item v-if="modelValue.obfs_type !== 'none'" label="混淆密码 (obfs-password)">
          <el-input
            :model-value="modelValue.obfs_password"
            placeholder="必填，客户端需完全一致"
            class="font-mono-num"
            @input="(v: string) => updateField('obfs_password', v)"
          />
        </el-form-item>

        <template v-if="modelValue.obfs_type === 'gecko'">
          <el-form-item label="最小分片字节">
            <el-input-number
              :model-value="modelValue.obfs_min_packet"
              :min="1"
              :max="2048"
              :step="64"
              style="width: 100%"
              class="font-mono-num"
              @update:model-value="(v: any) => updateField('obfs_min_packet', Number(v) || 512)"
            />
          </el-form-item>

          <el-form-item label="最大分片字节">
            <el-input-number
              :model-value="modelValue.obfs_max_packet"
              :min="1"
              :max="2048"
              :step="64"
              style="width: 100%"
              class="font-mono-num"
              @update:model-value="(v: any) => updateField('obfs_max_packet', Number(v) || 1200)"
            />
            <div class="field-hint">默认 512–1200，上限 2048；最小不得大于最大</div>
          </el-form-item>
        </template>
      </div>
    </div>

    <!-- 带宽（finalmask.quicParams.brutal*） -->
    <div class="sub-card">
      <div class="sub-card__title">
        <span class="dot bg-emerald-500"></span>
        <span>带宽限制 (finalmask.quicParams.brutal)</span>
      </div>
      <div class="form-grid">
        <el-form-item label="服务端上行 (brutalUp)">
          <el-input
            :model-value="modelValue.bw_up"
            placeholder="如 100 mbps，留空不限制"
            class="font-mono-num"
            @input="(v: string) => updateField('bw_up', v)"
          />
          <div class="field-hint">服务端发出方向 = 客户端下行；单位支持 k/m/g/t 与 mbps/gbps</div>
        </el-form-item>

        <el-form-item label="服务端下行 (brutalDown)">
          <el-input
            :model-value="modelValue.bw_down"
            placeholder="如 100 mbps，留空不限制"
            class="font-mono-num"
            @input="(v: string) => updateField('bw_down', v)"
          />
          <div class="field-hint">服务端接收方向 = 客户端上行；上下行都填写才会启用 Brutal，否则退回 BBR</div>
        </el-form-item>
      </div>
      <div class="compact-notice">
        服务端声明的是「允许客户端选用的最大 Brutal 速率」；两侧数值需同源下发到订阅，客户端带宽以最小值为准。
      </div>
    </div>

    <!-- 端口跳跃（finalmask.udp[type=udphop]，仅客户端消费） -->
    <div class="sub-card">
      <div class="sub-card__title">
        <span class="dot bg-amber-500"></span>
        <span>端口跳跃 (finalmask.udp[type=udphop]，仅下发客户端)</span>
      </div>
      <div class="form-grid">
        <el-form-item label="跳跃方式 (mode)">
          <el-select
            :model-value="modelValue.port_hop_mode"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('port_hop_mode', v)"
          >
            <el-option label="intervalRemote（定时换目标端口）" value="intervalRemote" />
            <el-option label="perConnRemote（每个连接随机端口）" value="perConnRemote" />
          </el-select>
          <div class="field-hint">两种都能映射到 mihomo 的 ports；intervalLocal 只跳源端口，面板不提供</div>
        </el-form-item>

        <el-form-item label="目标端口集合">
          <el-input
            :model-value="modelValue.port_hop_ports"
            placeholder="如 20000-30000，留空不启用"
            class="font-mono-num"
            @input="(v: string) => updateField('port_hop_ports', v)"
          />
          <div class="field-hint">支持 "20000-30000" 或 "20000-30000,40000"；节点本身仍只监听配置的端口</div>
        </el-form-item>

        <el-form-item v-if="modelValue.port_hop_mode === 'intervalRemote'" label="跳跃间隔 (秒)">
          <el-input
            :model-value="modelValue.port_hop_interval"
            placeholder="默认 30，支持如 10-20"
            class="font-mono-num"
            @input="(v: string) => updateField('port_hop_interval', v)"
          />
          <div class="field-hint">最小 5 秒；mihomo 与 xray 客户端都接受区间写法</div>
        </el-form-item>
      </div>
      <div class="compact-notice">
        端口跳跃只改变客户端发往的目标端口，xray 服务端不支持 udphop（写入节点配置会导致监听失败）。
        面板会把它从生成的服务端配置中剥离，仅保留给订阅与机间出站。
        请先在服务器防火墙 / NAT 把该端口段转发到本入站监听端口。
      </div>
    </div>

    <div v-if="errors.length" class="form-errors">
      <div v-for="e in errors" :key="e" class="form-errors__item">
        <el-icon><WarningFilled /></el-icon>
        <span>{{ e }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.hysteria-finalmask-box {
  display: flex;
  flex-direction: column;
}

.sub-card {
  background: var(--x-fill-2, #f8fafc);
  border: 1px solid var(--x-border-light, #e5e7eb);
  border-radius: var(--x-radius-sm, 8px);
  padding: 12px 14px 4px;
  margin-bottom: 12px;

  &:last-of-type {
    margin-bottom: 8px;
  }
}

.sub-card__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-text-2, #374151);
  margin-bottom: 10px;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.bg-indigo-500 {
  background: #6366f1;
}

.bg-emerald-500 {
  background: #10b981;
}

.bg-amber-500 {
  background: #f59e0b;
}

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 4px;
  line-height: 1.4;
}

.compact-tip {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 11.5px;
  padding: 8px 12px;
  border-radius: 6px;
  line-height: 1.5;
  margin-bottom: 12px;

  .el-icon {
    font-size: 14px;
    flex: none;
    margin-top: 2px;
  }

  &.gray {
    background: var(--x-fill-2, rgba(0, 0, 0, 0.03));
    color: var(--x-text-3, #9ca3af);
    border: 1px solid var(--x-border-light, #e5e7eb);
  }
}

.compact-notice {
  font-size: 11.5px;
  color: var(--x-text-3);
  line-height: 1.5;
  background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px dashed var(--x-border-light, #e5e7eb);
  margin-top: 4px;
  margin-bottom: 8px;
}

.form-errors {
  margin-bottom: 10px;
  padding: 8px 12px;
  border-radius: 6px;
  background: rgba(245, 108, 108, 0.08);
  border: 1px dashed rgba(245, 108, 108, 0.45);
}

.form-errors__item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 11.5px;
  color: #d93026;
  line-height: 1.5;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
