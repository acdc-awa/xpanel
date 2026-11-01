<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  CsBadge,
  CsButton,
  CsContainer,
  CsEmpty,
  CsOption,
  CsSelect,
  CsSpinner,
} from '@/components/cloudscape'
import {
  getServerOnlineIPs,
  getServerUserRates,
  type OnlineUserIPItem,
  type UserRateItem,
} from '@/api/admin'
import TipIcon from '@/components/base/TipIcon.vue'
import { formatBytes } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'

// 节点在线用户面板（agent 心跳的连接级实时快照）：
// 服务器设置弹窗（boxed）与仪表盘在线设备弹窗共用；serverId 变化即自动加载。
//
// 2026-11-01：快车道 metrics 帧接上后，本面板同时显示每用户实时上下行速率，并支持按
// 入站筛选。在线名单仍以 online-ips（慢车道心跳）为准；速率是按（用户, 入站）上报的
// 原始行，这里按用户合并 —— 「全部入站」显示跨入站合计，选中某入站时只显示该入站的值
// 且只保留在该入站有流量的用户。旧 agent 没有速率数据（supported=false）时两列与筛选
// 都不渲染，面板行为与今天完全一致。
const props = withDefaults(
  defineProps<{
    serverId: number
    /** 外面已经是裸内容区（如仪表盘弹窗）时用默认值；设置弹窗里套一层卡片，刷新/说明进卡片头。 */
    boxed?: boolean
  }>(),
  { boxed: false },
)

const ONLINE_TIP =
  '当前持有活跃连接的用户与其连接来源 IP，随连接建立/断开实时变化（空闲保持的连接也计为在线）。列表为空表示当前无人在线，或服务器 Agent 版本过旧未上报。'
const RATE_TIP =
  '上行/下行是该用户最近一个采样窗口内的真实速率（按入站分别统计后合并）。速率来自节点快车道指标帧，刷新间隔由设置页「实时指标周期」决定（默认 3 秒）。'

/** 速率轮询周期：与主控默认的实时指标周期一致，纯内存读、开销可忽略。 */
const RATE_POLL_MS = 3000

const onlineUsers = ref<OnlineUserIPItem[]>([])
const onlineLoading = ref(false)

const rates = ref<UserRateItem[]>([])
const rateInbounds = ref<{ id: number; tag: string; name: string }[]>([])
const rateSupported = ref(false)
const sampledAt = ref(0)
/** 入站筛选：0 = 全部 */
const inboundFilter = ref(0)

let pollTimer: number | undefined

async function loadOnlineUsers() {
  if (!props.serverId) return
  onlineLoading.value = true
  try {
    const { data } = await getServerOnlineIPs(props.serverId)
    if (data.code === 0) onlineUsers.value = data.data.users
  } catch {
    /* 面板内展示，失败不打扰 */
  } finally {
    onlineLoading.value = false
  }
}

async function loadRates() {
  if (!props.serverId) return
  try {
    const { data } = await getServerUserRates(props.serverId)
    if (data.code !== 0) return
    rates.value = data.data.rates ?? []
    rateInbounds.value = data.data.inbounds ?? []
    rateSupported.value = Boolean(data.data.supported)
    sampledAt.value = data.data.sampled_at ?? 0
    // 选中的入站可能已经没人用了：回到「全部」，否则面板会一直空着
    if (inboundFilter.value && !rateInbounds.value.some((i) => i.id === inboundFilter.value)) {
      inboundFilter.value = 0
    }
  } catch {
    /* 同上 */
  }
}

async function reload() {
  await Promise.all([loadOnlineUsers(), loadRates()])
}

function startPoll() {
  stopPoll()
  pollTimer = window.setInterval(loadRates, RATE_POLL_MS)
}

function stopPoll() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function kindText(kind: OnlineUserIPItem['kind']) {
  if (kind === 'user') return '用户'
  if (kind === 'relay') return '中转'
  return '其他'
}

function kindColor(kind: OnlineUserIPItem['kind']) {
  if (kind === 'user') return 'green' as const
  if (kind === 'relay') return 'blue' as const
  return 'grey' as const
}

function inboundLabel(inb: { id: number; tag: string; name: string }) {
  return inb.name || inb.tag || `#${inb.id}`
}

/** 按筛选条件把 (用户, 入站) 速率行合并成 userId → {up, down}。 */
const rateByUser = computed(() => {
  const m = new Map<number, { up: number; down: number }>()
  for (const r of rates.value) {
    if (!r.user_id) continue // relay 内部账户/自定义 email：不在用户行里合并
    if (inboundFilter.value && r.inbound_id !== inboundFilter.value) continue
    const cur = m.get(r.user_id) ?? { up: 0, down: 0 }
    cur.up += r.up
    cur.down += r.down
    m.set(r.user_id, cur)
  }
  return m
})

interface Row extends OnlineUserIPItem {
  rate?: { up: number; down: number }
}

/**
 * 面板行：在线用户 × 速率。
 * 选中某一入站时只保留在该入站有速率的用户（没走这个入站的人与这次观察无关）；
 * 有速率数据时整列表按上下行合计降序，没有速率行的用户排在末尾并保持原顺序（sort 稳定）。
 */
const rows = computed<Row[]>(() => {
  const list: Row[] = onlineUsers.value.map((u) => ({
    ...u,
    rate: u.user_id ? rateByUser.value.get(u.user_id) : undefined,
  }))
  if (!rateSupported.value) return list
  const filtered = inboundFilter.value ? list.filter((u) => u.rate) : list
  return filtered.sort((a, b) => totalOf(b) - totalOf(a))
})

function totalOf(r: Row) {
  return r.rate ? r.rate.up + r.rate.down : 0
}

function fmtRate(v: number) {
  return `${formatBytes(Math.round(v))}/s`
}

/** 采样时刻（HH:mm:ss）：让管理员一眼看出这份速率有多新。 */
const sampledText = computed(() => {
  if (!rateSupported.value || !sampledAt.value) return ''
  const iso = new Date(sampledAt.value * 1000).toISOString()
  const text = formatDateTime(iso, '')
  // 只取末尾的 HH:mm:ss（不同环境的分隔符/日期段写法不稳定，末尾 8 位是唯一稳的）
  return text ? `采样于 ${text.slice(-8)}` : ''
})

watch(
  () => props.serverId,
  (id) => {
    onlineUsers.value = []
    rates.value = []
    rateInbounds.value = []
    rateSupported.value = false
    inboundFilter.value = 0
    stopPoll()
    if (id) {
      void reload()
      startPoll()
    }
  },
  { immediate: true },
)

onBeforeUnmount(stopPoll)

// 供宿主在 Tab 重新激活/弹窗重开时强制重拉（serverId 未变时 watch 不触发）
defineExpose({ reload })
</script>

<template>
  <!-- 同一份内容按宿主切换外壳：设置弹窗里套卡片（刷新/说明在卡片头），仪表盘弹窗里保持裸内容区。 -->
  <component :is="boxed ? CsContainer : 'div'" :header="boxed ? '在线用户' : undefined">
    <template v-if="boxed" #actions>
      <CsButton icon-name="refresh" :loading="onlineLoading" @click="reload">刷新</CsButton>
    </template>
    <template v-if="boxed" #info>
      <TipIcon :content="rateSupported ? `${ONLINE_TIP}${RATE_TIP}` : ONLINE_TIP" />
    </template>

    <!-- 工具栏按需出现：没有速率数据、非裸内容区时整块不渲染（不让空 div 留下 10px 空隙） -->
    <div v-if="sampledText || (rateSupported && rateInbounds.length > 1) || !boxed" class="online-toolbar">
      <span v-if="sampledText" class="online-sampled">{{ sampledText }}</span>
      <CsSelect
        v-if="rateSupported && rateInbounds.length > 1"
        v-model="inboundFilter"
        class="online-filter"
        aria-label="按入站筛选"
      >        <CsOption :value="0" label="全部入站" />
        <CsOption v-for="inb in rateInbounds" :key="inb.id" :value="inb.id" :label="inboundLabel(inb)" />
      </CsSelect>
      <template v-if="!boxed">
        <CsButton icon-name="refresh" :loading="onlineLoading" @click="reload">刷新</CsButton>
        <TipIcon :content="rateSupported ? `${ONLINE_TIP}${RATE_TIP}` : ONLINE_TIP" />
      </template>
    </div>

    <div v-if="onlineLoading && onlineUsers.length === 0" class="online-loading">
      <CsSpinner size="large" />
    </div>
    <CsEmpty v-else-if="rows.length === 0" icon-name="user-profile" description="当前没有在线用户" />
    <div v-else class="online-list">
      <div v-for="u in rows" :key="u.email" class="online-row">
        <div class="online-main">
          <div class="online-user">
            <CsBadge :color="kindColor(u.kind)">{{ kindText(u.kind) }}</CsBadge>
            <span class="online-name">{{ u.email }}</span>
            <span v-if="u.ips.length > 1" class="online-count">× {{ u.ips.length }}</span>
          </div>
          <div class="online-ips">
            <code v-for="ip in u.ips" :key="ip" class="online-ip">{{ ip }}</code>
          </div>
        </div>
        <div v-if="rateSupported" class="online-rate">
          <span class="online-rate-item">
            <span class="online-rate-label">上行</span>
            <span class="online-rate-value">{{ u.rate ? fmtRate(u.rate.up) : '—' }}</span>
          </span>
          <span class="online-rate-item">
            <span class="online-rate-label">下行</span>
            <span class="online-rate-value">{{ u.rate ? fmtRate(u.rate.down) : '—' }}</span>
          </span>
        </div>
      </div>
    </div>
  </component>
</template>

<style scoped lang="scss">
.online-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}

.online-sampled {
  margin-inline-end: auto;
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.online-filter {
  width: 220px;
}

.online-loading {
  padding: 24px 0;
  text-align: center;
}

.online-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.online-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: flex-start;
  gap: 8px 16px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

.online-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.online-user {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
}

.online-count {
  font-size: 12px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.online-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.online-ip {
  padding: 2px 8px;
  border-radius: 6px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.online-rate {
  display: flex;
  gap: 12px;
  flex-shrink: 0;
  font-size: 12px;
}

.online-rate-item {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
  min-width: 72px;
}

.online-rate-label {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.online-rate-value {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  font-weight: 500;
}

@media (max-width: 640px) {
  .online-rate {
    width: 100%;
    justify-content: flex-start;
  }
  .online-rate-item {
    align-items: flex-start;
  }
}
</style>
