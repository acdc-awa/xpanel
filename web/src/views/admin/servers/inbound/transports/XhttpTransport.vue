<script setup lang="ts">
import type { XhttpConfig } from '../types'

const props = defineProps<{
  modelValue: XhttpConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: XhttpConfig): void
}>()

function updateField<K extends keyof XhttpConfig>(key: K, val: XhttpConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}
</script>

<template>
  <div class="transport-box">
    <div class="form-grid">
      <el-form-item label="传输模式 (Mode)">
        <el-select
          :model-value="modelValue.mode"
          style="width: 100%"
          @update:model-value="(v: 'auto' | 'packet-up' | 'stream-up' | 'stream-one') => updateField('mode', v)"
        >
          <el-option label="auto (自动)" value="auto" />
          <el-option label="packet-up" value="packet-up" />
          <el-option label="stream-up" value="stream-up" />
          <el-option label="stream-one" value="stream-one" />
        </el-select>
      </el-form-item>

      <el-form-item label="Path 路径">
        <el-input
          :model-value="modelValue.path"
          placeholder="/xhttp-stream"
          @input="(v: string) => updateField('path', v)"
        />
      </el-form-item>

      <el-form-item label="Host 域名标头 (可选)">
        <el-input
          :model-value="modelValue.host"
          placeholder="留空默认"
          @input="(v: string) => updateField('host', v)"
        />
      </el-form-item>

      <el-form-item label="Proxy Protocol">
        <div style="padding-top: 6px">
          <el-switch
            :model-value="modelValue.acceptProxyProtocol"
            active-text="接收 Proxy Protocol"
            @update:model-value="(v: any) => updateField('acceptProxyProtocol', Boolean(v))"
          />
        </div>
      </el-form-item>
    </div>
  </div>
</template>

<style scoped lang="scss">
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
