<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  CsButton,
  CsContainer,
  CsSegmentedControl,
  CsSelect,
  CsSpinner,
  CsToggle,
  ElMessage,
  type SegmentedOption,
} from '@/components/cloudscape'
import { getServerConfigPreview, serverCommand, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'
import { maskUUIDs } from '@/utils/mask'
import TipIcon from '@/components/base/TipIcon.vue'

type DiagnosticMode = 'logs' | 'config'

const props = defineProps<{
  server: ServerItem
  active: boolean
}>()

const modeOptions: SegmentedOption[] = [
  { id: 'logs', text: '运行日志' },
  { id: 'config', text: '配置预览' },
]

const lineOptions = [
  { label: '50 行', value: 50 },
  { label: '100 行', value: 100 },
  { label: '200 行', value: 200 },
  { label: '500 行', value: 500 },
]

const mode = ref<DiagnosticMode>('logs')
const logContent = ref('')
const logLoading = ref(false)
const logLoaded = ref(false)
const logLines = ref(50)
const logReverse = ref(false)
const logPreRef = ref<HTMLPreElement | null>(null)

const cfgText = ref('')
const cfgLoading = ref(false)
const cfgLoaded = ref(false)

const displayedLogContent = computed(() => {
  if (!logContent.value) return '（空）'
  if (!logReverse.value) return logContent.value
  return logContent.value.split('\n').slice().reverse().join('\n')
})

async function scrollToBottom() {
  await nextTick()
  if (logPreRef.value && !logReverse.value) {
    logPreRef.value.scrollTop = logPreRef.value.scrollHeight
  }
}

async function loadLogs() {
  logLoading.value = true
  try {
    const { data } = await serverCommand(props.server.id, 'get_logs', { lines: logLines.value })
    logContent.value = (data.data.data as string) || ''
    await scrollToBottom()
  } catch (e) {
    logContent.value = `读取失败：${errMsg(e)}`
  } finally {
    logLoading.value = false
    logLoaded.value = true
  }
}

async function loadConfig() {
  cfgLoading.value = true
  try {
    const { data } = await getServerConfigPreview(props.server.id)
    if (data.code === 0) cfgText.value = data.data.config || ''
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载配置预览失败'))
  } finally {
    cfgLoading.value = false
    cfgLoaded.value = true
  }
}

function onModeChange(value: string | number) {
  const next = value as DiagnosticMode
  if (next === mode.value) return
  mode.value = next
  if (next === 'config' && !cfgLoaded.value) void loadConfig()
  if (next === 'logs' && !logLoaded.value) void loadLogs()
}

function onLinesChange(value: string | number | (string | number)[] | '') {
  const next = Number(Array.isArray(value) ? value[0] : value)
  if (!next || next === logLines.value) return
  logLines.value = next
  void loadLogs()
}

async function copyLogs() {
  if (!logContent.value) return
  try {
    await navigator.clipboard.writeText(displayedLogContent.value)
    ElMessage.success('日志已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

async function copyConfig() {
  if (!cfgText.value) return
  try {
    await navigator.clipboard.writeText(cfgText.value)
    ElMessage.success('配置已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

watch(
  () => props.active,
  (active) => {
    if (!active) return
    if (mode.value === 'config') {
      if (!cfgLoaded.value) void loadConfig()
    } else if (!logLoaded.value) {
      void loadLogs()
    }
  },
  { immediate: true },
)

watch(
  () => props.server.id,
  () => {
    logContent.value = ''
    cfgText.value = ''
    logLoaded.value = false
    cfgLoaded.value = false
    if (props.active) void loadLogs()
  },
)
</script>

<template>
  <div class="server-tab-stack">
    <div class="server-diag-switch">
      <CsSegmentedControl :model-value="mode" :options="modeOptions" label="诊断内容" @update:model-value="onModeChange" />
    </div>

    <CsContainer v-if="mode === 'logs'" header="运行日志">
      <template #actions>
        <div class="server-diag-actions">
          <CsSelect
            :model-value="logLines"
            :options="lineOptions"
            class="server-diag-lines"
            aria-label="日志行数"
            @update:model-value="onLinesChange"
          />
          <CsToggle v-model="logReverse" label="最新在最前" />
          <CsButton icon-name="refresh" :loading="logLoading" @click="loadLogs">刷新</CsButton>
          <CsButton icon-name="copy" :disabled="!logContent" @click="copyLogs">复制</CsButton>
        </div>
      </template>

      <div v-if="logLoading && !logContent" class="server-diag-loading">
        <CsSpinner size="large" />
      </div>
      <pre v-else ref="logPreRef" class="server-log-view">{{ displayedLogContent }}</pre>
    </CsContainer>

    <CsContainer v-else header="配置预览">
      <template #actions>
        <div class="server-diag-actions">
          <CsButton icon-name="refresh" :loading="cfgLoading" @click="loadConfig">刷新</CsButton>
          <CsButton icon-name="copy" :disabled="!cfgText" @click="copyConfig">复制配置</CsButton>
          <TipIcon
            content="此配置为主控根据当前入站、出站、路由规则与有效用户实时渲染的目标 Xray 配置（业务变更系统会自动同步推送至服务器）。展示内容已对用户 UUID 脱敏，复制时保留原文。"
          />
        </div>
      </template>

      <div v-if="cfgLoading && !cfgText" class="server-diag-loading">
        <CsSpinner size="large" />
      </div>
      <pre v-else class="server-cfg-view">{{ maskUUIDs(cfgText) || '正在计算并加载配置预览…' }}</pre>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.server-tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.server-diag-switch {
  display: flex;
  align-items: center;
}

.server-diag-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.server-diag-lines {
  inline-size: 112px;
  flex: none;
}

.server-diag-loading {
  padding: 32px 0;
  text-align: center;
}

.server-log-view,
.server-cfg-view {
  margin: 0;
  max-height: 420px;
  overflow: auto;
  padding: 14px;
  border-radius: 8px;
  background: #0b0f19;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
}

.server-log-view {
  color: #c7d2fe;
}

.server-cfg-view {
  color: #34d399;
}
</style>
