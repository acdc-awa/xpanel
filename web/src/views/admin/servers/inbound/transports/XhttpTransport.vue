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
  <div class="x-micro-card">
    <div class="x-micro-card__head">
      <div class="head-title">
        <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
        <span>xhttp (SplitHTTP) 传输流配置</span>
      </div>
      <div class="head-meta">
        新一代多路分包传输
      </div>
    </div>
    <div class="x-micro-card__body">
      <div class="form-grid-2">
        <el-form-item label="传输模式 (Mode)">
          <el-select
            :model-value="modelValue.mode"
            style="width: 100%"
            @update:model-value="(v: 'auto' | 'packet-up' | 'stream-up' | 'stream-one') => updateField('mode', v)"
          >
            <el-option label="auto (自动协商)" value="auto" />
            <el-option label="packet-up (数据报上行)" value="packet-up" />
            <el-option label="stream-up (流式上行)" value="stream-up" />
            <el-option label="stream-one (单向全双工流)" value="stream-one" />
          </el-select>
        </el-form-item>

        <el-form-item label="分流 Path 路径">
          <el-input
            :model-value="modelValue.path"
            placeholder="/xhttp-stream"
            class="font-mono-num"
            @input="(v: string) => updateField('path', v)"
          />
        </el-form-item>

        <el-form-item label="Host 域名标头 (可选)">
          <el-input
            :model-value="modelValue.host"
            placeholder="留空默认"
            class="font-mono-num"
            @input="(v: string) => updateField('host', v)"
          />
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
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
:deep(.el-form-item) {
  margin-bottom: 0;
}
</style>
