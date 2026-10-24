<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsCheckbox,
  CsContainer,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsSelect,
  CsTooltip,
} from '@/components/cloudscape'
import type { HysteriaConfig, HysteriaMasquerade } from '../types'
import { validateHysteriaSettings } from '../validate'

const masqueradeOptions = [
  { label: '404（默认，无伪装）', value: '404' },
  { label: 'file（本地静态目录）', value: 'file' },
  { label: 'proxy（反代到目标站点）', value: 'proxy' },
  { label: 'string（固定响应内容）', value: 'string' },
]

const props = defineProps<{
  modelValue: HysteriaConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: HysteriaConfig): void
}>()

function updateField<K extends keyof HysteriaConfig>(key: K, val: HysteriaConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function updateMasq<K extends keyof HysteriaMasquerade>(key: K, val: HysteriaMasquerade[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    masquerade: { ...props.modelValue.masquerade, [key]: val },
  })
}

const errors = computed(() => validateHysteriaSettings(props.modelValue))
</script>

<template>
  <div class="hysteria-settings-box">
    <!-- 协议运行参数（hysteriaSettings） -->
    <CsContainer header="运行参数 (hysteriaSettings)">
      <div class="form-grid">
        <CsFormItem>
          <template #label>
            <span>UDP 空闲超时 (秒)</span>
            <CsTooltip content="连接在无数据交换时的超时释放时间，默认 60 秒。">
              <CsIcon name="status-info" class="help-icon" />
            </CsTooltip>
          </template>
          <CsInputNumber
            :model-value="modelValue.udp_idle_timeout ?? 60"
            :min="5"
            :max="600"
            :step="10"
            style="width: 100%"
            @update:model-value="(v: number | null) => updateField('udp_idle_timeout', typeof v === 'number' ? v : 60)"
          />
          <div class="field-hint cs-text-secondary">账号由绑定的接入点动态注入，此处不单独维护用户列表。</div>
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 伪装站点（hysteriaSettings.masquerade，纯服务端） -->
    <CsContainer header="伪装站点 (hysteriaSettings.masquerade，仅服务端)" class="section-gap">
      <div class="form-grid">
        <CsFormItem label="伪装类型">
          <CsSelect
            :model-value="modelValue.masquerade.type"
            :options="masqueradeOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateMasq('type', String(v) as HysteriaMasquerade['type'])"
          />
          <div class="field-hint cs-text-secondary">非 Hysteria 请求（如浏览器直接访问）返回的内容；不影响代理连接</div>
        </CsFormItem>

        <CsFormItem v-if="modelValue.masquerade.type === 'file'" label="静态目录 (dir)">
          <CsInput
            :model-value="modelValue.masquerade.dir"
            placeholder="/var/www/html"
            class="mono"
            @update:model-value="(v: string) => updateMasq('dir', v)"
          />
        </CsFormItem>

        <template v-if="modelValue.masquerade.type === 'proxy'">
          <CsFormItem label="目标 URL (url)">
            <CsInput
              :model-value="modelValue.masquerade.url"
              placeholder="https://example.com 或 unix:/run/app.sock"
              class="mono"
              @update:model-value="(v: string) => updateMasq('url', v)"
            />
            <div class="field-hint cs-text-secondary">支持 http/https；unix: 开头走本地 Unix Socket</div>
          </CsFormItem>
          <CsFormItem label="反代选项">
            <div class="inline-row">
              <CsCheckbox
                :model-value="modelValue.masquerade.rewriteHost"
                label="改写 Host"
                @update:model-value="(v: boolean) => updateMasq('rewriteHost', v)"
              />
              <CsCheckbox
                :model-value="modelValue.masquerade.xForwarded"
                label="追加 X-Forwarded"
                @update:model-value="(v: boolean) => updateMasq('xForwarded', v)"
              />
              <CsCheckbox
                :model-value="modelValue.masquerade.insecure"
                label="跳过证书校验"
                @update:model-value="(v: boolean) => updateMasq('insecure', v)"
              />
            </div>
          </CsFormItem>
        </template>

        <template v-if="modelValue.masquerade.type === 'string'">
          <CsFormItem label="响应内容 (content)">
            <CsInput
              :model-value="modelValue.masquerade.content"
              type="textarea"
              :rows="3"
              placeholder="例如一段 HTML"
              class="mono"
              @update:model-value="(v: string) => updateMasq('content', v)"
            />
          </CsFormItem>
          <CsFormItem label="状态码 (statusCode)">
            <CsInputNumber
              :model-value="modelValue.masquerade.statusCode"
              :min="100"
              :max="599"
              :step="1"
              placeholder="默认 200"
              style="width: 100%"
              class="mono"
              @update:model-value="(v: number | null) => updateMasq('statusCode', v == null ? undefined : v)"
            />
          </CsFormItem>
        </template>
      </div>
      <CsAlert v-if="modelValue.masquerade.type !== '404'" type="info" class="notice-gap">
        伪装响应只对「未通过 Hysteria 认证的普通 HTTP 请求」生效；代理流量不受影响。
      </CsAlert>
    </CsContainer>

    <CsAlert v-if="errors.length" type="error" class="errors-gap">
      <div v-for="e in errors" :key="e">{{ e }}</div>
    </CsAlert>

    <!-- 紧凑轻量提示 -->
    <CsAlert type="info" class="notice-gap">
      提示：Hysteria 2 采用 UDP (QUIC) 传输并强制 TLS 加密，账号由绑定的接入点动态提供。请在「安全层」绑定有效证书。
    </CsAlert>
  </div>
</template>

<style scoped lang="scss">
.hysteria-settings-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.notice-gap {
  margin-top: 4px;
}

.errors-gap {
  margin-top: 10px;
}

.help-icon {
  margin-left: 4px;
  cursor: help;
}

.inline-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 14px;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.4;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
