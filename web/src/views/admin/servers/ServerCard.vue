<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsIcon,
  CsStatusIndicator,
  CsTooltip,
  type BadgeColor,
} from '@/components/cloudscape'
import type { ServerItem } from '@/api/admin'
import { compareVersion } from '@/utils/version'
import { formatDateTime } from '@/utils/timezone'
import { formatExpireDate, getExpiryStatus, isUrl, normalizeUrl } from '@/utils/vps'
import { indicatorTypeOf, pushStateChip, xrayChip } from '@/utils/server-status'

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

const online = computed(() => props.server.status === 1)
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

const heartbeat = computed(() =>
  props.server.last_seen_at ? formatDateTime(props.server.last_seen_at, '—').slice(5, 16) : '—',
)

const hasAsset = computed(
  () =>
    Boolean(props.server.expire_at) ||
    Boolean(props.server.billing_cycle) ||
    Boolean(props.server.price) ||
    Boolean(props.server.idc_address),
)

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
    :data-server-id="server.id"
    @click="onRootClick"
  >
    <template #header>
      <div class="server-card-head">
        <span
          v-if="canReorder"
          class="server-drag-handle"
          draggable="true"
          data-drag-handle
          title="拖拽调整服务器顺序"
          @dragstart="dragging = true"
          @dragend="dragging = false"
        >
          <CsIcon name="drag-indicator" />
        </span>
        <button type="button" class="server-name" @click.stop="emit('open-settings')">{{ server.name }}</button>
        <CsBadge v-if="server.location" color="blue">{{ server.location }}</CsBadge>
      </div>
    </template>

    <!-- 状态条：三项常显 + 待办追加，固定顺序便于扫读 -->
    <div class="server-status-bar">
      <CsStatusIndicator :type="online ? 'success' : 'stopped'" :text="online ? '在线' : '离线'" />
      <span class="server-status-sep" aria-hidden="true" />
      <CsTooltip :content="xray.tip">
        <CsStatusIndicator :type="indicatorTypeOf(xray.color)" :text="xray.text" />
      </CsTooltip>
      <span class="server-status-sep" aria-hidden="true" />
      <CsTooltip :content="push.tip">
        <CsStatusIndicator :type="indicatorTypeOf(push.color)" :text="push.text" />
      </CsTooltip>
      <CsBadge v-if="agentOutdated" color="severity-medium" :title="`官方最新 ${latestAgentVersion}`">
        Agent 有新版
      </CsBadge>
      <CsBadge v-if="xrayOutdated" color="severity-medium" :title="`官方最新 ${latestXrayVersion}`">
        Xray 有新版
      </CsBadge>
      <CsTooltip v-if="expiry?.isUrgent" :content="expiry.tip">
        <CsBadge :color="expiryBadgeColor" class="server-chip-help">{{ expiry.text }}</CsBadge>
      </CsTooltip>
    </div>

    <div class="server-divider" aria-hidden="true" />

    <div class="server-details">
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
          <span class="server-label">接入点</span>
          <CsButton variant="inline-link" class="server-inbound-link" @click.stop="emit('go-inbounds')">
            <span class="cs-text-mono">{{ inboundCount ?? 0 }}</span> 个
          </CsButton>
        </span>
      </div>

      <div v-if="hasAsset" class="server-asset">
        <span
          v-if="server.expire_at"
          class="cs-text-mono"
          :style="expiry?.isUrgent ? { color: expiry.color, fontWeight: 600 } : {}"
          :title="expiry?.tip || `到期日：${formatExpireDate(server.expire_at)}`"
        >
          {{ formatExpireDate(server.expire_at) }}
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

.server-drag-handle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: 4px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  cursor: grab;

  &:hover {
    color: var(--color-text-body-default-b2us04, #000716);
    background: var(--color-background-button-normal-hover-1ajrtx, rgba(0, 7, 22, 0.06));
  }

  &:active {
    cursor: grabbing;
  }
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

.server-divider {
  margin-block: 16px;
  border-block-start: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
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

.server-inbound-link {
  font-size: 14px;
}

.server-asset {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  font-size: 13.5px;
}

.server-price {
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
