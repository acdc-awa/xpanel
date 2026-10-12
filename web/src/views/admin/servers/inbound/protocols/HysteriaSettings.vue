<script setup lang="ts">
import { CopyDocument, Refresh, QuestionFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { rotateInternalInbound } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { HysteriaConfig } from '../types'

const props = defineProps<{
  modelValue: HysteriaConfig
  inboundType: string
  internalUUID?: string
  inboundId?: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: HysteriaConfig): void
  (e: 'internal-uuid-changed', value: string): void
}>()

function updateField<K extends keyof HysteriaConfig>(key: K, val: HysteriaConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

async function rotateInternal() {
  if (!props.inboundId) {
    ElMessage.warning('请先保存入站后再执行轮换')
    return
  }
  try {
    const { data } = await rotateInternalInbound(props.inboundId)
    if (data.code === 0 && data.data?.internal_uuid) {
      emit('internal-uuid-changed', data.data.internal_uuid)
      ElMessage.success('内部 UUID 已成功轮换')
    } else {
      ElMessage.error(data.message || '轮换失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '轮换内部 UUID 失败'))
  }
}

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="hysteria-settings-box">
    <!-- Relay 内部 UUID 状态（节点间互联中继认证） -->
    <div v-if="inboundType === 'relay'" class="relay-card">
      <el-form-item label="内部中继认证 UUID">
        <div style="display: flex; gap: 8px; width: 100%">
          <el-input :model-value="internalUUID" placeholder="保存并下发后由系统生成" disabled />
          <el-button v-if="internalUUID" @click="copyText(internalUUID, '内部 UUID')">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
          <el-button type="primary" plain :disabled="!inboundId" @click="rotateInternal">
            <el-icon><Refresh /></el-icon>&nbsp;轮换
          </el-button>
        </div>
      </el-form-item>
      <p class="relay-tip">
        本入站作为链式代理落地端时，上游前置节点出站将使用该内部 UUID 作为认证凭据进行握手。
      </p>
    </div>

    <!-- 用户入站提示 -->
    <div v-else class="auth-notice">
      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="接入点统一鉴权与用户动态装配"
        description="Hysteria 2 基于 UDP (QUIC) 协议，专为恶劣网络和高丢包环境设计。服务端物理入站不设全局共享密码，认证完全由接入点关联的用户凭据（系统分配的用户 UUID 作为 auth）承担并进行独立计量计费与限速。"
      />
    </div>

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

      <el-form-item label="传输与安全绑定">
        <el-input value="强制 QUIC/UDP 传输 + TLS 加密" disabled />
      </el-form-item>
    </div>

    <!-- 证书提醒 -->
    <div class="cert-notice">
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="TLS 证书要求"
        description="Hysteria 2 强制要求 TLS 加密。请切换至「安全层」标签页绑定有效证书（域名证书或面板自签证书）。若为内部中继链路，对端节点出站将自动进行证书指纹锁定 (Pinned SHA256)。"
      />
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

.auth-notice {
  margin-bottom: 16px;
}

.cert-notice {
  margin-top: 8px;
  margin-bottom: 12px;
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
