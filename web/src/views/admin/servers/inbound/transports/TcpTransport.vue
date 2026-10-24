<script setup lang="ts">
import { CsContainer, CsFormItem, CsInput, CsSelect, CsToggle } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import type { TcpConfig } from '../types'

const props = defineProps<{
  modelValue: TcpConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TcpConfig): void
}>()

function updateField<K extends keyof TcpConfig>(key: K, val: TcpConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

const headerTypeOptions = [
  { label: 'none (无伪装，直通)', value: 'none' },
  { label: 'http (HTTP 头部混淆)', value: 'http' },
]
</script>

<template>
  <CsContainer header="TCP (RAW) 传输流配置">
    <div class="form-grid-2">
      <CsFormItem label="TCP 伪装类型 (Header)">
        <CsSelect
          :model-value="modelValue.header_type"
          :options="headerTypeOptions"
          style="width: 100%"
          @update:model-value="(v: any) => updateField('header_type', String(v) as TcpConfig['header_type'])"
        />
      </CsFormItem>

      <CsFormItem label="前置反向代理 (Proxy Protocol)">
        <template #info>
          <TipIcon content="前置 Caddy / Nginx 发送 PROXY Protocol 时开启；xray 从连接头解析真实来源 IP。" />
        </template>
        <div class="control-cell">
          <CsToggle
            :model-value="modelValue.acceptProxyProtocol"
            label="接收 Proxy Protocol v1/v2"
            @update:model-value="(v: boolean) => updateField('acceptProxyProtocol', v)"
          />
        </div>
      </CsFormItem>

      <template v-if="modelValue.header_type === 'http'">
        <CsFormItem label="HTTP Request Host">
          <CsInput
            :model-value="modelValue.request_host"
            placeholder="如 example.com"
            class="mono"
            @update:model-value="(v: string) => updateField('request_host', v)"
          />
        </CsFormItem>
        <CsFormItem label="HTTP Request Path">
          <CsInput
            :model-value="modelValue.request_path"
            placeholder="/"
            class="mono"
            @update:model-value="(v: string) => updateField('request_path', v)"
          />
        </CsFormItem>
      </template>
    </div>
  </CsContainer>
</template>

<style scoped lang="scss">
.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
