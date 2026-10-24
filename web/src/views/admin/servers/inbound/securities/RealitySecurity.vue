<script setup lang="ts">
import { ref } from 'vue'
import {
  CsButton,
  CsButtonDropdown,
  CsContainer,
  CsExpandableSection,
  CsFormItem,
  CsIconButton,
  CsInput,
  CsInputNumber,
  CsSelect,
  ElMessage,
} from '@/components/cloudscape'
import { getXrayKeys } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { RealityConfig } from '../types'

const props = defineProps<{
  modelValue: RealityConfig
  fingerprint?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: RealityConfig): void
  (e: 'update:fingerprint', value: string): void
}>()

const genKeyLoading = ref(false)

const REALITY_PRESETS = [
  { label: 'Cloudflare (www.cloudflare.com:443)', dest: 'www.cloudflare.com:443', sni: 'www.cloudflare.com' },
  { label: 'Amazon AWS (aws.amazon.com:443)', dest: 'aws.amazon.com:443', sni: 'aws.amazon.com' },
  { label: 'Yahoo Japan (www.yahoo.co.jp:443)', dest: 'www.yahoo.co.jp:443', sni: 'www.yahoo.co.jp' },
  { label: 'Docker Registry (registry-1.docker.io:443)', dest: 'registry-1.docker.io:443', sni: 'registry-1.docker.io' },
]

const presetItems = REALITY_PRESETS.map((preset) => ({ id: preset.dest, text: preset.label }))

const fingerprintOptions = [
  { label: 'Chrome (推荐)', value: 'chrome' },
  { label: 'Firefox', value: 'firefox' },
  { label: 'Safari', value: 'safari' },
  { label: 'Edge', value: 'edge' },
  { label: '360', value: '360' },
  { label: 'QQ', value: 'qq' },
  { label: 'Random (随机)', value: 'random' },
  { label: 'Randomized (随机打乱)', value: 'randomized' },
]

function updateField<K extends keyof RealityConfig>(key: K, val: RealityConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function applyPreset(id: string | number) {
  const preset = REALITY_PRESETS.find((item) => item.dest === String(id))
  if (!preset) return
  emit('update:modelValue', {
    ...props.modelValue,
    dest: preset.dest,
    server_name: preset.sni,
  })
  ElMessage.success(`已应用目标预设：${preset.sni}`)
}

function genShortId() {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  const hex = Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
  updateField('short_id', hex)
  ElMessage.success('已随机生成 16 位 Short ID')
}

async function fetchRealityKeys() {
  genKeyLoading.value = true
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0 && data.data) {
      emit('update:modelValue', {
        ...props.modelValue,
        private_key: data.data.private_key,
        public_key: data.data.public_key,
      })
      ElMessage.success('已生成 x25519 密钥对')
    } else {
      ElMessage.error(data.message || '生成失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '请求后端生成密钥失败'))
  } finally {
    genKeyLoading.value = false
  }
}

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="reality-box">
    <CsContainer header="目标伪装与 TLS 模拟 (必填核心)">
      <template #actions>
        <CsButtonDropdown
          :items="presetItems"
          text="常用目标预设"
          aria-label="常用目标预设"
          @select="applyPreset"
        />
      </template>
      <div class="form-grid-2">
        <CsFormItem label="借壳目标域名与端口 (Dest)">
          <CsInput
            :model-value="modelValue.dest"
            placeholder="如 www.cloudflare.com:443"
            class="mono"
            @update:model-value="(v: string) => updateField('dest', v)"
          />
        </CsFormItem>

        <CsFormItem label="伪装 SNI (Server Name)">
          <CsInput
            :model-value="modelValue.server_name"
            placeholder="如 www.cloudflare.com"
            class="mono"
            @update:model-value="(v: string) => updateField('server_name', v)"
          />
        </CsFormItem>

        <CsFormItem label="模拟客户端 TLS 指纹 (Fingerprint)">
          <CsSelect
            :model-value="fingerprint || 'chrome'"
            :options="fingerprintOptions"
            style="width: 100%"
            @update:model-value="(v: any) => emit('update:fingerprint', String(v))"
          />
        </CsFormItem>

        <CsFormItem label="Short ID (客户端认证短标识)">
          <div class="inline-row">
            <CsInput
              :model-value="modelValue.short_id"
              placeholder="16 位十六进制字符"
              class="mono"
              @update:model-value="(v: string) => updateField('short_id', v)"
            />
            <CsIconButton icon-name="refresh" label="随机生成 Short ID" @click="genShortId" />
          </div>
        </CsFormItem>
      </div>
    </CsContainer>

    <CsContainer header="Xray x25519 密钥对认证" class="section-gap">
      <template #actions>
        <CsButton
          variant="primary"
          icon-name="gen-ai"
          :loading="genKeyLoading"
          @click="fetchRealityKeys"
        >
          重新生成密钥对
        </CsButton>
      </template>
      <div class="form-grid-2">
        <CsFormItem label="服务端私钥 (Private Key)">
          <div class="inline-row">
            <CsInput
              :model-value="modelValue.private_key"
              placeholder="x25519 私钥 (仅服务器持有)"
              class="mono"
              @update:model-value="(v: string) => updateField('private_key', v)"
            />
            <CsIconButton
              v-if="modelValue.private_key"
              icon-name="copy"
              label="复制私钥"
              @click="copyText(modelValue.private_key, '私钥')"
            />
          </div>
        </CsFormItem>

        <CsFormItem label="客户端公钥 (Public Key - 自动下发)">
          <div class="inline-row">
            <CsInput
              :model-value="modelValue.public_key"
              placeholder="x25519 公钥 (订阅自动注入)"
              class="mono"
              @update:model-value="(v: string) => updateField('public_key', v)"
            />
            <CsIconButton
              v-if="modelValue.public_key"
              icon-name="copy"
              label="复制公钥"
              @click="copyText(modelValue.public_key, '公钥')"
            />
          </div>
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 折叠区必须自带容器：ExpandableSection 的 default 变体没有左右内边距，
         直接放在面板上会让内容贴到页签边缘（与上方卡片对不齐）。 -->
    <CsContainer class="section-gap" :disable-content-paddings="true">
      <CsExpandableSection header="客户端版本与时间漂移容差 (选填微调)" variant="container">
        <div class="form-grid-3">
          <CsFormItem label="最低版本限制 (minClientVer)">
            <CsInput
              :model-value="modelValue.min_client_ver"
              placeholder="留空不限制，如 1.8.0"
              class="mono"
              @update:model-value="(v: string) => updateField('min_client_ver', v)"
            />
          </CsFormItem>
          <CsFormItem label="最高版本限制 (maxClientVer)">
            <CsInput
              :model-value="modelValue.max_client_ver"
              placeholder="留空不限制"
              class="mono"
              @update:model-value="(v: string) => updateField('max_client_ver', v)"
            />
          </CsFormItem>
          <CsFormItem label="最大允许时钟差 (ms)">
            <CsInputNumber
              :model-value="modelValue.max_time_diff"
              :min="0"
              style="width: 100%"
              placeholder="0 = 不限制"
              class="mono"
              @update:model-value="(v: number | null) => updateField('max_time_diff', Number(v || 0))"
            />
          </CsFormItem>
        </div>
      </CsExpandableSection>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.reality-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 12px;
}

.inline-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.inline-row > :first-child {
  flex: 1;
  min-width: 0;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
