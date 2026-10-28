<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import {
  CsBadge,
  CsButton,
  CsModal,
  CsProgressBar,
  CsSpaceBetween,
  CsTable,
  ElMessage,
  type TableColumn,
} from '@/components/cloudscape'
import {
  getServerUpgradeStatuses,
  runServerCommand,
  type AgentUpgradeStatus,
  type CommandDispatchResult,
  type ServerCommandType,
} from '@/api/admin'
import { errMsg } from '@/api/http'

// 批量动作总览：一次只处理一种动作（升级 Agent / 升级 Xray / 重启 Xray）。
// 派发与轮询都收在这里，页面只负责「选服务器 → 确认 → 打开本弹窗」。

export type BatchActionKind = 'agent' | 'xray' | 'restart'

export interface BatchActionItem {
  id: number
  name: string
  version: string
}

export interface BatchActionRequest {
  ids: number[]
  type: ServerCommandType
  target?: string
  force?: boolean
}

interface BatchRow extends BatchActionItem {
  skipped: boolean
  reason?: string
  status: AgentUpgradeStatus | null
}

const props = defineProps<{
  visible: boolean
  kind: BatchActionKind
  items: BatchActionItem[]
  request: BatchActionRequest | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'finished'): void
}>()

const rows = ref<BatchRow[]>([])
const running = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const title = computed(() => {
  if (props.kind === 'xray') return '批量升级 Xray'
  if (props.kind === 'restart') return '批量重启 Xray'
  return '批量升级 Agent'
})

const showVersion = computed(() => props.kind !== 'restart')

const columns = computed<TableColumn[]>(() => {
  const cols: TableColumn[] = [{ id: 'name', header: '服务器', key: 'name', minWidth: 140 }]
  if (showVersion.value) {
    cols.push({
      id: 'version',
      header: props.kind === 'xray' ? '当前内核' : '当前版本',
      width: 110,
    })
  }
  cols.push({ id: 'status', header: '状态', width: 120 })
  cols.push({ id: 'progress', header: '进度', minWidth: 200 })
  return cols
})

const phaseLabels = computed<Record<string, string>>(() => ({
  starting: '排队中',
  checking: '检查版本',
  downloading: '下载资源',
  verifying: '校验完整性',
  replacing: '替换二进制',
  restarting: props.kind === 'restart' ? '重启中' : '重启生效',
  success: props.kind === 'restart' ? '已重启' : '升级成功',
  failed: props.kind === 'restart' ? '重启失败' : '升级失败',
}))

const phasePercents: Record<string, number> = {
  starting: 5,
  checking: 15,
  downloading: 50,
  verifying: 70,
  replacing: 85,
  restarting: 95,
  success: 100,
  failed: 100,
}

/** 状态槽按 kind 共用：只认本次动作对应的记录，避免上一次升级的残留状态串台。 */
function kindMatches(st: AgentUpgradeStatus) {
  if (props.kind === 'xray') return st.kind === 'xray'
  if (props.kind === 'restart') return st.kind === 'restart'
  return !st.kind || st.kind === 'agent'
}

function progressStatus(phase?: string): 'success' | 'error' | 'in-progress' {
  if (phase === 'success') return 'success'
  if (phase === 'failed') return 'error'
  return 'in-progress'
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

onUnmounted(stopPolling)

async function poll() {
  const activeIds = rows.value.filter((r) => !r.skipped).map((r) => r.id)
  if (!activeIds.length) {
    finish()
    return
  }
  try {
    const { data } = await getServerUpgradeStatuses(activeIds)
    if (data.code !== 0) return
    const statuses = data.data.statuses || {}
    for (const row of rows.value) {
      if (row.skipped) continue
      const st = statuses[String(row.id)]
      if (st && kindMatches(st)) row.status = st
    }
    const allDone = rows.value
      .filter((r) => !r.skipped)
      .every((r) => r.status && (r.status.phase === 'success' || r.status.phase === 'failed'))
    if (allDone) finish()
  } catch {
    // 轮询抖动忽略：下一轮继续
  }
}

function finish() {
  stopPolling()
  if (!running.value) return
  running.value = false
  emit('finished')
}

async function start() {
  if (!props.request || !props.request.ids.length) return
  rows.value = props.items.map((it) => ({
    ...it,
    skipped: false,
    status: { phase: 'starting', message: '提交中…', ts: Math.floor(Date.now() / 1000) },
  }))
  running.value = true
  stopPolling()
  pollTimer = setInterval(() => void poll(), 1500)

  try {
    const { data } = await runServerCommand<CommandDispatchResult>(props.request)
    if (data.code !== 0) {
      ElMessage.error(data.message || '提交失败')
      stopPolling()
      running.value = false
      return
    }
    const skippedMap = new Map(data.data.skipped.map((s) => [s.id, s.reason]))
    const dispatched = new Set(data.data.dispatched.map((d) => d.id))
    for (const row of rows.value) {
      if (skippedMap.has(row.id)) {
        row.skipped = true
        row.reason = skippedMap.get(row.id)
        row.status = null
      } else if (!dispatched.has(row.id)) {
        row.skipped = true
        row.reason = '未派发'
        row.status = null
      }
    }
    void poll()
  } catch (e) {
    stopPolling()
    running.value = false
    ElMessage.error(errMsg(e, '批量操作提交失败'))
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      void start()
    } else {
      stopPolling()
      running.value = false
    }
  },
)

function close() {
  stopPolling()
  running.value = false
  emit('update:visible', false)
}
</script>

<template>
  <CsModal
    :visible="visible"
    :header="title"
    size="large"
    :close-on-click-overlay="false"
    @update:visible="(v: boolean) => !v && close()"
  >
    <CsSpaceBetween size="s">
      <CsTable :items="rows" :columns="columns" track-by="id" empty-text="暂无服务器">
        <template v-if="showVersion" #cell-version="{ item }">
          <span class="cs-text-mono">{{ item.version }}</span>
        </template>
        <template #cell-status="{ item }">
          <CsBadge v-if="item.skipped" color="grey">已跳过</CsBadge>
          <CsBadge
            v-else-if="item.status"
            :color="item.status.phase === 'success' ? 'green' : item.status.phase === 'failed' ? 'red' : 'blue'"
          >
            {{ phaseLabels[item.status.phase] || item.status.phase }}
          </CsBadge>
          <CsBadge v-else color="grey">等待</CsBadge>
        </template>
        <template #cell-progress="{ item }">
          <span v-if="item.skipped" class="cs-text-secondary">{{ item.reason }}</span>
          <template v-else-if="item.status">
            <CsProgressBar
              :value="phasePercents[item.status.phase] ?? 0"
              :status="progressStatus(item.status.phase)"
              :bar-height="8"
            />
            <div v-if="item.status.error" class="batch-error cs-text-mono">{{ item.status.error }}</div>
            <div v-else-if="item.status.message" class="batch-message cs-text-secondary">
              {{ item.status.message }}
            </div>
          </template>
          <span v-else class="cs-text-secondary">—</span>
        </template>
      </CsTable>
      <p class="batch-hint cs-text-secondary">
        <template v-if="running">
          正在执行，进度实时更新…完成的服务器会在下一次心跳后刷新版本/运行状态。
        </template>
        <template v-else>全部服务器已处理完成。</template>
      </p>
    </CsSpaceBetween>
    <template #footer>
      <CsButton variant="primary" @click="close">关闭</CsButton>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.batch-error {
  margin-block-start: 2px;
  font-size: 11px;
  color: var(--color-text-status-error-ri38gt, #db0000);
  word-break: break-all;
}

.batch-message {
  margin-block-start: 2px;
  font-size: 11px;
}

.batch-hint {
  margin: 0;
  font-size: 12px;
}
</style>
