<script setup lang="ts">
import { computed } from 'vue'
import { QuestionFilled, WarningFilled } from '@element-plus/icons-vue'
import type { HysteriaConfig, HysteriaMasquerade } from '../types'
import { validateHysteriaSettings } from '../validate'

const props = defineProps<{
  modelValue: HysteriaConfig
  inboundType?: string
  internalUUID?: string
  inboundId?: number
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
    <div class="sub-card">
      <div class="sub-card__title">
        <span class="dot bg-sky-500"></span>
        <span>运行参数 (hysteriaSettings)</span>
      </div>
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
          <div class="field-hint">账号由绑定的接入点动态注入，此处不单独维护用户列表。</div>
        </el-form-item>
      </div>
    </div>

    <!-- 伪装站点（hysteriaSettings.masquerade，纯服务端） -->
    <div class="sub-card">
      <div class="sub-card__title">
        <span class="dot bg-rose-500"></span>
        <span>伪装站点 (hysteriaSettings.masquerade，仅服务端)</span>
      </div>
      <div class="form-grid">
        <el-form-item label="伪装类型">
          <el-select
            :model-value="modelValue.masquerade.type"
            style="width: 100%"
            @update:model-value="(v: any) => updateMasq('type', v)"
          >
            <el-option label="404（默认，无伪装）" value="404" />
            <el-option label="file（本地静态目录）" value="file" />
            <el-option label="proxy（反代到目标站点）" value="proxy" />
            <el-option label="string（固定响应内容）" value="string" />
          </el-select>
          <div class="field-hint">非 Hysteria 请求（如浏览器直接访问）返回的内容；不影响代理连接</div>
        </el-form-item>

        <el-form-item v-if="modelValue.masquerade.type === 'file'" label="静态目录 (dir)">
          <el-input
            :model-value="modelValue.masquerade.dir"
            placeholder="/var/www/html"
            class="font-mono-num"
            @input="(v: string) => updateMasq('dir', v)"
          />
        </el-form-item>

        <template v-if="modelValue.masquerade.type === 'proxy'">
          <el-form-item label="目标 URL (url)">
            <el-input
              :model-value="modelValue.masquerade.url"
              placeholder="https://example.com 或 unix:/run/app.sock"
              class="font-mono-num"
              @input="(v: string) => updateMasq('url', v)"
            />
            <div class="field-hint">支持 http/https；unix: 开头走本地 Unix Socket</div>
          </el-form-item>
          <el-form-item label="反代选项">
            <div class="inline-row">
              <el-checkbox
                :model-value="modelValue.masquerade.rewriteHost"
                @update:model-value="(v: any) => updateMasq('rewriteHost', Boolean(v))"
              >改写 Host</el-checkbox>
              <el-checkbox
                :model-value="modelValue.masquerade.xForwarded"
                @update:model-value="(v: any) => updateMasq('xForwarded', Boolean(v))"
              >追加 X-Forwarded</el-checkbox>
              <el-checkbox
                :model-value="modelValue.masquerade.insecure"
                @update:model-value="(v: any) => updateMasq('insecure', Boolean(v))"
              >跳过证书校验</el-checkbox>
            </div>
          </el-form-item>
        </template>

        <template v-if="modelValue.masquerade.type === 'string'">
          <el-form-item label="响应内容 (content)">
            <el-input
              :model-value="modelValue.masquerade.content"
              type="textarea"
              :rows="3"
              placeholder="例如一段 HTML"
              class="font-mono-num"
              @input="(v: string) => updateMasq('content', v)"
            />
          </el-form-item>
          <el-form-item label="状态码 (statusCode)">
            <el-input-number
              :model-value="modelValue.masquerade.statusCode"
              :min="100"
              :max="599"
              :step="1"
              placeholder="默认 200"
              style="width: 100%"
              class="font-mono-num"
              @update:model-value="(v: any) => updateMasq('statusCode', v == null ? undefined : Number(v))"
            />
          </el-form-item>
        </template>
      </div>
      <div v-if="modelValue.masquerade.type !== '404'" class="compact-notice">
        伪装响应只对「未通过 Hysteria 认证的普通 HTTP 请求」生效；代理流量不受影响。
      </div>
    </div>

    <div v-if="errors.length" class="form-errors">
      <div v-for="e in errors" :key="e" class="form-errors__item">
        <el-icon><WarningFilled /></el-icon>
        <span>{{ e }}</span>
      </div>
    </div>

    <!-- 紧凑轻量提示 -->
    <div class="compact-notice">
      <span>提示：Hysteria 2 采用 UDP (QUIC) 传输并强制 TLS 加密，账号由绑定的接入点动态提供。请在「安全层」绑定有效证书。</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.hysteria-settings-box {
  display: flex;
  flex-direction: column;
}

.sub-card {
  background: var(--x-fill-2, #f8fafc);
  border: 1px solid var(--x-border-light, #e5e7eb);
  border-radius: var(--x-radius-sm, 8px);
  padding: 12px 14px 4px;
  margin-bottom: 12px;

  &:last-of-type {
    margin-bottom: 8px;
  }
}

.sub-card__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-text-2, #374151);
  margin-bottom: 10px;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.bg-sky-500 {
  background: #0ea5e9;
}

.bg-rose-500 {
  background: #f43f5e;
}

.inline-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 14px;
}

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 4px;
  line-height: 1.4;
}

.compact-notice {
  font-size: 11.5px;
  color: var(--x-text-3);
  line-height: 1.5;
  background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px dashed var(--x-border-light, #e5e7eb);
  margin-top: 4px;
  margin-bottom: 8px;
}

.form-errors {
  margin-bottom: 10px;
  padding: 8px 12px;
  border-radius: 6px;
  background: rgba(245, 108, 108, 0.08);
  border: 1px dashed rgba(245, 108, 108, 0.45);
}

.form-errors__item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 11.5px;
  color: #d93026;
  line-height: 1.5;
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
