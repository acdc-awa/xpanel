<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Delete, QuestionFilled, CopyDocument, MagicStick } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getXrayVlessEnc } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { FallbackItem } from '@/api/types'
import type { VlessConfig } from '../types'

const props = defineProps<{
  modelValue: VlessConfig
  inboundType?: string
  internalUUID?: string
  inboundId?: number
  allowVision: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: VlessConfig): void
}>()

const veGenLoading = ref(false)

async function fetchVlessencKey() {
  veGenLoading.value = true
  try {
    const { data } = await getXrayVlessEnc()
    if (data.code === 0 && data.data) {
      updateField('decryption', data.data.decryption)
      ElMessage.success('已生成 VLESS Encryption 密钥')
    } else {
      ElMessage.error(data.message || '生成失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成密钥失败'))
  } finally {
    veGenLoading.value = false
  }
}

function updateField<K extends keyof VlessConfig>(key: K, val: VlessConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function addFallback() {
  const list = [...props.modelValue.fallbacks]
  list.push({ dest: '80', path: '', alpn: '', xver: 0, name: '' })
  updateField('fallbacks', list)
}

function removeFallback(idx: number) {
  const list = [...props.modelValue.fallbacks]
  list.splice(idx, 1)
  updateField('fallbacks', list)
}

function updateFallback(idx: number, patch: Partial<FallbackItem>) {
  const list = [...props.modelValue.fallbacks]
  list[idx] = { ...list[idx], ...patch }
  updateField('fallbacks', list)
}

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="vless-settings-box">
    <!-- 用户流控 (Flow) -->
    <div class="form-grid">
      <el-form-item>
        <template #label>
          <span>流控 (Flow)</span>
          <el-tooltip content="TCP + REALITY 或 TLS / VLESS Encryption 建议开启 xtls-rprx-vision。" placement="top">
            <el-icon class="help-icon"><QuestionFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-select
          :model-value="modelValue.flow"
          :disabled="!allowVision"
          style="width: 100%"
          @update:model-value="(v: string) => updateField('flow', v)"
        >
          <el-option label="开启 (xtls-rprx-vision)" value="xtls-rprx-vision" />
          <el-option label="不启用 (留空)" value="" />
        </el-select>
        <div v-if="!allowVision" class="muted" style="font-size: 11.5px; margin-top: 4px">
          Vision 流控仅在 TCP + REALITY / TLS / VLESS Encryption 下生效
        </div>
      </el-form-item>
    </div>

    <!-- 协议层加密 (VLESS Encryption / ML-KEM-768) -->
    <div class="vlessenc-card">
      <div class="card-head-flex" style="margin-bottom: 8px">
        <span class="sub-title">协议加密 (VLESS Encryption)</span>
        <el-tag size="small" type="info">ML-KEM-768 后量子算法</el-tag>
      </div>
      <el-form-item label="解密密钥 (decryption)">
        <div style="display: flex; gap: 8px; width: 100%">
          <el-input
            :model-value="modelValue.decryption || ''"
            placeholder="留空即为关闭 (none)；点击右侧生成或填入解密密钥"
            @input="(v: string) => updateField('decryption', v)"
          />
          <el-button v-if="modelValue.decryption" title="复制密钥" @click="copyText(modelValue.decryption, '密钥')">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
          <el-button type="primary" plain :loading="veGenLoading" @click="fetchVlessencKey">
            <el-icon><MagicStick /></el-icon>&nbsp;生成密钥
          </el-button>
        </div>
      </el-form-item>
      <div class="muted" style="font-size: 11.5px; line-height: 1.5; margin-top: 4px">
        基于后量子算法的协议内加密方案，适用于节点间私有中继链路。启用后协议层实施解密保护，底层传输层可保持明文 (none) 并支持 Splice / Vision 直通。
      </div>
    </div>

    <!-- Fallbacks 回落 -->
    <div class="fallback-section">
      <div class="card-head-flex">
        <span class="sub-title">回落设置 (Fallbacks)</span>
        <el-button size="small" type="primary" plain @click="addFallback">
          <el-icon><Plus /></el-icon>&nbsp;添加回落项
        </el-button>
      </div>

      <div v-if="!modelValue.fallbacks.length" class="empty-tip">
        未配置回落规则（可选）。可用于在认证失败时伪装为 Web 服务。
      </div>

      <div v-else class="fallback-list">
        <div v-for="(fb, idx) in modelValue.fallbacks" :key="idx" class="fallback-item">
          <div class="fb-grid">
            <el-form-item label="SNI (可选)">
              <el-input
                :model-value="fb.name || ''"
                placeholder="如 example.com"
                @input="(v: string) => updateFallback(idx, { name: v })"
              />
            </el-form-item>
            <el-form-item label="ALPN">
              <el-input
                :model-value="fb.alpn || ''"
                placeholder="如 h2"
                @input="(v: string) => updateFallback(idx, { alpn: v })"
              />
            </el-form-item>
            <el-form-item label="Path 路径">
              <el-input
                :model-value="fb.path || ''"
                placeholder="如 /"
                @input="(v: string) => updateFallback(idx, { path: v })"
              />
            </el-form-item>
            <el-form-item label="回落目标 (Dest)">
              <el-input
                :model-value="fb.dest"
                placeholder="80 或 127.0.0.1:80"
                @input="(v: string) => updateFallback(idx, { dest: v })"
              />
            </el-form-item>
            <el-form-item label="PROXY Protocol">
              <el-select
                :model-value="fb.xver ?? 0"
                style="width: 100%"
                @update:model-value="(v: number) => updateFallback(idx, { xver: v })"
              >
                <el-option label="不发送 (0)" :value="0" />
                <el-option label="Version 1" :value="1" />
                <el-option label="Version 2" :value="2" />
              </el-select>
            </el-form-item>
          </div>
          <div class="fb-actions">
            <el-button type="danger" text @click="removeFallback(idx)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.vless-settings-box {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.relay-card,
.vlessenc-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px 14px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.card-head-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.sub-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text);
}

.help-icon {
  color: var(--x-text-3);
  font-size: 13px;
  margin-left: 4px;
  cursor: help;
  &:hover { color: var(--x-primary); }
}

.empty-tip {
  color: var(--x-text-3);
  font-size: 12.5px;
  padding: 10px 0;
  text-align: center;
}

.fallback-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.fallback-item {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  background: var(--x-card);
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid var(--x-border);
}

.fb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 8px 10px;
  flex: 1;

  :deep(.el-form-item) {
    margin-bottom: 0;
  }
}

.fb-actions {
  display: flex;
  align-items: center;
  padding-bottom: 2px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .fallback-item {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  .fb-actions {
    justify-content: flex-end;
  }
}
</style>
