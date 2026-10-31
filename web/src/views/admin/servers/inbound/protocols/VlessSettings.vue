<script setup lang="ts">
import { ref } from 'vue'
import {
  CsBadge,
  CsButton,
  CsContainer,
  CsFormItem,
  CsIconButton,
  CsInput,
  CsSelect,
  ElMessage,
} from '@/components/cloudscape'
import { getXrayVlessEnc } from '@/api/admin'
import { errMsg } from '@/api/http'
import TipIcon from '@/components/base/TipIcon.vue'
import type { FallbackItem } from '@/api/types'
import type { VlessConfig } from '../types'

const props = defineProps<{
  modelValue: VlessConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: VlessConfig): void
}>()

const veGenLoading = ref(false)

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
    <!-- 微卡片 1: 协议内后量子加密（settings.decryption） -->
    <CsContainer header="后量子加密 (VLESS Encryption)">
      <template #actions>
        <CsBadge color="blue">ML-KEM-768</CsBadge>
      </template>
      <CsFormItem
        label="协议解密密钥 (decryption)"
        :constraint-text="modelValue.decryption ? '' : '留空为 none。裸 TCP 想用 Vision，必须开启本项加密（或改用 TLS / REALITY）。'"
      >
        <template #info>
          <TipIcon content="服务端用该密钥解密；订阅下发时自动派生对应的客户端 encryption，无需手工填写。" />
        </template>
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
    </CsContainer>

    <!-- 微卡片 2: 回落规则 (Fallbacks) -->
    <CsContainer header="Web 伪装回落 (Fallbacks - 选填)" class="section-gap">
      <template #info>
        <TipIcon content="客户端认证失败时把流量回落到本机 Web 站点，对外看似普通的 HTTPS 访问，用于抵御主动探测。" />
      </template>
      <template #actions>
        <CsButton icon-name="add-plus" @click="addFallback">添加回落项</CsButton>
      </template>
      <div v-if="!modelValue.fallbacks.length" class="card-note">
        未配置回落：认证失败时直接断连（推荐）。需要伪装时再添加。
      </div>

      <div v-else class="fallback-list">
        <div v-for="(fb, idx) in modelValue.fallbacks" :key="idx" class="fallback-item">
          <div class="fb-head">
            <span class="fb-title">回落 #{{ idx + 1 }}</span>
            <CsIconButton icon-name="remove" label="移除回落项" @click="removeFallback(idx)" />
          </div>
          <div class="form-grid-3">
            <CsFormItem label="回落目标 (Dest)">
              <CsInput
                :model-value="fb.dest"
                placeholder="80 或 127.0.0.1:80"
                class="mono"
                @update:model-value="(v: string) => updateFallback(idx, { dest: v })"
              />
            </CsFormItem>
            <CsFormItem label="Path 路径">
              <template #info>
                <TipIcon content="仅当回落请求的路径匹配时生效；留空匹配所有路径。" />
              </template>
              <CsInput
                :model-value="fb.path || ''"
                placeholder="如 /"
                @update:model-value="(v: string) => updateFallback(idx, { path: v })"
              />
            </CsFormItem>
            <CsFormItem label="SNI (可选)">
              <template #info>
                <TipIcon content="按请求携带的 SNI 选择回落目标；留空不限定。" />
              </template>
              <CsInput
                :model-value="fb.name || ''"
                placeholder="如 site.com"
                @update:model-value="(v: string) => updateFallback(idx, { name: v })"
              />
            </CsFormItem>
            <CsFormItem label="ALPN">
              <template #info>
                <TipIcon content="按协商的 ALPN 选择回落目标，如 h2 / http/1.1；留空不限定。" />
              </template>
              <CsInput
                :model-value="fb.alpn || ''"
                placeholder="如 h2"
                @update:model-value="(v: string) => updateFallback(idx, { alpn: v })"
              />
            </CsFormItem>
            <CsFormItem label="PROXY Protocol" class="fb-span-2">
              <template #info>
                <TipIcon content="向回落目标传递真实来源 IP（v1/v2）；回落目标（如 Nginx）需要支持 PROXY Protocol。" />
              </template>
              <CsSelect
                :model-value="fb.xver ?? 0"
                :options="xverOptions"
                style="width: 100%"
                @update:model-value="(v: any) => updateFallback(idx, { xver: Number(v) })"
              />
            </CsFormItem>
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
  margin-top: 12px;
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

.card-note {
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-form-secondary-wt0md2, #656871);
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
  background: var(--color-background-container-content-mjfil9, #ffffff);
  padding: 10px 14px 2px;
  border-radius: 8px;
  border: 1px solid var(--color-border-card-btvpmf, #e9ebed);
}

.fb-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.fb-title {
  font-size: 12px;
  line-height: 16px;
  font-weight: 600;
  color: var(--color-text-form-secondary-wt0md2, #656871);
}

.fb-span-2 {
  grid-column: span 2;
}

@media (max-width: $bp-tablet-max) {
  .fb-span-2 {
    grid-column: auto;
  }
}
</style>
