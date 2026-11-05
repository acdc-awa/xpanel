<script setup lang="ts">
import { computed } from 'vue'
import {
  CsAlert,
  CsButton,
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

// packet-up 专属参数：服务端 auto 三种全收，因此 auto 下也要能配。
const showPacketUp = computed(() => props.modelValue.mode === 'packet-up' || props.modelValue.mode === 'auto')
// 流式上行参数：packet-up 不产生上行流，隐藏即可（生成侧也按模式裁剪）。
const showStreamUp = computed(
  () => props.modelValue.mode === 'stream-up' || props.modelValue.mode === 'stream-one' || props.modelValue.mode === 'auto',
)

// extra 一旦存在，xray 只认 host/path/mode 三个外层键，其余全部来自 extra。
const extraActive = computed(() => {
  const extra = props.modelValue.rawExtra?.extra
  return !!extra && typeof extra === 'object' && Object.keys(extra).length > 0
})

const modeOptions = [
  { label: 'auto（自动）', value: 'auto' },
  { label: 'packet-up（分包上行）', value: 'packet-up' },
  { label: 'stream-up（流式上行）', value: 'stream-up' },
  { label: 'stream-one（单请求全双工）', value: 'stream-one' },
]

const placementOptions = [
  { label: 'path（路径片段）', value: 'path' },
  { label: 'query（查询参数）', value: 'query' },
  { label: 'header（请求头）', value: 'header' },
  { label: 'cookie（Cookie）', value: 'cookie' },
]

const sessionPlacementOptions = [{ label: '默认 (path)', value: '' }, ...placementOptions]

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

// GET 只允许 packet-up，其它模式写了会让 xray 拒绝整份配置，所以选项按当前模式门控。
const uplinkMethodOptions = computed(() => {
  const opts = [
    { label: '默认 (POST)', value: '' },
    { label: 'POST', value: 'POST' },
    { label: 'PUT', value: 'PUT' },
    { label: 'PATCH', value: 'PATCH' },
  ]
  if (props.modelValue.mode === 'packet-up') opts.push({ label: 'GET（仅 packet-up）', value: 'GET' })
  return opts
})

const uplinkDataPlacementOptions = computed(() => {
  const opts = [
    { label: '默认 (auto)', value: '' },
    { label: 'body（放请求体）', value: 'body' },
  ]
  // header/cookie 仅 packet-up 允许。
  if (props.modelValue.mode === 'packet-up') {
    opts.push({ label: 'header（拆分进请求头）', value: 'header' })
    opts.push({ label: 'cookie（拆分进 Cookie）', value: 'cookie' })
  }
  return opts
})

const headers = computed(() => props.modelValue.headers ?? [])

function setHeader(index: number, field: 'key' | 'value', val: string) {
  const next = headers.value.map((h, i) => (i === index ? { ...h, [field]: val } : h))
  updateField('headers', next)
}

function addHeader() {
  updateField('headers', [...headers.value, { key: '', value: '' }])
}

function removeHeader(index: number) {
  updateField('headers', headers.value.filter((_, i) => i !== index))
}
</script>

<template>
  <CsContainer header="xhttp (SplitHTTP) 传输流配置">
    <CsAlert v-if="extraActive" type="warning" class="alert-gap">
      已配置 <code>extra</code>：xray 只保留外层的 mode / path / host，其余参数全部以 extra 为准，下方表单里的同类字段不会生效。
    </CsAlert>

    <div class="form-grid-2">
      <CsFormItem label="传输模式 (Mode)">
        <template #info>
          <TipIcon
            content="服务端 auto 同时接受三种模式；选具体模式则只接受它（stream-up 额外接受 stream-one）。客户端 auto 的实际选择：REALITY 用 stream-one（配了 downloadSettings 则 stream-up），其余（含 TLS H2）用 packet-up。"
          />
        </template>
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

    <!-- packet-up：分包上行。服务端只消费 scMaxEachPostBytes / scMaxBufferedPosts。 -->
    <CsExpandableSection v-if="showPacketUp" header="分包上行 packet-up" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="单 POST 上限 (scMaxEachPostBytes)">
          <template #info>
            <TipIcon content="客户端每个 POST 最多携带的字节数，服务端也按它拒绝超限请求。默认 1000000（1MB），应小于 CDN / 反代允许的最大请求体；可写区间如 500000-1000000 降低指纹。" />
          </template>
          <CsInput
            :model-value="modelValue.scMaxEachPostBytes"
            placeholder="默认 1000000，或 500000-1000000"
            class="mono"
            @update:model-value="(v: string) => updateField('scMaxEachPostBytes', v)"
          />
        </CsFormItem>

        <CsFormItem label="上发最小间隔 (scMinPostsIntervalMs)">
          <template #info>
            <TipIcon content="仅客户端：同一个代理请求下两个 POST 的最小间隔。默认 30 毫秒，可写区间如 10-50。" />
          </template>
          <CsInput
            :model-value="modelValue.scMinPostsIntervalMs"
            placeholder="默认 30，或 10-50"
            class="mono"
            @update:model-value="(v: string) => updateField('scMinPostsIntervalMs', v)"
          />
        </CsFormItem>

        <CsFormItem label="服务端缓冲请求数 (scMaxBufferedPosts)">
          <template #info>
            <TipIcon content="仅节点生效：同一代理请求下最多缓存多少个乱序 POST，超限断连。默认 30。" />
          </template>
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

        <CsFormItem label="上行 HTTP 方法 (uplinkHTTPMethod)">
          <template #info>
            <TipIcon content="换成带请求体的方法可减少特征；GET 仅 packet-up 支持，其它模式写了会让 xray 拒绝整份配置。" />
          </template>
          <CsSelect
            :model-value="modelValue.uplinkHTTPMethod"
            :options="uplinkMethodOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('uplinkHTTPMethod', v)"
          />
        </CsFormItem>

        <CsFormItem label="序列号位置 (seqPlacement)">
          <CsInput
            :model-value="modelValue.seqPlacement"
            placeholder="留空默认 path"
            class="mono"
            @update:model-value="(v: string) => updateField('seqPlacement', v as XhttpConfig['seqPlacement'])"
          />
        </CsFormItem>

        <CsFormItem label="序列号键名 (seqKey)">
          <CsInput
            :model-value="modelValue.seqKey"
            placeholder="留空按位置取默认（如 x_seq / X-Seq）"
            class="mono"
            @update:model-value="(v: string) => updateField('seqKey', v)"
          />
        </CsFormItem>

        <CsFormItem label="上行数据位置 (uplinkDataPlacement)">
          <template #info>
            <TipIcon content="把上行数据拆进请求头 / Cookie 可进一步伪装；header 与 cookie 仅 packet-up 支持，非 packet-up 模式写了会让 xray 拒绝整份配置。" />
          </template>
          <CsSelect
            :model-value="modelValue.uplinkDataPlacement"
            :options="uplinkDataPlacementOptions"
            style="width: 100%"
            @update:model-value="(v: any) => updateField('uplinkDataPlacement', v)"
          />
        </CsFormItem>

        <CsFormItem label="上行数据键名 (uplinkDataKey)">
          <CsInput
            :model-value="modelValue.uplinkDataKey"
            placeholder="留空按位置取默认（如 X-Data / x_data）"
            class="mono"
            @update:model-value="(v: string) => updateField('uplinkDataKey', v)"
          />
        </CsFormItem>

        <CsFormItem label="上行分片大小 (uplinkChunkSize)">
          <template #info>
            <TipIcon content="仅客户端：数据拆到 header / cookie 时每片的最大字节，最小值 64。官方建议 cookie 用 3072、header 用 4096 一带；可写区间。" />
          </template>
          <CsInput
            :model-value="modelValue.uplinkChunkSize"
            placeholder="如 3072 或 4096-8192"
            class="mono"
            @update:model-value="(v: string) => updateField('uplinkChunkSize', v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <!-- stream-up / stream-one：流式上行。 -->
    <CsExpandableSection v-if="showStreamUp" header="流式上行 stream-up / stream-one" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="关闭 gRPC 伪装头 (noGRPCHeader)">
          <template #info>
            <TipIcon content="stream-up / stream-one 的上行默认带 Content-Type: application/grpc 伪装成 gRPC（穿 CF 需要它）；打开本开关则去掉该头。" />
          </template>
          <div class="control-cell">
            <CsToggle
              :model-value="modelValue.noGRPCHeader"
              label="不发送 gRPC 内容类型头"
              @update:model-value="(v: boolean) => updateField('noGRPCHeader', v)"
            />
          </div>
        </CsFormItem>

        <CsFormItem label="上行保活间隔 (scStreamUpServerSecs)">
          <template #info>
            <TipIcon content="仅节点生效：上行流每隔这段时间发一次填充字节保活（CF 会掐断 100 秒无数据的下行）。默认 20-80 秒；填 -1 关闭。" />
          </template>
          <CsInput
            :model-value="modelValue.scStreamUpServerSecs"
            placeholder="默认 20-80；-1 关闭"
            class="mono"
            @update:model-value="(v: string) => updateField('scStreamUpServerSecs', v)"
          />
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <CsExpandableSection header="会话标识（两端共享）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="会话 ID 位置 (sessionIDPlacement)">
          <template #info>
            <TipIcon content="会话 ID 放在请求的哪个部位；两端必须一致。stream-one 不产生会话 ID。" />
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
            <TipIcon content="必须写成字符串「8-16」或单个整数；写成对象会让 xray 拒绝整份配置。填了字符表时长度起点必须大于 0。" />
          </template>
          <CsInput
            :model-value="modelValue.sessionIDLength"
            placeholder="如 8-16，留空默认"
            class="mono"
            @update:model-value="(v: string) => updateField('sessionIDLength', v)"
          />
        </CsFormItem>

        <CsFormItem label="会话 ID 字符表 (sessionIDTable)">
          <template #info>
            <TipIcon content="可填预置表名（ALPHABET / Alphabet / BASE36 / Base62 / HEX / alphabet / base36 / hex / number）或自定义 ASCII 字符。字符表 + 长度必须能生成 ≥ 2^31 个 ID。" />
          </template>
          <CsInput
            :model-value="modelValue.sessionIDTable"
            placeholder="留空默认（UUID）"
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

    <!-- 客户端请求头：服务端不消费，生成侧会剥离。 -->
    <CsExpandableSection header="客户端请求头（headers）" class="section-gap">
      <div v-if="headers.length" class="kv-rows">
        <div v-for="(h, i) in headers" :key="i" class="kv-row">
          <CsInput
            :model-value="h.key"
            placeholder="Header 名称"
            class="mono"
            @update:model-value="(v: string) => setHeader(i, 'key', v)"
          />
          <CsInput
            :model-value="h.value"
            placeholder="值"
            class="mono"
            @update:model-value="(v: string) => setHeader(i, 'value', v)"
          />
          <CsButton variant="icon" icon-name="remove" :aria-label="`删除请求头 ${h.key || i + 1}`" @click="removeHeader(i)" />
        </div>
      </div>
      <CsButton icon-name="add" class="add-header" @click="addHeader">添加请求头</CsButton>
      <div class="hint">Host 必须填在上方的 Host 字段，写进这里会被 xray 拒绝。</div>
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

        <CsFormItem label="复用次数上限 cMaxReuseTimes">
          <template #info>
            <TipIcon content="一条底层连接最多被复用几次；0 或留空表示不限制。" />
          </template>
          <CsInput
            :model-value="modelValue.xmuxCMaxReuseTimes"
            placeholder="默认 0（不限制），或区间"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxCMaxReuseTimes', v)"
          />
        </CsFormItem>

        <CsFormItem label="空闲保活间隔 hKeepAlivePeriod">
          <template #info>
            <TipIcon content="H2/H3 空闲时每隔多少秒发保活包；0 为内核默认（H2 45s / H3 10s），-1 关闭。该值不允许写区间。" />
          </template>
          <CsInput
            :model-value="modelValue.xmuxHKeepAlivePeriod"
            placeholder="默认 0；-1 关闭"
            class="mono"
            @update:model-value="(v: string) => updateField('xmuxHKeepAlivePeriod', v)"
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

.alert-gap {
  margin-bottom: 12px;
}

.xmux-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
}

.kv-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}

.kv-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto;
  gap: 8px;
  align-items: center;
}

.add-header {
  margin-bottom: 4px;
}

.hint {
  font-size: 12px;
  color: var(--color-text-status-inactive-fqjc97);
}

.errors-gap {
  margin-top: 12px;
}
</style>
