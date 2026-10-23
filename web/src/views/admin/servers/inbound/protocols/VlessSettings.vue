<script setup lang="ts">
import { ref } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsFormItem,
  CsIcon,
  CsIconButton,
  CsInput,
  CsSelect,
  CsTooltip,
  ElMessage,
} from '@/components/cloudscape'
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

const flowOptions = [
  { label: '开启 (xtls-rprx-vision 极速)', value: 'xtls-rprx-vision' },
  { label: '不启用 (none 留空)', value: '' },
]

const xverOptions = [
  { label: '不发送 (0)', value: 0 },
  { label: 'Version 1', value: 1 },
  { label: 'Version 2', value: 2 },
]

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
    <CsContainer header="用户流控 (Flow) 与后量子加密 (VLESS Encryption)">
      <template #actions>
        <CsBadge color="blue">ML-KEM-768</CsBadge>
      </template>
      <div class="form-grid-2">
        <CsFormItem>
          <template #label>
            <span>流控算法 (Flow)</span>
            <CsTooltip content="TCP + REALITY 或 TLS / VLESS Encryption 建议开启 xtls-rprx-vision。">
              <CsIcon name="status-info" class="help-icon" />
            </CsTooltip>
          </template>
          <CsSelect
            :model-value="modelValue.flow"
            :options="flowOptions"
            :disabled="!allowVision"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('flow', String(v))"
          />
          <div v-if="!allowVision" class="field-hint cs-text-secondary">
            Vision 流控仅在 TCP + REALITY / TLS / VLESS Encryption 下生效
          </div>
        </CsFormItem>

        <CsFormItem label="协议解密密钥 (decryption)">
          <div class="inline-row">
            <CsInput
              :model-value="modelValue.decryption || ''"
              placeholder="留空为关闭 (none)"
              class="mono"
              @update:model-value="(v: string) => updateField('decryption', v)"
            />
            <CsIconButton
              v-if="modelValue.decryption"
              icon-name="copy"
              label="复制密钥"
              @click="copyText(modelValue.decryption, '密钥')"
            />
            <CsButton
              variant="primary"
              icon-name="gen-ai"
              :loading="veGenLoading"
              @click="fetchVlessencKey"
            >
              生成
            </CsButton>
          </div>
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 微卡片 2: 回落规则 (Fallbacks) -->
    <CsContainer header="Web 伪装回落 (Fallbacks - 选填)" class="section-gap">
      <template #actions>
        <CsButton icon-name="add-plus" @click="addFallback">添加回落项</CsButton>
      </template>
      <CsAlert v-if="!modelValue.fallbacks.length" type="info">
        未配置回落规则。客户端认证失败时直接中断连接（推荐），或添加回落转发至 Web 站点以防探测。
      </CsAlert>

      <div v-else class="fallback-list">
        <div v-for="(fb, idx) in modelValue.fallbacks" :key="idx" class="fallback-item">
          <div class="fb-grid">
            <CsFormItem label="SNI (可选)">
              <CsInput
                :model-value="fb.name || ''"
                placeholder="如 site.com"
                @update:model-value="(v: string) => updateFallback(idx, { name: v })"
              />
            </CsFormItem>
            <CsFormItem label="ALPN">
              <CsInput
                :model-value="fb.alpn || ''"
                placeholder="如 h2"
                @update:model-value="(v: string) => updateFallback(idx, { alpn: v })"
              />
            </CsFormItem>
            <CsFormItem label="Path 路径">
              <CsInput
                :model-value="fb.path || ''"
                placeholder="如 /"
                @update:model-value="(v: string) => updateFallback(idx, { path: v })"
              />
            </CsFormItem>
            <CsFormItem label="回落目标 (Dest)">
              <CsInput
                :model-value="fb.dest"
                placeholder="80 或 127.0.0.1:80"
                class="mono"
                @update:model-value="(v: string) => updateFallback(idx, { dest: v })"
              />
            </CsFormItem>
            <CsFormItem label="PROXY Protocol">
              <CsSelect
                :model-value="fb.xver ?? 0"
                :options="xverOptions"
                style="width: 100%"
                @update:model-value="(v: any) => updateFallback(idx, { xver: Number(v) })"
              />
            </CsFormItem>
          </div>
          <div class="fb-actions">
            <CsIconButton icon-name="remove" label="移除回落项" @click="removeFallback(idx)" />
          </div>
        </div>
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.vless-settings-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.help-icon {
  margin-left: 4px;
  cursor: help;
}

.inline-row {
  display: flex;
  gap: 8px;
  width: 100%;
  align-items: center;
}

.inline-row > :first-child {
  flex: 1;
  min-width: 0;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.35;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
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
  background: var(--color-background-container-content-mjfil9, #ffffff);
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid var(--color-border-card-btvpmf, #e9ebed);
}

.fb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 8px 10px;
  flex: 1;
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
