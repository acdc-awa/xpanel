<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsKeyValuePairs,
  CsStatusIndicator,
  CsTooltip,
  ElMessage,
  type KeyValuePair,
} from '@/components/cloudscape'
import { getInbounds, serverCommand, type CommandResult, type InboundItem, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateTime } from '@/utils/timezone'
import { formatExpireDate, getExpiryStatus, isUrl, normalizeUrl } from '@/utils/vps'

type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-critical'
  | 'severity-high'
  | 'severity-medium'
  | 'severity-low'
  | 'severity-neutral'

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
const statusLoaded = ref(false)

const status = computed(() => (statusData.value?.data ?? null) as any)

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

function xrayStateText(state: string) {
  switch (state) {
    case 'running':
      return '运行中'
    case 'restarting':
      return '启动失败，自动重试中'
    case 'failed':
      return '启动失败，已停止自动拉起'
    case 'stopped':
      return '未启动'
    default:
      return state
  }
}

const xrayChip = computed<{ color: BadgeColor; text: string }>(() => {
  const s = props.server.xray_state
  if (s === 'failed') return { color: 'red', text: '启动失败（已停止重试）' }
  if (s === 'restarting') return { color: 'severity-medium', text: '启动失败，重试中' }
  if (props.server.xray_running) return { color: 'green', text: '运行中' }
  return { color: 'red', text: '未运行' }
})

const pushChip = computed<{ color: BadgeColor; text: string }>(() => {
  const st =
    props.server.push_state ||
    (props.server.config_status === 'pushed'
      ? 'synced'
      : props.server.config_status === 'pending'
        ? 'pending'
        : 'none')
  switch (st) {
    case 'drift':
      return { color: 'red', text: '磁盘偏离' }
    case 'rejected':
      return { color: 'red', text: '推送被拒' }
    case 'synced':
      return { color: 'green', text: 'SYNCED' }
    case 'pending':
      return { color: 'severity-medium', text: '待推送' }
    default:
      return { color: 'grey', text: '未投递' }
  }
})

const expiry = computed(() => getExpiryStatus(props.server.expire_at))

function expiryColor(cls: string): BadgeColor {
  if (cls === 'red') return 'red'
  if (cls === 'deep-orange' || cls === 'orange' || cls === 'yellow') return 'severity-medium'
  return 'grey'
}

const statusStateText = computed(() => {
  const s = status.value
  if (!s?.xray_state) return ''
  const label = xrayStateText(s.xray_state)
  return s.xray_restart_failures ? `${label}（连续失败 ${s.xray_restart_failures} 次）` : label
})

const processText = computed(() => {
  const s = status.value
  if (!s) return statusLoading.value ? '加载中…' : '—'
  const parts: string[] = []
  if (s.pid) parts.push(`PID ${s.pid}`)
  if (s.uptime_sec !== undefined && s.uptime_sec !== null) parts.push(`已运行 ${formatUptime(s.uptime_sec)}`)
  if (s.started_at) parts.push(`启动于 ${fmtTime(s.started_at)}`)
  if (s.config_path) parts.push(s.config_path)
  return parts.join(' · ') || '—'
})

const runtimeItems: KeyValuePair[] = [
  { key: 'status', label: '通信状态' },
  { key: 'xray', label: 'Xray 进程' },
  { key: 'agent', label: 'Agent 版本' },
  { key: 'xrayVersion', label: 'Xray 内核' },
  { key: 'heartbeat', label: '最后心跳' },
  { key: 'inbounds', label: '托管接入点' },
  { key: 'process', label: '实时进程' },
  { key: 'node', label: 'Node ID' },
]

const networkItems: KeyValuePair[] = [
  { key: 'host', label: '主机地址 Host' },
  { key: 'location', label: '所在地区' },
  { key: 'expire', label: '到期日' },
  { key: 'billing', label: '计费与价格' },
]

const assetItems = computed<KeyValuePair[]>(() => {
  const items: KeyValuePair[] = []
  if (props.server.idc_address) items.push({ key: 'idc', label: 'IDC 控制台' })
  if (props.server.remark) items.push({ key: 'remark', label: '备注说明' })
  return items
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
  statusLoading.value = true
  try {
    const { data } = await serverCommand(props.server.id, 'get_status')
    statusData.value = data.data
  } catch (e) {
    statusData.value = null
    ElMessage.error(errMsg(e, '查询状态失败'))
  } finally {
    statusLoading.value = false
    statusLoaded.value = true
  }
}

function refresh() {
  void loadStatus()
  void loadInbounds()
}

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

watch(
  () => [props.active, props.server.id] as const,
  ([active, id], previous) => {
    if (previous && previous[1] !== id) {
      inbounds.value = []
      statusData.value = null
      inboundsLoaded.value = false
      statusLoaded.value = false
    }
    if (!active) return
    if (!inboundsLoaded.value) void loadInbounds()
    if (!statusLoaded.value) void loadStatus()
  },
  { immediate: true },
)
</script>

<template>
  <div class="server-tab-stack">
    <CsContainer header="运行时与通信">
      <template #actions>
        <div class="server-tab-actions">
          <CsStatusIndicator
            :type="server.status === 1 ? 'success' : 'stopped'"
            :text="server.status === 1 ? '在线' : '离线'"
          />
          <CsBadge :color="pushChip.color">{{ pushChip.text }}</CsBadge>
          <CsButton icon-name="refresh" :loading="statusLoading" @click="refresh">刷新状态</CsButton>
        </div>
      </template>

      <CsKeyValuePairs :items="runtimeItems" :columns="2" aria-label="运行时与通信">
        <template #value="{ item }">
          <template v-if="item.key === 'status'">
            <CsStatusIndicator
              :type="server.status === 1 ? 'success' : 'stopped'"
              :text="server.status === 1 ? '在线' : '离线'"
            />
          </template>
          <template v-else-if="item.key === 'xray'">
            <span class="server-tab-inline">
              <CsBadge :color="xrayChip.color">{{ xrayChip.text }}</CsBadge>
              <span v-if="statusStateText" class="server-tab-muted">{{ statusStateText }}</span>
            </span>
          </template>
          <template v-else-if="item.key === 'agent'">
            <span class="cs-text-mono">{{ server.agent_version || '—' }}</span>
          </template>
          <template v-else-if="item.key === 'xrayVersion'">
            <span class="cs-text-mono">{{ server.xray_version || '—' }}</span>
          </template>
          <template v-else-if="item.key === 'heartbeat'">
            <span class="cs-text-mono">{{ fmtTime(server.last_seen_at) }}</span>
          </template>
          <template v-else-if="item.key === 'inbounds'">
            <span class="server-tab-inline">
              <span class="cs-text-mono">{{ inboundsLoading && !inboundsLoaded ? '…' : inbounds.length }}</span>
              <span>个入站</span>
              <CsButton variant="inline-link" @click="emit('open-inbounds')">前往管理</CsButton>
            </span>
          </template>
          <template v-else-if="item.key === 'process'">
            <span class="cs-text-mono server-tab-wrap">{{ processText }}</span>
          </template>
          <template v-else-if="item.key === 'node'">
            <span class="server-tab-inline">
              <span class="cs-text-mono server-tab-wrap">{{ server.node_id }}</span>
              <CsButton
                variant="inline-icon"
                icon-name="copy"
                aria-label="复制 Node ID"
                @click="copyText(server.node_id, 'Node ID')"
              />
            </span>
          </template>
        </template>
      </CsKeyValuePairs>

      <div v-if="server.xray_last_error || server.config_drift || (server.config_status === 'pending' && server.push_error)" class="server-tab-alerts">
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

    <CsContainer header="网络接入与 VPS 资产">
      <CsKeyValuePairs :items="networkItems" :columns="2" aria-label="网络接入与 VPS 资产">
        <template #value="{ item }">
          <template v-if="item.key === 'host'">
            <span class="server-tab-inline">
              <span class="cs-text-mono server-tab-wrap">{{ server.host }}</span>
              <CsButton
                variant="inline-icon"
                icon-name="copy"
                aria-label="复制主机地址"
                @click="copyText(server.host, '主机地址')"
              />
            </span>
          </template>
          <template v-else-if="item.key === 'location'">
            <CsBadge v-if="server.location" color="blue">{{ server.location }}</CsBadge>
            <span v-else class="server-tab-muted">—</span>
          </template>
          <template v-else-if="item.key === 'expire'">
            <span v-if="expiry" class="server-tab-inline">
              <span class="cs-text-mono">{{ formatExpireDate(server.expire_at) }}</span>
              <CsTooltip :content="expiry.tip">
                <CsBadge :color="expiryColor(expiry.cls)">{{ expiry.text }}</CsBadge>
              </CsTooltip>
            </span>
            <span v-else class="server-tab-muted">—</span>
          </template>
          <template v-else-if="item.key === 'billing'">
            <span class="server-tab-inline">
              <span v-if="server.price" class="cs-text-mono server-tab-price">{{ server.price }}</span>
              <CsBadge v-if="server.billing_cycle" color="grey">{{ server.billing_cycle }}</CsBadge>
              <span v-if="!server.price && !server.billing_cycle" class="server-tab-muted">—</span>
            </span>
          </template>
        </template>
      </CsKeyValuePairs>

      <CsKeyValuePairs v-if="assetItems.length" :items="assetItems" :columns="1" class="server-tab-kv-extra" aria-label="VPS 资产补充信息">
        <template #value="{ item }">
          <template v-if="item.key === 'idc'">
            <span class="server-tab-inline">
              <CsButton
                v-if="isUrl(server.idc_address!)"
                variant="inline-link"
                icon-name="external"
                :href="normalizeUrl(server.idc_address!)"
                target="_blank"
              >{{ server.idc_address }}</CsButton>
              <span v-else class="cs-text-mono server-tab-wrap">{{ server.idc_address }}</span>
              <CsButton
                variant="inline-icon"
                icon-name="copy"
                aria-label="复制 IDC 地址"
                @click="copyText(server.idc_address!, 'IDC 控制台')"
              />
            </span>
          </template>
          <template v-else-if="item.key === 'remark'">
            <span class="server-tab-remark">{{ server.remark }}</span>
          </template>
        </template>
      </CsKeyValuePairs>
    </CsContainer>

    <CsContainer header="节点通信凭据">
      <CsKeyValuePairs :items="[{ key: 'node', label: 'Node ID' }]" :columns="1" aria-label="节点通信凭据">
        <template #value>
          <span class="server-tab-inline">
            <span class="cs-text-mono server-tab-wrap">{{ server.node_id }}</span>
            <CsButton
              variant="inline-icon"
              icon-name="copy"
              aria-label="复制 Node ID"
              @click="copyText(server.node_id, 'Node ID')"
            />
          </span>
        </template>
      </CsKeyValuePairs>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.server-tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.server-tab-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.server-tab-inline {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  max-inline-size: 100%;
}

.server-tab-wrap {
  overflow-wrap: anywhere;
}

.server-tab-muted {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: 12.5px;
}

.server-tab-price {
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

.server-tab-remark {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

.server-tab-alerts {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 16px;
}

.server-tab-kv-extra {
  margin-top: 12px;
}
</style>
