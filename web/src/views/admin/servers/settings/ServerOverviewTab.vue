<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsStatusIndicator,
  CsTooltip,
  ElMessage,
} from '@/components/cloudscape'
import { getInbounds, runServerCommand, type CommandResult, type InboundItem, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateTime } from '@/utils/timezone'
import { pushStateChip, pushStateSummary, xrayChip } from '@/utils/server-status'

// 概览只回答「这台机器现在怎么样」：在线/进程/配置同步/接入点。
// 名称、地址、地区、到期、计费这些可编辑的资产字段一律只在「设置」页签出现，
// 服务器身份摘要放在弹窗标题下（ServerSettingsModal），这里不再回放第二份。

const props = defineProps<{
  server: ServerItem
  active: boolean
}>()

const emit = defineEmits<{
  (e: 'open-inbounds'): void
}>()

const inbounds = ref<InboundItem[]>([])
const inboundsLoading = ref(false)
const inboundsLoaded = ref(false)

const statusData = ref<CommandResult<any> | null>(null)
const statusLoading = ref(false)

const status = computed(() => (statusData.value?.data ?? null) as any)
const offline = computed(() => props.server.status !== 1)

const pushChip = computed(() => pushStateChip(props.server))
const pushSummary = computed(() => pushStateSummary(props.server))
const xray = computed(() => xrayChip(props.server))

function fmtTime(t: string | null | undefined) {
  if (!t) return '—'
  return formatDateTime(t, '—')
}

function formatUptime(sec: number) {
  if (!sec) return '0 秒'
  if (sec < 60) return `${sec} 秒`
  const m = Math.floor(sec / 60)
  const h = Math.floor(m / 60)
  const d = Math.floor(h / 24)
  if (d) return `${d} 天 ${h % 24} 小时`
  if (h) return `${h} 小时 ${m % 60} 分`
  return `${m} 分`
}

const processText = computed(() => {
  if (offline.value) return '服务器离线，无法查询实时进程'
  const s = status.value
  if (!s) return statusLoading.value ? '加载中…' : '未获取到进程信息'
  const parts: string[] = []
  if (s.pid) parts.push(`PID ${s.pid}`)
  if (s.uptime_sec !== undefined && s.uptime_sec !== null) parts.push(`已运行 ${formatUptime(s.uptime_sec)}`)
  if (s.started_at) parts.push(`启动于 ${fmtTime(s.started_at)}`)
  if (s.config_path) parts.push(s.config_path)
  return parts.join(' · ') || '未获取到进程信息'
})

async function loadInbounds() {
  inboundsLoading.value = true
  try {
    const { data } = await getInbounds(props.server.id)
    if (data.code === 0) inbounds.value = data.data.items
  } catch {
    /* 摘要加载失败不打扰 */
  } finally {
    inboundsLoading.value = false
    inboundsLoaded.value = true
  }
}

async function loadStatus() {
  // 离线节点的 get_status 只会被网关直接拒掉（502「服务器离线」），
  // 自动查询会在每次打开弹窗时弹一个必然失败的 toast，这里直接跳过。
  if (offline.value) {
    statusData.value = null
    return
  }
  statusLoading.value = true
  try {
    const { data } = await runServerCommand<CommandResult>({ ids: [props.server.id], type: 'get_status' })
    statusData.value = data.data
  } catch (e) {
    statusData.value = null
    ElMessage.error(errMsg(e, '查询状态失败'))
  } finally {
    statusLoading.value = false
  }
}

function refresh() {
  void loadStatus()
  void loadInbounds()
}

watch(
  () => [props.active, props.server.id, props.server.status] as const,
  ([active, id, status], previous) => {
    if (previous && previous[1] !== id) {
      inbounds.value = []
      statusData.value = null
      inboundsLoaded.value = false
    }
    // 在线状态变化（掉线/恢复）后旧进程信息即失效：清空后转在线时会自动补查一次。
    if (previous && previous[2] !== status) {
      statusData.value = null
    }
    if (!active) return
    if (!inboundsLoaded.value) void loadInbounds()
    // 离线期间不查实时进程；节点转在线后自动补一次。
    if (status === 1 && !statusData.value) void loadStatus()
  },
  { immediate: true },
)
</script>

<template>
  <div class="server-tab-stack">
    <CsContainer header="运行状态">
      <template #actions>
        <CsTooltip v-if="offline" content="服务器离线，无法查询实时进程">
          <CsButton icon-name="refresh" disabled>刷新状态</CsButton>
        </CsTooltip>
        <CsButton v-else icon-name="refresh" :loading="statusLoading" @click="refresh">刷新状态</CsButton>
      </template>

      <div class="server-status-band">
        <div class="server-status-cell">
          <span class="server-status-label">通信状态</span>
          <span class="server-status-value">
            <CsStatusIndicator
              :type="offline ? 'stopped' : 'success'"
              :text="offline ? '离线' : '在线'"
            />
          </span>
          <span class="server-status-sub">最后心跳 {{ fmtTime(server.last_seen_at) }}</span>
        </div>

        <div class="server-status-cell">
          <span class="server-status-label">Xray 进程</span>
          <span class="server-status-value">
            <CsTooltip :content="xray.tip">
              <CsBadge :color="xray.color">{{ xray.text }}</CsBadge>
            </CsTooltip>
          </span>
        </div>

        <div class="server-status-cell">
          <span class="server-status-label">配置同步</span>
          <span class="server-status-value">
            <CsTooltip :content="pushChip.tip">
              <CsBadge :color="pushChip.color">{{ pushChip.text }}</CsBadge>
            </CsTooltip>
          </span>
          <span class="server-status-sub">{{ pushSummary }}</span>
        </div>

        <div class="server-status-cell">
          <span class="server-status-label">托管接入点</span>
          <span class="server-status-value">
            <span class="server-status-count">{{ inboundsLoading && !inboundsLoaded ? '…' : inbounds.length }}</span>
            <span class="server-status-unit">个</span>
            <CsButton variant="inline-link" @click="emit('open-inbounds')">前往管理</CsButton>
          </span>
        </div>
      </div>

      <div class="server-meta">
        <div class="server-meta-row">
          <span class="server-status-label">Agent 版本</span>
          <span class="cs-text-mono server-meta-value">{{ server.agent_version || '—' }}</span>
          <span class="server-status-label">Xray 内核</span>
          <span class="cs-text-mono server-meta-value">{{ server.xray_version || '—' }}</span>
        </div>
        <div class="server-meta-row">
          <span class="server-status-label">实时进程</span>
          <span class="cs-text-mono server-meta-value server-process-value">{{ processText }}</span>
        </div>
      </div>

      <div
        v-if="server.xray_last_error || server.config_drift || (server.config_status === 'pending' && server.push_error)"
        class="server-tab-alerts"
      >
        <CsAlert
          v-if="server.xray_last_error"
          :type="server.xray_state === 'failed' ? 'error' : 'warning'"
          :header="server.xray_state === 'failed' ? 'Xray 启动失败' : '上次启动警告'"
        >
          {{ server.xray_last_error }}
          <template v-if="server.xray_failures">（连续失败 {{ server.xray_failures }} 次，最后尝试 {{ fmtTime(server.xray_error_at) }}）</template>
          <template v-if="server.xray_state === 'failed'">（已停止重试，可尝试重启 Xray）</template>
        </CsAlert>

        <CsAlert v-if="server.config_drift" type="error" header="磁盘偏离">
          节点磁盘配置与主控不一致。磁盘哈希 {{ (server.xray_disk_hash || '').slice(0, 12) || '未上报' }}…，
          重新生成并推送一次即可对齐。
        </CsAlert>

        <CsAlert
          v-if="server.config_status === 'pending' && server.push_error"
          type="warning"
          header="推送失败"
        >
          {{ server.push_error }}（已重试 {{ server.push_attempts || 0 }} 次，最后尝试 {{ fmtTime(server.push_last_try_at) }}）
        </CsAlert>
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.server-tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 状态带：4 个同等权重的「主」字段，用格子线分隔，与下方次要信息（KV/进程行）拉开层级。
   用 gap + 背景色画分隔线，2 列降级时也不会出现缺边或重边。 */
.server-status-band {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: 8px;
  overflow: hidden;
  background: var(--color-border-divider-default-diwd4w, #dedee3);
}

.server-status-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-inline-size: 0;
  padding: 12px 16px;
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

.server-status-label {
  font-size: var(--font-size-body-s, 12px);
  line-height: var(--line-height-body-s, 16px);
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-status-value {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: var(--font-size-body-m, 14px);
  line-height: var(--line-height-body-m, 20px);
  font-weight: 600;
}

.server-status-count {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 18px;
  line-height: 22px;
  font-variant-numeric: tabular-nums;
}

.server-status-unit {
  font-size: var(--font-size-body-s, 12px);
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-status-sub {
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

/* 次要信息：版本与进程细节，小字灰标签 + 默认色等宽值，明确低于状态带一级。 */
.server-meta {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
}

.server-meta-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  min-inline-size: 0;
}

.server-meta-value {
  margin-inline-end: 14px;
  font-size: 12.5px;
  overflow-wrap: anywhere;
}

.server-process-value {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-tab-alerts {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 16px;
}

@media (max-width: 720px) {
  .server-status-band {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
