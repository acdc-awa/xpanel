<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Delete, QuestionFilled, CopyDocument, MagicStick } from '@element-plus/icons-vue'
import { ElMessage } from '@/components/cloudscape'
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
    <!-- 微卡片 1: 流控与协议内解密 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
          <span>用户流控 (Flow) 与后量子加密 (VLESS Encryption)</span>
        </div>
        <div class="head-meta">
          <el-tag size="small" type="info">ML-KEM-768</el-tag>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item>
            <template #label>
              <span>流控算法 (Flow)</span>
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
              <el-option label="开启 (xtls-rprx-vision 极速)" value="xtls-rprx-vision" />
              <el-option label="不启用 (none 留空)" value="" />
            </el-select>
            <div v-if="!allowVision" class="muted" style="font-size: 11px; margin-top: 4px">
              Vision 流控仅在 TCP + REALITY / TLS / VLESS Encryption 下生效
            </div>
          </el-form-item>

          <el-form-item label="协议解密密钥 (decryption)">
            <div style="display: flex; gap: 8px; width: 100%">
              <el-input
                :model-value="modelValue.decryption || ''"
                placeholder="留空为关闭 (none)"
                class="font-mono-num"
                @input="(v: string) => updateField('decryption', v)"
              >
                <template #append>
                  <el-button v-if="modelValue.decryption" @click="copyText(modelValue.decryption, '密钥')">
                    <el-icon><CopyDocument /></el-icon>
                  </el-button>
                </template>
              </el-input>
              <el-button type="primary" plain :loading="veGenLoading" @click="fetchVlessencKey">
                <el-icon><MagicStick /></el-icon>&nbsp;生成
              </el-button>
            </div>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 2: 回落规则 (Fallbacks) -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-sky-500"></span>
          <span>Web 伪装回落 (Fallbacks - 选填)</span>
        </div>
        <div class="head-meta">
          <el-button size="small" type="primary" plain @click="addFallback">
            <el-icon><Plus /></el-icon>&nbsp;添加回落项
          </el-button>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div v-if="!modelValue.fallbacks.length" class="empty-tip">
          未配置回落规则。客户端认证失败时直接中断连接（推荐），或添加回落转发至 Web 站点以防探测。
        </div>

        <div v-else class="fallback-list">
          <div v-for="(fb, idx) in modelValue.fallbacks" :key="idx" class="fallback-item">
            <div class="fb-grid">
              <el-form-item label="SNI (可选)">
                <el-input
                  :model-value="fb.name || ''"
                  placeholder="如 site.com"
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
                  class="font-mono-num"
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
  </div>
</template>

<style scoped lang="scss">
.vless-settings-box {
  display: flex;
  flex-direction: column;
}

:deep(.el-form-item) {
  margin-bottom: 0;
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
  font-size: 12px;
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
