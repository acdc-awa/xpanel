<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsProgressBar,
  CsStatusIndicator,
  CsTooltip,
  type BadgeColor,
} from '@/components/cloudscape'
import SortHandle from '@/components/base/SortHandle.vue'
import type { ServerItem } from '@/api/admin'
import { compareVersion } from '@/utils/version'
import { formatBytes } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'
import { formatExpireDate, getExpiryStatus, isUrl, normalizeUrl } from '@/utils/vps'
import { indicatorTypeOf, onlineChip, pushStateChip, xrayChip } from '@/utils/server-status'
import {
  hasNicTrafficData,
  nicQuotaColor,
  nicQuotaPercentText,
  nicResetText,
  nicTrafficOf,
  nicTrafficTooltip,
  nicUsageParts,
} from '@/utils/server-nic-traffic'

// 服务器卡片：名称 → 状态条 → 地址/心跳/版本 → 操作。
// 状态文案与配色全部来自 utils/server-status（与设置弹窗同一份），这里只负责排版。
// 拖拽不在此组件内实现：抓起手把 dragstart 冒泡到列表容器，由页面统一重排（单一写入点）。

const props = defineProps<{
  server: ServerItem
  selected?: boolean
  selectMode?: boolean
  /** 是否允许拖拽排序（筛选/搜索/多选时为 false）。 */
  canReorder?: boolean
  latestAgentVersion?: string
  latestXrayVersion?: string
  inboundCount?: number
}>()

const emit = defineEmits<{
  (e: 'toggle-select'): void
  (e: 'open-settings'): void
  (e: 'restart'): void
  (e: 'metrics'): void
  (e: 'go-inbounds'): void
  (e: 'copy-text', text: string, label: string): void
}>()

const dragging = ref(false)

const comm = computed(() => onlineChip(props.server))
const xray = computed(() => xrayChip(props.server))
const push = computed(() => pushStateChip(props.server))
const expiry = computed(() => getExpiryStatus(props.server.expire_at))

const agentOutdated = computed(
  () =>
    Boolean(props.latestAgentVersion) &&
    Boolean(props.server.agent_version) &&
    compareVersion(props.server.agent_version, props.latestAgentVersion!) < 0,
)
const xrayOutdated = computed(
  () =>
    Boolean(props.latestXrayVersion) &&
    Boolean(props.server.xray_version) &&
    compareVersion(props.server.xray_version!, props.latestXrayVersion!) < 0,
)

// 版本提醒合并成一条徽章：Agent/Xray 同时落后也只有一项，状态条不会因此挤到第二行。
const versionAlert = computed(() => {
  const names: string[] = []
  const detail: string[] = []
  if (agentOutdated.value) {
    names.push('Agent')
    detail.push(`Agent ${props.server.agent_version} → ${props.latestAgentVersion}`)
  }
  if (xrayOutdated.value) {
    names.push('Xray')
    detail.push(`Xray ${props.server.xray_version} → ${props.latestXrayVersion}`)
  }
  if (!names.length) return null
  return { text: `${names.join('、')} 有新版`, tip: detail.join('；') }
})

const heartbeat = computed(() =>
  props.server.last_seen_at ? formatDateTime(props.server.last_seen_at, '—').slice(5, 16) : '—',
)

// 连接数：-1 = 从未上报（旧 agent / 非 Linux 节点），展示为 —，不能显示成「0 个连接」
const hasConns = computed(() => (props.server.conn_tcp_established ?? -1) >= 0)
const connsText = computed(() => {
  if (!hasConns.value) return '—'
  return `TCP ${props.server.conn_tcp_established}，UDP ${props.server.conn_udp}`
})
const connsTip = computed(() => {
  if (!hasConns.value) return '该节点尚未上报连接数（需要 agent 升级）'
  return `TCP 已建立 ${props.server.conn_tcp_established}，连接表共 ${props.server.conn_tcp_total}（含 TIME_WAIT 等残留，不含 LISTEN）；UDP 为绑定的 socket 数，不是会话数`
})

const hasAsset = computed(
  () =>
    Boolean(props.server.expire_at) ||
    Boolean(props.server.billing_cycle) ||
    Boolean(props.server.price) ||
    Boolean(props.server.idc_address),
)

// 周期流量（网卡整机口径）：数据由列表接口附带，卡片只格式化；额度为 0 时不画进度条。
const nic = computed(() => nicTrafficOf(props.server))
const nicHasData = computed(() => hasNicTrafficData(nic.value))
const nicUsage = computed(() =>
  nic.value && nicHasData.value ? nicUsageParts(nic.value) : { used: '—', quota: '' },
)
/** 无额度：额度侧显示 ∞（与进度条一样，只在 quota_bytes > 0 时才画）。 */
const nicUnlimited = computed(() => Boolean(nic.value && nic.value.quota_bytes <= 0))
const nicPercent = computed(() =>
  nic.value && nic.value.quota_bytes > 0 && nicHasData.value ? nicQuotaPercentText(nic.value) : '',
)
const nicBarColor = computed(() => nicQuotaColor(nic.value))
const nicTip = computed(() => nicTrafficTooltip(nic.value))
const nicReset = computed(() => (nic.value && nicHasData.value ? nicResetText(nic.value) : ''))
const nicDown = computed(() => (nic.value && nicHasData.value ? formatBytes(nic.value.rx_bytes) : ''))
const nicUp = computed(() => (nic.value && nicHasData.value ? formatBytes(nic.value.tx_bytes) : ''))

/** 到期状态的颜色语义映射到 Cloudscape Badge（与 utils/vps 的 cls 一一对应）。 */
function badgeColorOf(cls: string): BadgeColor {
  switch (cls) {
    case 'red':
      return 'red'
    case 'green':
    case 'emerald':
      return 'green'
    case 'blue':
      return 'blue'
    case 'orange':
    case 'deep-orange':
    case 'yellow':
    case 'amber':
      return 'severity-medium'
    default:
      return 'grey'
  }
}

const expiryBadgeColor = computed<BadgeColor>(() => (expiry.value ? badgeColorOf(expiry.value.cls) : 'grey'))

function onRootClick() {
  if (props.selectMode) emit('toggle-select')
}
</script>

<template>
  <CsCard
    class="server-card"
    :class="{
      'server-card--selected': selected,
      'server-card--select-mode': selectMode,
      'server-card--dragging': dragging,
    }"
    :data-sort-id="server.id"
    @click="onRootClick"
  >
    <template #header>
      <div class="server-card-head">
        <SortHandle
          v-if="canReorder"
          title="拖拽调整服务器顺序"
          @dragstart="dragging = true"
          @dragend="dragging = false"
        />
        <button type="button" class="server-name" @click.stop="emit('open-settings')">{{ server.name }}</button>
        <CsBadge v-if="server.location" color="blue">{{ server.location }}</CsBadge>
      </div>
    </template>

    <!-- 状态条：只放机器状态（在线 / Xray / 同步 / 版本提醒）。到期属于资产信息，
         跟着到期日落在下方资产行，状态条因此最多 4 项，不会多出一枚孤零零的徽章。 -->
    <div class="server-status-bar">
      <CsTooltip :content="comm.tip">
        <CsStatusIndicator :type="indicatorTypeOf(comm.color)" :text="comm.text" />
      </CsTooltip>
      <span class="server-status-sep" aria-hidden="true" />
      <CsTooltip :content="xray.tip">
        <CsStatusIndicator :type="indicatorTypeOf(xray.color)" :text="xray.text" />
      </CsTooltip>
      <span class="server-status-sep" aria-hidden="true" />
      <CsTooltip :content="push.tip">
        <CsStatusIndicator :type="indicatorTypeOf(push.color)" :text="push.text" />
      </CsTooltip>
      <CsTooltip v-if="versionAlert" :content="versionAlert.tip">
        <CsBadge color="severity-medium" class="server-chip-help">{{ versionAlert.text }}</CsBadge>
      </CsTooltip>
    </div>

    <div class="server-divider" aria-hidden="true" />

    <div class="server-details">
      <!-- 周期流量放在最上面：这是卡片上最常看的一块，地址与机器信息退到它下面。 -->
      <div v-if="nic" class="server-traffic">
        <CsTooltip :content="nicTip" class="traffic-head">
          <span class="traffic-label">已用流量：</span>
          <span class="traffic-value">
            <strong :style="nicBarColor ? { color: nicBarColor } : {}">{{ nicUsage.used }}</strong>
            <span v-if="nicUsage.quota" class="traffic-quota" :class="{ 'traffic-quota--inf': nicUnlimited }">
              / {{ nicUsage.quota }}
            </span>
            <span v-if="nicPercent" class="traffic-percent">({{ nicPercent }})</span>
          </span>
        </CsTooltip>
        <CsProgressBar
          v-if="nicHasData && nic.quota_bytes > 0"
          :value="nic.quota_percent"
          :bar-height="5"
          hide-text
          :color="nicBarColor"
          :aria-label="`周期流量已用 ${nicPercent}`"
        />
        <!-- 明细只留「双向用量 + 还剩几天重置」；完整周期区间与数据完整性交给头部 tooltip -->
        <div v-if="nicHasData" class="traffic-meta">
          <span class="traffic-split">
            <span>↓ {{ nicDown }}</span>
            <span>↑ {{ nicUp }}</span>
          </span>
          <span v-if="nicReset" class="traffic-reset">{{ nicReset }}</span>
        </div>
      </div>

      <!-- 分隔符是独立元素，不归属任何一块：上下留白由 .server-details 的 gap 统一给。 -->
      <div v-if="nic" class="server-divider" aria-hidden="true" />

      <div class="server-host-row">
        <span class="server-label">服务器地址</span>
        <CsButton
          variant="inline-link"
          icon-name="copy"
          class="server-host"
          @click.stop="emit('copy-text', server.host, '服务器地址')"
        >
          <span class="cs-text-mono">{{ server.host }}</span>
        </CsButton>
      </div>

      <div class="server-meta">
        <span class="server-meta-item">
          <span class="server-label">心跳</span>
          <span class="cs-text-mono server-meta-value">{{ heartbeat }}</span>
        </span>
        <span class="server-meta-item">
          <span class="server-label">Agent</span>
          <span class="cs-text-mono server-meta-value">{{ server.agent_version || '—' }}</span>
        </span>
        <span class="server-meta-item">
          <span class="server-label">Xray</span>
          <span class="cs-text-mono server-meta-value">{{ server.xray_version || '—' }}</span>
        </span>
        <span class="server-meta-item">
          <CsTooltip :content="connsTip">
            <span class="server-label">连接</span>
            <span class="cs-text-mono server-meta-value server-meta-conns">{{ connsText }}</span>
          </CsTooltip>
        </span>
        <span class="server-meta-item">
          <span class="server-label">入站</span>
          <CsButton variant="inline-link" class="server-inbound-link" @click.stop="emit('go-inbounds')">
            <span class="cs-text-mono">{{ inboundCount ?? 0 }}</span> 个
          </CsButton>
        </span>
      </div>

      <div v-if="hasAsset" class="server-asset">
        <!-- 到期提醒跟着到期日本身走：日期变色 + 剩余天数徽章放在同一处，不再散落到状态条 -->
        <span v-if="server.expire_at" class="server-expiry">
          <span
            class="cs-text-mono"
            :style="expiry?.isUrgent ? { color: expiry.color, fontWeight: 600 } : {}"
            :title="expiry?.tip || `到期日：${formatExpireDate(server.expire_at)}`"
          >
            {{ formatExpireDate(server.expire_at) }}
          </span>
          <CsTooltip v-if="expiry?.isUrgent" :content="expiry.tip">
            <CsBadge :color="expiryBadgeColor" class="server-chip-help">{{ expiry.text }}</CsBadge>
          </CsTooltip>
        </span>
        <CsBadge v-if="server.billing_cycle" color="grey">{{ server.billing_cycle }}</CsBadge>
        <span v-if="server.price" class="cs-text-mono server-price">{{ server.price }}</span>
        <CsButton
          v-if="server.idc_address && isUrl(server.idc_address)"
          variant="inline-link"
          icon-name="external"
          :href="normalizeUrl(server.idc_address)"
          target="_blank"
          @click.stop
        >
          IDC 控制台
        </CsButton>
        <CsButton
          v-else-if="server.idc_address"
          variant="inline-link"
          icon-name="copy"
          @click.stop="emit('copy-text', server.idc_address || '', 'IDC 地址')"
        >
          {{ server.idc_address }}
        </CsButton>
      </div>
    </div>

    <template #footer>
      <CsButton icon-name="settings" @click.stop="emit('open-settings')">设置</CsButton>
      <CsButton icon-name="refresh" @click.stop="emit('restart')">重启</CsButton>
      <CsButton icon-name="view-horizontal" @click.stop="emit('metrics')">监控</CsButton>
    </template>
  </CsCard>
</template>

<style scoped lang="scss">
.server-card {
  position: relative;
  transition: box-shadow 0.18s ease, opacity 0.12s ease;
}

.server-card--dragging {
  opacity: 0.45;
}

.server-card--selected,
.server-card--select-mode.server-card--selected,
.server-card--select-mode.server-card--selected:hover {
  // 选中只靠边缘发光，不放勾选标记。厂商 item-card 的 box-shadow 规则带 :not(#\9)（ID 级优先级）
  // 且值恒为 none，不加 !important 会被整条吃掉。
  box-shadow:
    0 0 0 2px var(--color-text-link-default-qabnuk, #0972d3),
    0 0 18px 4px rgba(9, 114, 211, 0.45) !important;
}

/* 多选模式：整卡可点，卡片内部的链接/按钮不再各自响应点击。 */
.server-card--select-mode {
  cursor: pointer;
  user-select: none;

  &:hover {
    box-shadow: var(--shadow-item-card-hover, 0 2px 8px rgba(0, 0, 0, 0.08)) !important;
  }

  :deep(.cs-card__header),
  :deep(.cs-card__body) {
    pointer-events: none;
  }
}

.server-card-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  min-inline-size: 0;
}

.server-name {
  min-inline-size: 0;
  padding: 0;
  border: none;
  background: none;
  font-family: inherit;
  font-size: 20px;
  line-height: 28px;
  font-weight: 700;
  color: var(--color-text-link-default-qabnuk, #0972d3);
  text-align: start;
  overflow-wrap: anywhere;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
}

.server-status-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  font-size: 14px;
  line-height: 22px;
}

.server-status-sep {
  flex: none;
  inline-size: 1px;
  block-size: 14px;
  background: var(--color-border-divider-default-diwd4w, #dedee3);
}

.server-chip-help {
  cursor: help;
}

/* 分隔符：纯排版元素，不属于上下任何一块。详情区里上下留白交给 .server-details 的 gap，
   自身不再带外边距，否则 12px gap 会再叠一层。 */
.server-divider {
  margin-block: 12px;
  border-block-start: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
}

.server-details > .server-divider {
  margin-block: 0;
}

.server-details {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-inline-size: 0;
}

.server-host-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-inline-size: 0;
}

.server-label {
  flex: none;
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-host {
  min-inline-size: 0;
  font-size: 14px;
  text-align: start;
  overflow-wrap: anywhere;
}

.server-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 22px;
  min-inline-size: 0;
}

.server-meta-item {
  display: inline-flex;
  align-items: baseline;
  gap: 8px;
  min-inline-size: 0;
}

.server-meta-value {
  font-size: 14px;
  line-height: 18px;
}

/* Tooltip 触发层是 inline-flex 容器，父级 .server-meta-item 的 gap 进不到内部 */
.server-meta-conns {
  margin-inline-start: 8px;
}

.server-inbound-link {
  font-size: 14px;
}

/* cs-text-mono 自带 12px/16px：卡内所有「数值」都按所在行的字号来，否则同样是一行里的读数，
   地址、接入点数、到期日、价格会比心跳/版本小一号，整卡字号读起来不齐。 */
.server-host,
.server-inbound-link,
.server-expiry {
  .cs-text-mono {
    font-size: 14px;
    line-height: 18px;
  }
}

/* 周期流量（网卡整机口径）：块本身不带内外边距与分隔线，与相邻块的分隔交给独立的
   .server-divider 元素 + .server-details 的 12px gap，间距口径全卡一致。 */
.server-traffic {
  min-inline-size: 0;
}

.traffic-head {
  inline-size: 100%;
  justify-content: space-between;
  gap: 8px;
  /* 进度条是紧凑模式（容器高度 auto），条形直接贴着头行，留白只由这里给。 */
  margin-block-end: 10px;
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.traffic-label {
  flex: none;
}

.traffic-value {
  min-inline-size: 0;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-default-b2us04, #0f141a);
  text-align: end;
}

.traffic-percent {
  margin-inline-start: 2px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

/* 额度无穷：∞ 在等宽字体里又小又偏下，改正文字体并提一档，否则它比读数矮一截还贴基线。 */
.traffic-quota--inf {
  font-family: var(--font-family-base-c9u5cr, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
  font-size: 16px;
}

.traffic-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 4px 12px;
  margin-block-start: 6px;
}

.traffic-split,
.traffic-reset {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 13px;
  line-height: 18px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.traffic-split {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px 12px;
}

.traffic-reset {
  margin-inline-start: auto;
  text-align: end;
}

.server-asset {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  font-size: 14px;
}

.server-expiry {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.server-price {
  font-size: 14px;
  line-height: 18px;
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

/* 卡片自身三段的内边距：默认 header→body 只留 4px，字变大后挤在一起，
   这里统一放大，让「标题 / 状态+信息 / 操作」三段呼吸开。 */
:deep(.cs-card__header) {
  padding-block: 18px 10px !important;
}

:deep(.cs-card__body) {
  padding-block: 16px !important;
}

:deep(.cs-card__footer) {
  padding-block: 12px 18px !important;
}

:deep(.cs-card__footer button) {
  font-size: 14px !important;
}
</style>
