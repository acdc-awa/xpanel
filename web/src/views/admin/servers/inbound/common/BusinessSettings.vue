<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsButton,
  CsContainer,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsSelect,
  CsTooltip,
  ElMessage,
} from '@/components/cloudscape'
import type { AccessLayer } from '@/api/types'

const props = defineProps<{
  inboundType: string
  layerId: number
  layers: AccessLayer[]
  serverId?: number
  shareStrategy: string
  shareAddr: string
  sharePort: number
  shareSecurity: string
  shareSni: string
  shareHost: string
  sharePath: string
  shareAllowInsecure: boolean
  caddySnippet: string
  layerDeleting?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:layerId', value: number): void
  (e: 'update:shareStrategy', value: string): void
  (e: 'update:shareAddr', value: string): void
  (e: 'update:sharePort', value: number): void
  (e: 'update:shareSecurity', value: string): void
  (e: 'update:shareSni', value: string): void
  (e: 'update:shareHost', value: string): void
  (e: 'update:sharePath', value: string): void
  (e: 'update:shareAllowInsecure', value: boolean): void
  (e: 'open-layer-quick'): void
  (e: 'delete-layer'): void
  (e: 'apply-caddy-preset'): void
}>()

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}

const layerOptions = computed(() => [
  { label: '不使用（直连端点）', value: 0 },
  ...props.layers.map((layer) => ({
    label: `${layer.name} (${layer.host}:${layer.port} ${layer.security === 'tls' ? 'TLS' : '明文'})`,
    value: layer.id,
  })),
])

const shareStrategyOptions = [
  { label: '跟随服务器 IP / 域名', value: 'node' },
  { label: '自定义域名或 IP', value: 'custom' },
]

const shareSecurityOptions = [
  { label: 'auto (跟随传输层配置)', value: 'auto' },
  { label: 'tls (强制要求 TLS)', value: 'tls' },
  { label: 'none (明文传输)', value: 'none' },
]
</script>

<template>
  <div class="business-settings-box">
    <!-- 微卡片 1: 对外接入层绑定 -->
    <CsContainer header="对外接入层绑定 (Access Layer)">
      <template #info>
        <CsTooltip content="用于向客户端分发订阅时解析的连接地址与参数。若挂载对外接入层，则 host/port/安全层由该层决议。">
          <CsIcon name="status-info" />
        </CsTooltip>
      </template>
      <template #actions>
        <CsButton variant="primary" icon-name="gen-ai" :disabled="!serverId" @click="emit('apply-caddy-preset')">
          应用前置反代预设
        </CsButton>
      </template>
      <CsFormItem label="挂载接入层">
        <div class="layer-row">
          <CsSelect
            :model-value="layerId || undefined"
            :options="layerOptions"
            :disabled="!serverId"
            placeholder="选择对外层（直连请留空）"
            clearable
            style="flex: 1; min-width: 240px"
            @update:model-value="(v: any) => emit('update:layerId', Number(v || 0))"
          />
          <CsButton icon-name="add-plus" :disabled="!serverId" @click="emit('open-layer-quick')">新建对外层</CsButton>
          <CsButton
            icon-name="remove"
            variant="danger-outline"
            :disabled="!serverId || !layerId"
            :loading="layerDeleting"
            @click="emit('delete-layer')"
          >
            删除该层
          </CsButton>
        </div>
      </CsFormItem>
      <CsAlert v-if="layerId > 0" type="success" class="alert-gap">
        已挂载对外接入层：订阅中的连接 host、port 和安全层将自动取自该层。下方自定义地址组已自动失效，仅 SNI/Path 等覆写项继续生效。
      </CsAlert>
    </CsContainer>

    <!-- 微卡片 2: 订阅下发覆写与自定义参数 -->
    <CsContainer header="订阅下发参数覆写 (Subscription Overrides)" class="section-gap">
      <template #actions>
        <span class="cs-text-secondary">下发至客户端的连接配置</span>
      </template>
      <div class="form-grid-2">
        <template v-if="!layerId">
          <CsFormItem label="分享地址策略">
            <CsSelect
              :model-value="shareStrategy"
              :options="shareStrategyOptions"
              style="width: 100%"
              @update:model-value="(v: any) => emit('update:shareStrategy', String(v))"
            />
          </CsFormItem>

          <CsFormItem label="订阅安全层 (TLS)">
            <CsSelect
              :model-value="shareSecurity"
              :options="shareSecurityOptions"
              style="width: 100%"
              @update:model-value="(v: any) => emit('update:shareSecurity', String(v))"
            />
          </CsFormItem>

          <template v-if="shareStrategy === 'custom'">
            <CsFormItem label="自定义分享地址 (Host)">
              <CsInput
                :model-value="shareAddr"
                placeholder="如 cdn.example.com"
                class="mono"
                @update:model-value="(v: string) => emit('update:shareAddr', v)"
              />
            </CsFormItem>
            <CsFormItem label="自定义分享端口 (Port)">
              <CsInputNumber
                :model-value="sharePort"
                :min="0"
                :max="65535"
                placeholder="0 = 默认入站端口"
                class="mono"
                style="width: 100%"
                @update:model-value="(v: number | null) => emit('update:sharePort', Number(v || 0))"
              />
            </CsFormItem>
          </template>
        </template>

        <CsFormItem label="客户端覆写 SNI">
          <CsInput
            :model-value="shareSni"
            placeholder="留空默认跟随服务器配置"
            class="mono"
            @update:model-value="(v: string) => emit('update:shareSni', v)"
          />
        </CsFormItem>

        <CsFormItem label="客户端覆写 HTTP Host">
          <CsInput
            :model-value="shareHost"
            placeholder="留空默认跟随传输配置"
            class="mono"
            @update:model-value="(v: string) => emit('update:shareHost', v)"
          />
        </CsFormItem>

        <CsFormItem label="客户端覆写分流 Path">
          <CsInput
            :model-value="sharePath"
            placeholder="留空默认跟随传输路径"
            class="mono"
            @update:model-value="(v: string) => emit('update:sharePath', v)"
          />
        </CsFormItem>

        <CsFormItem label="客户端证书校验">
          <div class="toggle-pad">
            <CsToggle
              :model-value="shareAllowInsecure"
              label="跳过客户端证书校验 (allowInsecure)"
              @update:model-value="(v: boolean) => emit('update:shareAllowInsecure', v)"
            />
          </div>
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 微卡片 3: Caddyfile 片段参考 -->
    <CsContainer v-if="caddySnippet" header="Caddyfile 反向代理配置参考" class="section-gap">
      <template #actions>
        <CsButton variant="link" icon-name="copy" @click="copyText(caddySnippet, 'Caddyfile 配置')">
          复制 Caddyfile 片段
        </CsButton>
      </template>
      <pre class="caddy-code"><code>{{ caddySnippet }}</code></pre>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.business-settings-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.alert-gap {
  margin-top: 10px;
}

.layer-row {
  display: flex;
  gap: 10px;
  width: 100%;
  align-items: center;
  flex-wrap: wrap;
}

.toggle-pad {
  padding-top: 4px;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.caddy-code {
  background: var(--color-background-input-default-1u0v1p, #ffffff);
  color: var(--color-text-body-default-b2us04, #0f141a);
  border: 1px solid var(--color-border-input-default-jtkw9d, #8c8c94);
  border-radius: 8px;
  padding: 12px 14px;
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 1.5;
  overflow-x: auto;
  margin: 0;
}
</style>
