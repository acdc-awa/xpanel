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
  <div class="socks-http-box">
    <div class="form-grid">
      <el-form-item label="认证方式">
        <el-select
          :model-value="'password'"
          style="width: 100%"
          disabled
        >
          <el-option label="账密认证 (强制)" value="password" />
        </el-select>
      </el-form-item>

      <el-form-item v-if="protocol === 'socks' || protocol === 'mixed'" label="UDP 转发">
        <div style="padding-top: 6px">
          <el-switch
            :model-value="modelValue.udp"
            active-text="启用 UDP 转发"
            @update:model-value="(v: any) => updateField('udp', Boolean(v))"
          />
        </div>
      </el-form-item>
    </div>

    <!-- 接入点账号装配提示 -->
    <div class="compact-notice">
      <span>提示：SOCKS/HTTP 认证账号由关联的接入点与转发用户动态装配；若无有效账号，入站将物理跳过以保障安全。</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.socks-http-box {
  display: flex;
  flex-direction: column;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
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
  margin-top: 6px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
