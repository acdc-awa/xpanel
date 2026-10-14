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
  <div class="x-micro-card">
    <div class="x-micro-card__head">
      <div class="head-title">
        <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
        <span>TCP (RAW) 传输流配置</span>
      </div>
      <div class="head-meta">
        标准传输模式
      </div>
    </div>
    <div class="x-micro-card__body">
      <div class="form-grid-2">
        <el-form-item label="TCP 伪装类型 (Header)">
          <el-select
            :model-value="modelValue.header_type"
            style="width: 100%"
            @update:model-value="(v: 'none' | 'http') => updateField('header_type', v)"
          >
            <el-option label="none (无伪装，直通)" value="none" />
            <el-option label="http (HTTP 头部混淆)" value="http" />
          </el-select>
        </el-form-item>

        <el-form-item label="前置反向代理 (Proxy Protocol)">
          <div style="padding-top: 4px">
            <el-switch
              :model-value="modelValue.acceptProxyProtocol"
              active-text="接收 Proxy Protocol v1/v2"
              @update:model-value="(v: any) => updateField('acceptProxyProtocol', Boolean(v))"
            />
          </div>
        </el-form-item>

        <template v-if="modelValue.header_type === 'http'">
          <el-form-item label="HTTP Request Host">
            <el-input
              :model-value="modelValue.request_host"
              placeholder="如 example.com"
              class="font-mono-num"
              @input="(v: string) => updateField('request_host', v)"
            />
          </el-form-item>
          <el-form-item label="HTTP Request Path">
            <el-input
              :model-value="modelValue.request_path"
              placeholder="/"
              class="font-mono-num"
              @input="(v: string) => updateField('request_path', v)"
            />
          </el-form-item>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
:deep(.el-form-item) {
  margin-bottom: 0;
}
</style>
