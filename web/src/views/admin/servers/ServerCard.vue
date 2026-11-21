<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CsButton,
  CsCard,
  CsStatusIndicator,
  CsTooltip,
  type BadgeColor,
} from '@/components/cloudscape'
import SortHandle from '@/components/base/SortHandle.vue'
import type { ServerItem } from '@/api/admin'
import { compareVersion } from '@/utils/version'
import { formatExpireDate, getExpiryStatus, isUrl, normalizeUrl } from '@/utils/vps'
import { indicatorTypeOf, onlineChip, pushStateChip, xrayChip } from '@/utils/server-status'
import {
  hasNicTrafficData,
  nicQuotaColor,
  nicTrafficOf,
  nicTrafficTooltip,
  nicUsageParts,
} from '@/utils/server-nic-traffic'

const props = defineProps<{
  server: ServerItem
  selected?: boolean
  selectMode?: boolean
  /** 是否允许拖拽排序（筛选/搜索/多选时为 false）。 */
  canReorder?: boolean
  latestAgentVersion?: string
  latestXrayVersion?: string
}>()

const emit = defineEmits<{
  (e: 'toggle-select'): void
  (e: 'open-settings'): void
  (e: 'restart'): void
  (e: 'metrics'): void
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

const statusTip = computed(() =>
  [
    comm.value.tip || '与节点 Agent 连接正常',
    `Xray：${xray.value.text}`,
    `配置：${push.value.text}`,
  ].join('；'),
)

type TagTone = 'info' | 'warn' | 'error' | 'neutral'

function toneOfColor(color: BadgeColor): TagTone {
  switch (color) {
    case 'red':
    case 'severity-critical':
    case 'severity-high':
      return 'error'
    case 'severity-medium':
    case 'severity-low':
      return 'warn'
    case 'blue':
      return 'info'
    default:
      return 'neutral'
  }
}

const extraBadge = computed<{ text: string; tone: TagTone; tip: string } | null>(() => {
  if (props.server.status === 1 && xray.value.color !== 'green') {
    return { text: xray.value.text, tone: toneOfColor(xray.value.color), tip: xray.value.tip }
  }
  if (props.server.status === 1 && push.value.color !== 'green' && push.value.color !== 'grey') {
    return { text: push.value.text, tone: toneOfColor(push.value.color), tip: push.value.tip }
  }
  if (versionAlert.value) {
    return { text: versionAlert.value.text, tone: 'warn', tip: versionAlert.value.tip }
  }
  return null
})

// 周期流量圆圈（复用用户列表 18px 环形 SVG 语义）
const RING_RADIUS = 7.5
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS

const nic = computed(() => nicTrafficOf(props.server))
const nicHasData = computed(() => hasNicTrafficData(nic.value))
const nicUsage = computed(() =>
  nic.value && nicHasData.value ? nicUsageParts(nic.value) : { used: '—', quota: '' },
)
const nicUnlimited = computed(() => Boolean(nic.value && nic.value.quota_bytes <= 0))
const nicBarColor = computed(() => nicQuotaColor(nic.value))
const nicTip = computed(() => nicTrafficTooltip(nic.value))
const ringDashOffset = computed(() => {
  if (!nic.value || nic.value.quota_bytes <= 0) return RING_CIRCUMFERENCE
  const clamped = Math.min(100, Math.max(0, Number(nic.value.quota_percent) || 0))
  return RING_CIRCUMFERENCE * (1 - clamped / 100)
})

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
        <span v-if="server.location" class="soft-tag soft-tag--info">{{ server.location }}</span>
      </div>
    </template>

    <template #actions>
      <div class="server-card-status">
        <CsTooltip :content="statusTip">
          <CsStatusIndicator :type="indicatorTypeOf(comm.color)" :text="comm.text" />
        </CsTooltip>
        <CsTooltip v-if="extraBadge" :content="extraBadge.tip">
          <span class="soft-tag server-chip-help" :class="`soft-tag--${extraBadge.tone}`">{{ extraBadge.text }}</span>
        </CsTooltip>
      </div>
    </template>

    <div class="server-card-body">
      <div class="server-traffic-row">
        <span class="server-field-label">周期流量</span>
        <CsTooltip :content="nicTip">
          <div v-if="nic && nicHasData" class="traffic-cell">
            <svg
              v-if="nic.quota_bytes > 0"
              class="traffic-ring"
              viewBox="0 0 20 20"
              aria-hidden="true"
            >
              <circle class="traffic-ring-track" cx="10" cy="10" r="7.5" />
              <circle
                v-if="nic.quota_used_bytes > 0"
                class="traffic-ring-fill"
                cx="10"
                cy="10"
                r="7.5"
                transform="rotate(-90 10 10)"
                :stroke-dasharray="RING_CIRCUMFERENCE"
                :stroke-dashoffset="ringDashOffset"
                :style="nicBarColor ? { stroke: nicBarColor } : undefined"
              />
            </svg>
            <span class="traffic-text cell-mono">
              <strong :style="nicBarColor ? { color: nicBarColor } : undefined">{{ nicUsage.used }}</strong>
              <span> / </span>
              <span v-if="!nicUnlimited">{{ nicUsage.quota }}</span>
              <span v-else class="traffic-inf">∞</span>
            </span>
          </div>
          <span v-else class="cell-muted">—</span>
        </CsTooltip>
      </div>

      <div v-if="server.expire_at || server.idc_address" class="server-asset-row">
        <span v-if="server.expire_at" class="server-expiry">
          <span class="server-field-label">到期</span>
          <span
            class="cell-mono expiry-date"
            :style="expiry?.isUrgent ? { color: expiry.color, fontWeight: 600 } : undefined"
            :title="expiry?.tip || `到期日：${formatExpireDate(server.expire_at)}`"
          >
            {{ formatExpireDate(server.expire_at) }}
          </span>
          <CsTooltip v-if="expiry?.isUrgent" :content="expiry.tip">
            <span class="expiry-note server-chip-help" :style="{ color: expiry.color }">{{ expiry.text }}</span>
          </CsTooltip>
        </span>

        <span v-if="server.idc_address" class="server-idc">
          <CsButton
            v-if="isUrl(server.idc_address)"
            variant="inline-link"
            icon-name="external"
            :href="normalizeUrl(server.idc_address)"
            target="_blank"
            @click.stop
          >
            IDC 控制台
          </CsButton>
          <CsButton
            v-else
            variant="inline-link"
            icon-name="copy"
            @click.stop="emit('copy-text', server.idc_address || '', 'IDC 地址')"
          >
            {{ server.idc_address }}
          </CsButton>
        </span>
      </div>
    </div>

    <template #footer>
      <CsButton :wrap-text="false" @click.stop="emit('metrics')">监控</CsButton>
      <CsButton :wrap-text="false" @click.stop="emit('open-settings')">设置</CsButton>
      <CsButton :wrap-text="false" @click.stop="emit('restart')">重启</CsButton>
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
  gap: 6px 8px;
  min-inline-size: 0;
}

.server-name {
  min-inline-size: 0;
  padding: 0;
  border: none;
  background: none;
  font-family: inherit;
  font-size: 16px;
  line-height: 22px;
  font-weight: 700;
  color: var(--color-text-link-default-qabnuk, #0972d3);
  text-align: start;
  overflow-wrap: anywhere;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
}

.server-card-status {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  white-space: nowrap;
}

.server-chip-help {
  cursor: help;
}

.soft-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  font-size: 12px;
  font-weight: 500;
  line-height: 16px;
  white-space: nowrap;
  flex-shrink: 0;
}

.soft-tag--info {
  color: var(--x-tag-info-color, #006ce0);
  background: var(--x-tag-info-bg, #f0fbff);
  border-color: var(--x-tag-info-border, rgba(0, 108, 224, 0.25));
}

.soft-tag--warn {
  color: var(--color-text-status-warning-fabq6h, #8d6605);
  background: var(--x-warning-soft, #fffbeb);
  border-color: rgba(245, 158, 11, 0.3);
}

.soft-tag--error {
  color: var(--color-text-status-error-ri38gt, #db0000);
  background: var(--x-danger-soft, #fef2f2);
  border-color: rgba(239, 68, 68, 0.3);
}

.soft-tag--neutral {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  background: var(--x-card-soft, #f8fafc);
  border-color: var(--color-border-divider-default-diwd4w, #e9ebed);
}

.server-card-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-inline-size: 0;
}

.server-traffic-row,
.server-asset-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 6px 10px;
  min-inline-size: 0;
}

.server-field-label {
  flex: none;
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.traffic-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  vertical-align: middle;
}

.traffic-ring {
  width: 18px;
  height: 18px;
  flex: none;
}

.traffic-ring-track {
  fill: none;
  stroke: var(--color-border-divider-default-diwd4w, #e9ebed);
  stroke-width: 2.5;
}

.traffic-ring-fill {
  fill: none;
  stroke: var(--color-text-link-default-qabnuk, #006ce0);
  stroke-width: 2.5;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.2s ease;
}

.traffic-text {
  font-size: 14px;
  line-height: 20px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.traffic-inf {
  font-family: var(--font-family-base-c9u5cr, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
  font-size: 16px;
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.server-expiry {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.expiry-date {
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.expiry-note {
  font-size: 12px;
  line-height: 16px;
  font-weight: 500;
}

.server-idc {
  margin-inline-start: auto;
  font-size: 13px;
}

:deep(.cs-card__header) {
  padding-block: 14px 8px !important;
}

:deep(.cs-card__body) {
  padding-block: 8px 12px !important;
}

:deep(.cs-card__footer) {
  padding-block: 10px 14px !important;
}

:deep(.cs-card__footer button) {
  font-size: 13px !important;
}
</style>
