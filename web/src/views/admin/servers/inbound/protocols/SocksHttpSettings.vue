<script setup lang="ts">
import type { SocksHttpConfig } from '../types'

const props = defineProps<{
  protocol: 'socks' | 'http'
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

      <el-form-item v-if="protocol === 'socks'" label="UDP 转发">
        <div style="padding-top: 6px">
          <el-switch
            :model-value="modelValue.udp"
            active-text="启用 UDP 转发"
            @update:model-value="(v: any) => updateField('udp', Boolean(v))"
          />
        </div>
      </el-form-item>
    </div>

    <!-- 统一接入点鉴权提示 -->
    <div class="auth-notice">
      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="接入点统一鉴权与转发用户动态装配"
        description="物理入站不持有静态账号。请在「接入点」或「拓扑画布」中创建接入点并连线指向本入站，系统会自动聚合所有关联接入点开放权限组下的「转发用户」生成认证账号并提供独立计量计费。若无有效关联账号，本入站将被物理跳过以杜绝免密开放风险。"
      />
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

.auth-notice {
  margin-top: 8px;
  margin-bottom: 14px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
