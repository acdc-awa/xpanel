<script setup lang="ts">
import { QuestionFilled } from '@element-plus/icons-vue'
import type { HysteriaConfig } from '../types'

const props = defineProps<{
  modelValue: HysteriaConfig
  inboundType?: string
  internalUUID?: string
  inboundId?: number
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
</script>

<template>
  <div class="hysteria-settings-box">
    <!-- 协议运行参数 -->
    <div class="form-grid">
      <el-form-item>
        <template #label>
          <span>UDP 空闲超时 (秒)</span>
          <el-tooltip content="连接在无数据交换时的超时释放时间，默认 60 秒。" placement="top">
            <el-icon class="help-icon"><QuestionFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number
          :model-value="modelValue.udp_idle_timeout ?? 60"
          :min="5"
          :max="600"
          :step="10"
          style="width: 100%"
          @update:model-value="(v) => updateField('udp_idle_timeout', typeof v === 'number' ? v : 60)"
        />
      </el-form-item>

      <el-form-item label="传输与加密模式">
        <el-input value="QUIC / UDP 传输 + 强制 TLS" disabled />
      </el-form-item>
    </div>

    <!-- 紧凑轻量提示 -->
    <div class="compact-notice">
      <span>提示：Hysteria 2 采用 UDP (QUIC) 传输并强制 TLS 加密，账号由绑定的接入点动态提供。请在「安全层」绑定有效证书。</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.hysteria-settings-box {
  display: flex;
  flex-direction: column;
}

.relay-card {
  background: var(--x-fill-2, #f8fafc);
  border: 1px dashed var(--x-border);
  border-radius: var(--x-radius-sm, 8px);
  padding: 12px 14px;
  margin-bottom: 16px;

  :deep(.el-form-item) {
    margin-bottom: 6px;
  }
}

.relay-tip {
  font-size: 11.5px;
  color: var(--x-text-3);
  margin: 0;
  line-height: 1.4;
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
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.help-icon {
  margin-left: 4px;
  color: var(--x-text-3);
  vertical-align: middle;
  cursor: pointer;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
