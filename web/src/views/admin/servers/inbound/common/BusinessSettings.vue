<script setup lang="ts">
import { computed } from 'vue'
import {
  CsButton,
  CsContainer,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsKeyValuePairs,
  CsSelect,
  ElMessage,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import type { AccessLayer } from '@/api/types'

const props = defineProps<{
  /** 前置 TLS 卸载只对 vless（xhttp 传输）成立；Hysteria/直通显示该按钮只会建出无关的层。 */
  canOffload: boolean
  layerId: number
  layers: AccessLayer[]
  serverId?: number
  shareStrategy: string
  shareAddr: string
  sharePort: number
  shareSni: string
  /** 入站自身的传输安全层：不挂接入层时它就是对外安全层 */
  transportSecurity?: string
  caddySnippet: string
  layerDeleting?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:layerId', value: number): void
  (e: 'update:shareStrategy', value: string): void
  (e: 'update:shareAddr', value: string): void
  (e: 'update:sharePort', value: number): void
  (e: 'update:shareSni', value: string): void
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

const boundLayer = computed(() => props.layers.find((l) => l.id === props.layerId) || null)

// 与后端 subscribe.BuildNodeDTO 的取值顺序完全一致：
// 接入层 > 入站自定义地址 > 服务器 Host；分享安全层显式值再覆盖层定义。
const effectiveEndpoint = computed(() => {
  const layer = boundLayer.value
  if (layer) return `${layer.host}:${layer.port}`
  const port = props.sharePort > 0 ? String(props.sharePort) : '入站端口'
  if (props.shareStrategy === 'custom' && props.shareAddr.trim()) {
    return `${props.shareAddr.trim()}:${port}`
  }
  return `服务器主机名:${port}`
})

// 对外安全层不再是可填字段：挂层取层定义，不挂层就是入站传输安全层。
const effectiveSecurity = computed(() => {
  const layer = boundLayer.value
  if (layer) return `${layer.security}（接入层）`
  return props.transportSecurity ? `${props.transportSecurity}（入站传输层）` : '跟随传输层'
})

const effectiveSni = computed(() => {
  if (props.shareSni.trim()) return props.shareSni.trim()
  const layer = boundLayer.value
  return layer ? `${layer.host}（接入层）` : '跟随传输层'
})

// 与后端取值顺序同源的三项结果：纯展示，不再用横幅抢视觉
const endpointItems = computed(() => [
  { key: 'endpoint', label: '下发地址', value: effectiveEndpoint.value },
  { key: 'security', label: '安全层', value: effectiveSecurity.value },
  { key: 'sni', label: 'SNI', value: effectiveSni.value },
])

const shareStrategyOptions = [
  { label: '跟随服务器 IP / 域名', value: 'node' },
  { label: '自定义域名或 IP', value: 'custom' },
]

</script>

<template>
  <div class="business-settings-box">
    <!-- 订阅对外入口：接入层（可复用）> 自定义地址 > 服务器主机名，三级取值在同处展示 -->
    <CsContainer header="订阅对外入口 (Entry Endpoint)">
      <template #info>
        <TipIcon content="下发到客户端的地址按「接入层 > 入站自定义地址 > 服务器主机名」逐级取值；客户端覆写只改 SNI；Host 与 Path 取传输页的 xhttp 设置，安全层由接入层决议。" />
      </template>
      <template #actions>
        <CsButton
          v-if="canOffload"
          variant="primary"
          icon-name="gen-ai"
          :disabled="!serverId"
          @click="emit('apply-caddy-preset')"
        >
          应用前置反代预设
        </CsButton>
      </template>

      <CsKeyValuePairs class="endpoint-preview" :items="endpointItems" :columns="3" aria-label="当前下发的客户端入口">
        <template #value="{ item }">
          <span :class="item.key === 'endpoint' ? 'mono' : ''">{{ item.value }}</span>
        </template>
      </CsKeyValuePairs>

      <CsFormItem label="对外接入层" class="layer-field">
        <template #info>
          <TipIcon content="接入层是「多入站共用一个对外入口」的实体（典型场景：一个域名 / 反代端口挂多个入站）。挂载后 host 与 port 由该层决议，下方自定义地址组失效。" />
        </template>
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

      <div class="form-grid-2">
        <template v-if="!layerId">
          <CsFormItem label="分享地址策略">
            <template #info>
              <TipIcon content="node = 用服务器主机名；custom = 用下面的自定义地址（只对当前入站生效）。" />
            </template>
            <CsSelect
              :model-value="shareStrategy"
              :options="shareStrategyOptions"
              style="width: 100%"
              @update:model-value="(v: any) => emit('update:shareStrategy', String(v))"
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
          <template #info>
            <TipIcon content="只在 CDN 前置、订阅域名与传输层 serverName 不一致时填写；其余留空，自动跟随接入层 Host 或传输层。不改 Host 与 Path。" />
          </template>
          <CsInput
            :model-value="shareSni"
            placeholder="留空跟随接入层 / 传输层"
            class="mono"
            @update:model-value="(v: string) => emit('update:shareSni', v)"
          />
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 微卡片 2: Caddyfile 片段参考 -->
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

.layer-field {
  margin-top: 12px;
}

.endpoint-preview {
  margin-bottom: 4px;
}

.layer-row {
  display: flex;
  gap: 10px;
  width: 100%;
  align-items: center;
  flex-wrap: wrap;
}

code {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
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
