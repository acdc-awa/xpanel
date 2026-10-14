<script setup lang="ts">
import type { SocksHttpConfig } from '../types'

const props = defineProps<{
  protocol: 'socks' | 'http' | 'mixed'
  modelValue: SocksHttpConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: SocksHttpConfig): void
}>()

function updateField<K extends keyof SocksHttpConfig>(key: K, val: SocksHttpConfig[K]) {
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
        <span>SOCKS / HTTP 协议策略</span>
      </div>
      <div class="head-meta">
        强制账密认证
      </div>
    </div>
    <div class="x-micro-card__body">
      <div class="form-grid-2">
        <el-form-item label="认证鉴权模式">
          <el-select
            :model-value="'password'"
            style="width: 100%"
            disabled
          >
            <el-option label="账密认证 (强制防空开公网)" value="password" />
          </el-select>
        </el-form-item>

        <el-form-item v-if="protocol === 'socks' || protocol === 'mixed'" label="UDP 转发支持">
          <div style="padding-top: 4px">
            <el-switch
              :model-value="modelValue.udp"
              active-text="启用 UDP 代理通道"
              @update:model-value="(v: any) => updateField('udp', Boolean(v))"
            />
          </div>
        </el-form-item>
      </div>

      <!-- 接入点账号装配提示 -->
      <div class="compact-notice">
        <span>安全铁律：SOCKS/HTTP 认证账号由关联的接入点与转发用户动态装配；若无有效账号，入站将物理跳过以防公网免密开放。</span>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
:deep(.el-form-item) {
  margin-bottom: 0;
}

.compact-notice {
  font-size: 11.5px;
  color: var(--x-text-3);
  line-height: 1.5;
  background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px dashed var(--x-border);
  margin-top: 12px;
}
</style>
