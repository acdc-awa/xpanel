<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsContainer,
  CsExpandableSection,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsSelect,
  CsToggle,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import type { XhttpConfig } from '../types'
import { validateXhttp } from '../validate'

const props = defineProps<{
  modelValue: XhttpConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: XhttpConfig): void
}>()

function updateField<K extends keyof XhttpConfig>(key: K, val: XhttpConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

const errors = computed(() => validateXhttp(props.modelValue))

const modeOptions = [
  { label: 'auto (自动协商)', value: 'auto' },
  { label: 'packet-up (数据报上行)', value: 'packet-up' },
  { label: 'stream-up (流式上行)', value: 'stream-up' },
  { label: 'stream-one (单向全双工流)', value: 'stream-one' },
]

const placementOptions = [
  { label: 'path（路径片段）', value: 'path' },
  { label: 'query（查询参数）', value: 'query' },
  { label: 'header（请求头）', value: 'header' },
  { label: 'cookie（Cookie）', value: 'cookie' },
]

const sessionPlacementOptions = [
  { label: '默认 (path)', value: '' },
  ...placementOptions,
]

const paddingPlacementOptions = [
  { label: '默认 (queryInHeader)', value: '' },
  { label: 'queryInHeader（查询串放进请求头）', value: 'queryInHeader' },
  { label: 'query（查询参数）', value: 'query' },
  { label: 'header（请求头）', value: 'header' },
  { label: 'cookie（Cookie）', value: 'cookie' },
]

const paddingMethodOptions = [
  { label: '默认 (repeat-x)', value: '' },
  { label: 'repeat-x（重复字符）', value: 'repeat-x' },
  { label: 'tokenish（类 token 随机串）', value: 'tokenish' },
]
</script>

<template>
  <CsContainer header="xhttp (SplitHTTP) 传输流配置">
    <div class="form-grid-2">
      <CsFormItem label="传输模式 (Mode)">
        <CsSelect
          :model-value="modelValue.mode"
          :options="modeOptions"
          style="width: 100%"
          @update:model-value="(v: any) => updateField('mode', v)"
        />
      </CsFormItem>

      <CsFormItem label="分流 Path 路径">
        <template #info>
          <TipIcon content="反代按此路径把流量分到本端口。xray 用前缀匹配，必须与 Caddy / Nginx 的匹配串一致；同一接入层下不要出现互为前缀的路径。" />
        </template>
        <CsInput
          :model-value="modelValue.path"
          placeholder="/xhttp-stream"
          class="mono"
          @update:model-value="(v: string) => updateField('path', v)"
        />
      </CsFormItem>

      <CsFormItem label="HTTP Host 头 (可选)">
        <template #info>
          <TipIcon content="客户端请求头里的 Host。CDN 前置时填 CDN 域名；留空则由反代透传。订阅与反代必须同值。" />
        </template>
        <CsInput
          :model-value="modelValue.host"
          placeholder="留空由反代透传"
          class="mono"
          @update:model-value="(v: string) => updateField('host', v)"
        />
      </CsFormItem>
    </div>

    <CsExpandableSection header="会话标识（两端共享）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="会话 ID 位置 (sessionIDPlacement)">
          <template #info>
            <TipIcon content="会话 ID 放在请求的哪个部位；两端必须一致。" />
          </template>
          <CsSelect
            :model-value="modelValue.sessionIDPlacement"
            :options="sessionPlacementOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('sessionIDPlacement', v)"
          />
        </CsFormItem>

        <CsFormItem label="会话 ID 键名 (sessionIDKey)">
          <CsInput
            :model-value="modelValue.sessionIDKey"
            placeholder="留空按位置取默认（如 x_session / X-Session）"
            class="mono"
            @update:model-value="(v: string) => updateField('sessionIDKey', v)"
          />
        </CsFormItem>

        <CsFormItem label="会话 ID 长度区间 (sessionIDLength)">
          <template #info>
            <TipIcon content="必须写成字符串「8-16」或单个整数；写成对象会让 xray 拒绝整份配置。" />
          </template>
          <CsInput
            :model-value="modelValue.sessionIDLength"
            placeholder="如 8-16，留空默认"
            class="mono"
            @update:model-value="(v: string) => updateField('sessionIDLength', v)"
          />
        </CsFormItem>

        <CsFormItem label="会话 ID 字符表 (sessionIDTable)">
          <CsInput
            :model-value="modelValue.sessionIDTable"
            placeholder="留空默认；可填预置表名或自定义 ASCII 字符"
            class="mono"
            @update:model-value="(v: string) => updateField('sessionIDTable', v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <CsExpandableSection header="填充与混淆（两端共享）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="填充长度区间 (xPaddingBytes)">
          <template #info>
            <TipIcon content="字符串「a-b」或整数；xray 不允许把填充区间设为 0。留空不填充。" />
          </template>
          <CsInput
            :model-value="modelValue.xPaddingBytes"
            placeholder="如 100-1000，留空不填充"
            class="mono"
            @update:model-value="(v: string) => updateField('xPaddingBytes', v)"
          />
        </CsFormItem>

        <CsFormItem label="填充混淆模式 (xPaddingObfsMode)">
          <div class="control-cell">
            <CsToggle
              :model-value="modelValue.paddingObfsMode"
              label="对填充内容做混淆"
              @update:model-value="(v: boolean) => updateField('paddingObfsMode', v)"
            />
          </div>
        </CsFormItem>

        <CsFormItem label="填充键名 (xPaddingKey)">
          <CsInput
            :model-value="modelValue.paddingKey"
            placeholder="默认 x_padding"
            class="mono"
            @update:model-value="(v: string) => updateField('paddingKey', v)"
          />
        </CsFormItem>

        <CsFormItem label="填充请求头 (xPaddingHeader)">
          <CsInput
            :model-value="modelValue.paddingHeader"
            placeholder="默认 X-Padding"
            class="mono"
            @update:model-value="(v: string) => updateField('paddingHeader', v)"
          />
        </CsFormItem>

        <CsFormItem label="填充位置 (xPaddingPlacement)">
          <CsSelect
            :model-value="modelValue.paddingPlacement"
            :options="paddingPlacementOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('paddingPlacement', v)"
          />
        </CsFormItem>

        <CsFormItem label="填充生成方式 (xPaddingMethod)">
          <CsSelect
            :model-value="modelValue.paddingMethod"
            :options="paddingMethodOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('paddingMethod', v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <!-- 服务端缓冲：只影响节点上的监听器 -->
    <CsExpandableSection header="服务端缓冲（仅节点生效）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="关闭 SSE 响应头 (noSSEHeader)">
          <template #info>
            <TipIcon content="前置 CDN / 反代对 SSE 响应头敏感时可关闭。" />
          </template>
          <div class="control-cell">
            <CsToggle
              :model-value="modelValue.noSSEHeader"
              label="不发送 SSE 头"
              @update:model-value="(v: boolean) => updateField('noSSEHeader', v)"
            />
          </div>
        </CsFormItem>

        <CsFormItem label="最大缓冲请求数 (scMaxBufferedPosts)">
          <CsInputNumber
            :model-value="modelValue.scMaxBufferedPosts"
            :min="0"
            :max="65536"
            :step="16"
            placeholder="留空为内核默认"
            style="width: 100%"
            class="mono"
            @update:model-value="(v: number | null) => updateField('scMaxBufferedPosts', v == null ? undefined : v)"
          />
        </CsFormItem>

        <CsFormItem label="上行流保持时长 (scStreamUpServerSecs)">
          <CsInput
            :model-value="modelValue.scStreamUpServerSecs"
            placeholder="如 20-80 或单个整数，留空默认"
            class="mono"
            @update:model-value="(v: string) => updateField('scStreamUpServerSecs', v)"
          />
        </CsFormItem>

        <CsFormItem label="最大请求头字节 (serverMaxHeaderBytes)">
          <CsInputNumber
            :model-value="modelValue.serverMaxHeaderBytes"
            :min="0"
            :max="1048576"
            :step="1024"
            placeholder="留空为内核默认"
            style="width: 100%"
            class="mono"
            @update:model-value="(v: number | null) => updateField('serverMaxHeaderBytes', v == null ? undefined : v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <!-- xmux：客户端复用策略，服务端不消费但订阅/互联必须同源 -->
    <CsExpandableSection header="多路复用 xmux（下发客户端）" class="section-gap">
      <div class="xmux-toggle">
        <CsToggle
          :model-value="modelValue.xmuxEnabled"
          label="显式写入复用参数"
          @update:model-value="(v: boolean) => updateField('xmuxEnabled', v)"
        />
        <TipIcon
          content="不启用时交给内核默认（maxConnections=3，hMaxRequestTimes=600-900，hMaxReusableSecs=1800-3000）；一旦显式写入，必须整组填全，否则未填项会变成 0（不限制）。"
        />
      </div>
      <div v-if="modelValue.xmuxEnabled" class="form-grid-2">
        <CsFormItem label="并发连接 maxConnections">
          <CsInput
            :model-value="modelValue.xmuxMaxConnections"
            placeholder="默认 3"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxMaxConnections', v)"
          />
        </CsFormItem>

        <CsFormItem label="并发流 maxConcurrency">
          <template #info>
            <TipIcon content="与 maxConnections 只能填一个，两边同时 >0 会被 xray 拒绝。" />
          </template>
          <CsInput
            :model-value="modelValue.xmuxMaxConcurrency"
            placeholder="留空；与 maxConnections 互斥"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxMaxConcurrency', v)"
          />
        </CsFormItem>

        <CsFormItem label="单连接复用次数 hMaxRequestTimes">
          <CsInput
            :model-value="modelValue.xmuxMaxRequestTimes"
            placeholder="默认 600-900"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxMaxRequestTimes', v)"
          />
        </CsFormItem>

        <CsFormItem label="连接可复用秒数 hMaxReusableSecs">
          <CsInput
            :model-value="modelValue.xmuxMaxReusableSecs"
            placeholder="默认 1800-3000"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxMaxReusableSecs', v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <CsAlert v-if="errors.length" type="error" class="errors-gap">
      <div v-for="e in errors" :key="e">{{ e }}</div>
    </CsAlert>
  </CsContainer>
</template>

<style scoped lang="scss">
.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.section-gap {
  margin-top: 12px;
}

.xmux-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
}

.errors-gap {
  margin-top: 12px;
}
</style>
