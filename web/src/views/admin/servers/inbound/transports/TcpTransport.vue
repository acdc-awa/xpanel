<script setup lang="ts">
import type { TcpConfig } from '../types'

const props = defineProps<{
  modelValue: TcpConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TcpConfig): void
}>()

function updateField<K extends keyof TcpConfig>(key: K, val: TcpConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}
</script>

<template>
  <div class="transport-box">
    <div class="form-grid">
      <el-form-item label="伪装类型 (Header)">
        <el-select
          :model-value="modelValue.header_type"
          style="width: 100%"
          @update:model-value="(v: 'none' | 'http') => updateField('header_type', v)"
        >
          <el-option label="none (无伪装)" value="none" />
          <el-option label="http (HTTP 伪装)" value="http" />
        </el-select>
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

      <template v-if="modelValue.header_type === 'http'">
        <el-form-item label="Request Host">
          <el-input
            :model-value="modelValue.request_host"
            placeholder="如 example.com"
            @input="(v: string) => updateField('request_host', v)"
          />
        </el-form-item>
        <el-form-item label="Request Path">
          <el-input
            :model-value="modelValue.request_path"
            placeholder="/"
            @input="(v: string) => updateField('request_path', v)"
          />
        </el-form-item>
      </template>
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
