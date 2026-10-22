<script setup lang="ts">
import { computed, ref } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'
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

const openSections = ref<string[]>([])
const errors = computed(() => validateXhttp(props.modelValue))

const placementOptions = [
  { label: 'path（路径片段）', value: 'path' },
  { label: 'query（查询参数）', value: 'query' },
  { label: 'header（请求头）', value: 'header' },
  { label: 'cookie（Cookie）', value: 'cookie' },
]

const paddingPlacementOptions = [
  { label: 'queryInHeader（查询串放进请求头）', value: 'queryInHeader' },
  { label: 'query（查询参数）', value: 'query' },
  { label: 'header（请求头）', value: 'header' },
  { label: 'cookie（Cookie）', value: 'cookie' },
]

const paddingMethodOptions = [
  { label: 'repeat-x（重复字符）', value: 'repeat-x' },
  { label: 'tokenish（类 token 随机串）', value: 'tokenish' },
]
</script>

<template>
  <div class="x-micro-card">
    <div class="x-micro-card__head">
      <div class="head-title">
        <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
        <span>xhttp (SplitHTTP) 传输流配置</span>
      </div>
      <div class="head-meta">
        新一代多路分包传输
      </div>
    </div>
    <div class="x-micro-card__body">
      <div class="form-grid-2">
        <el-form-item label="传输模式 (Mode)">
          <el-select
            :model-value="modelValue.mode"
            style="width: 100%"
            @update:model-value="(v: 'auto' | 'packet-up' | 'stream-up' | 'stream-one') => updateField('mode', v)"
          >
            <el-option label="auto (自动协商)" value="auto" />
            <el-option label="packet-up (数据报上行)" value="packet-up" />
            <el-option label="stream-up (流式上行)" value="stream-up" />
            <el-option label="stream-one (单向全双工流)" value="stream-one" />
          </el-select>
        </el-form-item>

        <el-form-item label="分流 Path 路径">
          <el-input
            :model-value="modelValue.path"
            placeholder="/xhttp-stream"
            class="font-mono-num"
            @input="(v: string) => updateField('path', v)"
          />
        </el-form-item>

        <el-form-item label="Host 域名标头 (可选)">
          <el-input
            :model-value="modelValue.host"
            placeholder="留空默认"
            class="font-mono-num"
            @input="(v: string) => updateField('host', v)"
          />
        </el-form-item>
      </div>

      <el-collapse v-model="openSections" class="xhttp-advanced">
        <!-- 会话与填充：都是两端共享参数，服务端与订阅必须一致 -->
        <el-collapse-item name="session" title="会话 ID 与填充（两端共享）">
          <div class="form-grid-2">
            <el-form-item label="会话 ID 位置 (sessionIDPlacement)">
              <el-select
                :model-value="modelValue.sessionIDPlacement"
                style="width: 100%"
                @update:model-value="(v: any) => updateField('sessionIDPlacement', v)"
              >
                <el-option label="默认 (path)" value="" />
                <el-option v-for="o in placementOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>

            <el-form-item label="会话 ID 键名 (sessionIDKey)">
              <el-input
                :model-value="modelValue.sessionIDKey"
                placeholder="留空按位置取默认（如 x_session / X-Session）"
                class="font-mono-num"
                @input="(v: string) => updateField('sessionIDKey', v)"
              />
            </el-form-item>

            <el-form-item label="会话 ID 长度区间 (sessionIDLength)">
              <el-input
                :model-value="modelValue.sessionIDLength"
                placeholder="如 8-16，留空默认"
                class="font-mono-num"
                @input="(v: string) => updateField('sessionIDLength', v)"
              />
              <div class="field-hint">区间必须是字符串或整数，不能写成对象</div>
            </el-form-item>

            <el-form-item label="会话 ID 字符表 (sessionIDTable)">
              <el-input
                :model-value="modelValue.sessionIDTable"
                placeholder="留空默认；可填预置表名或自定义 ASCII 字符"
                class="font-mono-num"
                @input="(v: string) => updateField('sessionIDTable', v)"
              />
            </el-form-item>

            <el-form-item label="填充长度区间 (xPaddingBytes)">
              <el-input
                :model-value="modelValue.xPaddingBytes"
                placeholder="如 100-1000，留空不填充"
                class="font-mono-num"
                @input="(v: string) => updateField('xPaddingBytes', v)"
              />
              <div class="field-hint">xray 不允许把填充区间设为 0</div>
            </el-form-item>

            <el-form-item label="填充混淆模式 (xPaddingObfsMode)">
              <div style="padding-top: 4px">
                <el-switch
                  :model-value="modelValue.paddingObfsMode"
                  active-text="对填充内容做混淆"
                  @update:model-value="(v: any) => updateField('paddingObfsMode', Boolean(v))"
                />
              </div>
            </el-form-item>

            <el-form-item label="填充键名 (xPaddingKey)">
              <el-input
                :model-value="modelValue.paddingKey"
                placeholder="默认 x_padding"
                class="font-mono-num"
                @input="(v: string) => updateField('paddingKey', v)"
              />
            </el-form-item>

            <el-form-item label="填充请求头 (xPaddingHeader)">
              <el-input
                :model-value="modelValue.paddingHeader"
                placeholder="默认 X-Padding"
                class="font-mono-num"
                @input="(v: string) => updateField('paddingHeader', v)"
              />
            </el-form-item>

            <el-form-item label="填充位置 (xPaddingPlacement)">
              <el-select
                :model-value="modelValue.paddingPlacement"
                style="width: 100%"
                @update:model-value="(v: any) => updateField('paddingPlacement', v)"
              >
                <el-option label="默认 (queryInHeader)" value="" />
                <el-option v-for="o in paddingPlacementOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>

            <el-form-item label="填充生成方式 (xPaddingMethod)">
              <el-select
                :model-value="modelValue.paddingMethod"
                style="width: 100%"
                @update:model-value="(v: any) => updateField('paddingMethod', v)"
              >
                <el-option label="默认 (repeat-x)" value="" />
                <el-option v-for="o in paddingMethodOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>
          </div>
        </el-collapse-item>

        <!-- 服务端缓冲：只影响节点上的监听器 -->
        <el-collapse-item name="server" title="服务端缓冲（仅节点生效）">
          <div class="form-grid-2">
            <el-form-item label="关闭 SSE 响应头 (noSSEHeader)">
              <div style="padding-top: 4px">
                <el-switch
                  :model-value="modelValue.noSSEHeader"
                  active-text="不发送 SSE 头"
                  @update:model-value="(v: any) => updateField('noSSEHeader', Boolean(v))"
                />
              </div>
              <div class="field-hint">前置 CDN / 反代对 SSE 头敏感时可关闭</div>
            </el-form-item>

            <el-form-item label="最大缓冲请求数 (scMaxBufferedPosts)">
              <el-input-number
                :model-value="modelValue.scMaxBufferedPosts"
                :min="0"
                :max="65536"
                :step="16"
                placeholder="留空为内核默认"
                style="width: 100%"
                class="font-mono-num"
                @update:model-value="(v: any) => updateField('scMaxBufferedPosts', v == null ? undefined : Number(v))"
              />
            </el-form-item>

            <el-form-item label="上行流保持时长 (scStreamUpServerSecs)">
              <el-input
                :model-value="modelValue.scStreamUpServerSecs"
                placeholder="如 20-80 或单个整数，留空默认"
                class="font-mono-num"
                @input="(v: string) => updateField('scStreamUpServerSecs', v)"
              />
            </el-form-item>

            <el-form-item label="最大请求头字节 (serverMaxHeaderBytes)">
              <el-input-number
                :model-value="modelValue.serverMaxHeaderBytes"
                :min="0"
                :max="1048576"
                :step="1024"
                placeholder="留空为内核默认"
                style="width: 100%"
                class="font-mono-num"
                @update:model-value="(v: any) => updateField('serverMaxHeaderBytes', v == null ? undefined : Number(v))"
              />
            </el-form-item>
          </div>
        </el-collapse-item>

        <!-- xmux：客户端复用策略，服务端不消费但订阅/互联必须同源 -->
        <el-collapse-item name="xmux" title="多路复用 xmux（下发客户端）">
          <div style="padding-top: 4px; margin-bottom: 12px">
            <el-switch
              :model-value="modelValue.xmuxEnabled"
              active-text="显式写入复用参数"
              @update:model-value="(v: any) => updateField('xmuxEnabled', Boolean(v))"
            />
            <div class="field-hint">
              不启用时交给内核默认（maxConnections=3，hMaxRequestTimes=600-900，hMaxReusableSecs=1800-3000）；
              一旦显式写入，必须整组填全，否则未填项会变成 0（不限制）。
            </div>
          </div>
          <div v-if="modelValue.xmuxEnabled" class="form-grid-2">
            <el-form-item label="并发连接 maxConnections">
              <el-input
                :model-value="modelValue.xmuxMaxConnections"
                placeholder="默认 3"
                class="font-mono-num"
                @input="(v: string) => updateField('xmuxMaxConnections', v)"
              />
            </el-form-item>

            <el-form-item label="并发流 maxConcurrency">
              <el-input
                :model-value="modelValue.xmuxMaxConcurrency"
                placeholder="留空；与 maxConnections 互斥"
                class="font-mono-num"
                @input="(v: string) => updateField('xmuxMaxConcurrency', v)"
              />
              <div class="field-hint">maxConnections 与 maxConcurrency 只能填一个（两边同时 &gt;0 会被 xray 拒绝）</div>
            </el-form-item>

            <el-form-item label="单连接复用次数 hMaxRequestTimes">
              <el-input
                :model-value="modelValue.xmuxMaxRequestTimes"
                placeholder="默认 600-900"
                class="font-mono-num"
                @input="(v: string) => updateField('xmuxMaxRequestTimes', v)"
              />
            </el-form-item>

            <el-form-item label="连接可复用秒数 hMaxReusableSecs">
              <el-input
                :model-value="modelValue.xmuxMaxReusableSecs"
                placeholder="默认 1800-3000"
                class="font-mono-num"
                @input="(v: string) => updateField('xmuxMaxReusableSecs', v)"
              />
            </el-form-item>
          </div>
        </el-collapse-item>
      </el-collapse>

      <div v-if="errors.length" class="form-errors">
        <div v-for="e in errors" :key="e" class="form-errors__item">
          <el-icon><WarningFilled /></el-icon>
          <span>{{ e }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
:deep(.el-form-item) {
  margin-bottom: 0;
}

.xhttp-advanced {
  margin-top: 16px;
  border-top: 1px dashed var(--x-border-light, #e5e7eb);
  border-bottom: none;

  :deep(.el-collapse-item__header) {
    font-size: 12.5px;
    font-weight: 600;
    color: var(--x-text-2, #374151);
    background: transparent;
  }

  :deep(.el-collapse-item__content) {
    padding-bottom: 8px;
  }

  :deep(.el-collapse-item__wrap) {
    background: transparent;
  }
}

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 4px;
  line-height: 1.35;
}

.form-errors {
  margin-top: 12px;
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
</style>
