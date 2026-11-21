<script setup lang="ts">
import { computed } from 'vue'
import {
  CsButton,
  CsCheckbox,
  CsIcon,
  CsStatusIndicator,
  CsTooltip,
  type BadgeColor,
} from '@/components/cloudscape'
import SortHandle from '@/components/base/SortHandle.vue'
import type { ServerItem, ServerNicTraffic } from '@/api/admin'
import { compareVersion } from '@/utils/version'
import { formatDateTime } from '@/utils/timezone'
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
  servers: ServerItem[]
  selectedIds: number[]
  allSelected: boolean
  indeterminate: boolean
  canReorder: boolean
  draggingId: number | null
  latestAgentVersion?: string
  latestXrayVersion?: string
  inboundCountMap: Record<number, number>
}>()

const emit = defineEmits<{
  (e: 'toggle-select', id: number, checked: boolean): void
  (e: 'toggle-select-all', checked: boolean): void
  (e: 'open-settings', row: ServerItem): void
  (e: 'restart', row: ServerItem): void
  (e: 'metrics', row: ServerItem): void
  (e: 'go-inbounds', row: ServerItem): void
  (e: 'copy-text', text: string, label: string): void
  (e: 'pointerdown', event: PointerEvent): void
}>()

const RING_RADIUS = 7.5
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS

function ringDashOffset(percent: number | undefined): number {
  const clamped = Math.min(100, Math.max(0, Number(percent) || 0))
  return RING_CIRCUMFERENCE * (1 - clamped / 100)
}

function versionAlertOf(row: ServerItem): { text: string; tip: string } | null {
  const names: string[] = []
  const detail: string[] = []
  if (props.latestAgentVersion && row.agent_version && compareVersion(row.agent_version, props.latestAgentVersion) < 0) {
    names.push('Agent')
    detail.push(`Agent ${row.agent_version} → ${props.latestAgentVersion}`)
  }
  if (props.latestXrayVersion && row.xray_version && compareVersion(row.xray_version, props.latestXrayVersion) < 0) {
    names.push('Xray')
    detail.push(`Xray ${row.xray_version} → ${props.latestXrayVersion}`)
  }
  if (!names.length) return null
  return { text: `${names.join('、')} 有新版`, tip: detail.join('；') }
}

function connsSummary(row: ServerItem): string {
  if ((row.conn_tcp_established ?? -1) < 0) return '—'
  return `TCP ${row.conn_tcp_established}, UDP ${row.conn_udp}`
}

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

function statusMetaOf(row: ServerItem) {
  const comm = onlineChip(row)
  const xray = xrayChip(row)
  const push = pushStateChip(row)
  const alert = versionAlertOf(row)
  const heartbeat = row.last_seen_at ? formatDateTime(row.last_seen_at, '—').slice(5, 16) : '—'

  const tipParts = [
    comm.tip || '与节点 Agent 连接正常',
    `Xray：${xray.text}`,
    `配置：${push.text}`,
    `心跳：${heartbeat}`,
    `Agent：${row.agent_version || '—'}`,
    `内核：${row.xray_version || '—'}`,
    `连接：${connsSummary(row)}`,
  ]

  const extraBadges: Array<{ text: string; tone: TagTone; tip: string }> = []
  if (row.status === 1 && xray.color !== 'green') {
    extraBadges.push({ text: xray.text, tone: toneOfColor(xray.color), tip: xray.tip })
  }
  if (row.status === 1 && push.color !== 'green' && push.color !== 'grey') {
    extraBadges.push({ text: push.text, tone: toneOfColor(push.color), tip: push.tip })
  }
  if (alert) {
    extraBadges.push({ text: alert.text, tone: 'warn', tip: alert.tip })
  }

  return { comm, extraBadges, tip: tipParts.join('；') }
}

function assetMetaOf(row: ServerItem) {
  const expiry = getExpiryStatus(row.expire_at)
  const cyclePrice = [row.billing_cycle, row.price].filter(Boolean).join(' ')
  const titleParts: string[] = []
  if (expiry?.tip) titleParts.push(expiry.tip)
  else if (row.expire_at) titleParts.push(`到期日：${formatExpireDate(row.expire_at)}`)
  if (cyclePrice) titleParts.push(cyclePrice)

  return {
    expiry,
    cyclePrice,
    title: titleParts.join('；') || undefined,
    hasAny: Boolean(row.expire_at || cyclePrice || row.idc_address),
  }
}

function trafficMetaOf(nic: ServerNicTraffic | undefined) {
  const hasData = hasNicTrafficData(nic)
  const parts = nic && hasData ? nicUsageParts(nic) : null
  const color = nic ? nicQuotaColor(nic) : undefined
  return {
    nic,
    hasData,
    parts,
    tip: nicTrafficTooltip(nic),
    colorStyle: color ? { color } : undefined,
    strokeStyle: color ? { stroke: color } : undefined,
    dashOffset: nic ? ringDashOffset(nic.quota_percent) : RING_CIRCUMFERENCE,
  }
}

const rows = computed(() =>
  props.servers.map((row) => ({
    row,
    selected: props.selectedIds.includes(row.id),
    status: statusMetaOf(row),
    traffic: trafficMetaOf(nicTrafficOf(row)),
    asset: assetMetaOf(row),
    inboundCount: props.inboundCountMap[row.id] ?? 0,
  })),
)
</script>

<template>
  <div
    class="server-table-wrap"
    @pointerdown="emit('pointerdown', $event)"
    @dragstart.prevent
  >
    <table
      class="server-table"
      :class="{ 'server-table--dragging': draggingId !== null }"
      aria-label="服务器列表"
    >
      <thead>
        <tr>
          <th class="col-select">
            <CsCheckbox
              :model-value="allSelected"
              :indeterminate="indeterminate"
              aria-label="全选服务器"
              @change="(v: boolean) => emit('toggle-select-all', !!v)"
            />
          </th>
          <th v-if="canReorder" class="col-sort" aria-label="拖拽排序" />
          <th>服务器</th>
          <th>状态</th>
          <th>周期流量</th>
          <th>到期/资产</th>
          <th class="col-inbounds">入站数</th>
          <th class="col-actions">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="item in rows"
          :key="item.row.id"
          class="server-row"
          :class="{
            'server-row--selected': item.selected,
            'server-row--dragging': draggingId === item.row.id,
          }"
          :data-sort-id="item.row.id"
        >
          <td class="col-select">
            <CsCheckbox
              :model-value="item.selected"
              :aria-label="`选择服务器 ${item.row.name}`"
              @change="(v: boolean) => emit('toggle-select', item.row.id, !!v)"
            />
          </td>

          <td v-if="canReorder" class="col-sort">
            <SortHandle :draggable="false" title="拖拽调整服务器顺序" />
          </td>

          <td>
            <div class="server-cell">
              <div class="server-name-line">
                <button
                  type="button"
                  class="server-name-btn"
                  :title="item.row.remark ? `备注：${item.row.remark}` : '点击打开服务器设置'"
                  @click="emit('open-settings', item.row)"
                >
                  {{ item.row.name }}
                </button>
                <span v-if="item.row.location" class="soft-tag soft-tag--info">{{ item.row.location }}</span>
              </div>
              <button
                type="button"
                class="server-host-btn cell-mono"
                :title="`点击复制服务器地址：${item.row.host}`"
                @click="emit('copy-text', item.row.host, '服务器地址')"
              >
                <span class="server-host-text">{{ item.row.host }}</span>
                <CsIcon name="copy" size="small" class="host-copy-icon" />
              </button>
            </div>
          </td>

          <td>
            <div class="status-cell">
              <CsTooltip :content="item.status.tip">
                <CsStatusIndicator
                  :type="indicatorTypeOf(item.status.comm.color)"
                  :text="item.status.comm.text"
                />
              </CsTooltip>
              <CsTooltip v-for="b in item.status.extraBadges" :key="b.text" :content="b.tip">
                <span class="soft-tag status-extra-badge" :class="`soft-tag--${b.tone}`">{{ b.text }}</span>
              </CsTooltip>
            </div>
          </td>

          <td>
            <CsTooltip :content="item.traffic.tip">
              <div v-if="item.traffic.nic && item.traffic.hasData && item.traffic.parts" class="traffic-cell">
                <svg
                  v-if="item.traffic.nic.quota_bytes > 0"
                  class="traffic-ring"
                  viewBox="0 0 20 20"
                  aria-hidden="true"
                >
                  <circle class="traffic-ring-track" cx="10" cy="10" r="7.5" />
                  <circle
                    v-if="item.traffic.nic.quota_used_bytes > 0"
                    class="traffic-ring-fill"
                    cx="10"
                    cy="10"
                    r="7.5"
                    transform="rotate(-90 10 10)"
                    :stroke-dasharray="RING_CIRCUMFERENCE"
                    :stroke-dashoffset="item.traffic.dashOffset"
                    :style="item.traffic.strokeStyle"
                  />
                </svg>
                <span class="traffic-text cell-mono">
                  <strong :style="item.traffic.colorStyle">{{ item.traffic.parts.used }}</strong>
                  <span> / </span>
                  <span v-if="item.traffic.nic.quota_bytes > 0">{{ item.traffic.parts.quota }}</span>
                  <span v-else class="traffic-inf">∞</span>
                </span>
              </div>
              <span v-else class="cell-muted">—</span>
            </CsTooltip>
          </td>

          <td>
            <div v-if="item.asset.hasAny" class="asset-cell">
              <div v-if="item.row.expire_at" class="expire-stack" :title="item.asset.title">
                <span
                  class="expire-date cell-mono"
                  :style="item.asset.expiry?.isUrgent ? { color: item.asset.expiry.color, fontWeight: 600 } : undefined"
                >
                  {{ formatExpireDate(item.row.expire_at) }}
                </span>
                <CsTooltip v-if="item.asset.expiry?.isUrgent" :content="item.asset.expiry.tip">
                  <span class="expire-note" :style="{ color: item.asset.expiry.color }">
                    {{ item.asset.expiry.text }}
                  </span>
                </CsTooltip>
              </div>
              <span
                v-else-if="item.asset.cyclePrice"
                class="asset-cycle-text"
                :title="item.asset.title"
              >
                {{ item.asset.cyclePrice }}
              </span>
              <CsButton
                v-if="item.row.idc_address && isUrl(item.row.idc_address)"
                variant="inline-link"
                icon-name="external"
                :href="normalizeUrl(item.row.idc_address)"
                target="_blank"
                title="打开 IDC 控制台"
              >
                IDC
              </CsButton>
              <CsButton
                v-else-if="item.row.idc_address"
                variant="inline-link"
                icon-name="copy"
                :title="`复制 IDC 地址：${item.row.idc_address}`"
                @click="emit('copy-text', item.row.idc_address || '', 'IDC 地址')"
              >
                IDC
              </CsButton>
            </div>
            <span v-else class="cell-muted">—</span>
          </td>

          <td class="col-inbounds">
            <CsButton variant="inline-link" title="前往入站管理查看" @click="emit('go-inbounds', item.row)">
              <span class="cell-mono">{{ item.inboundCount }}</span>
            </CsButton>
          </td>

          <td class="col-actions">
            <div class="row-actions">
              <CsButton :wrap-text="false" @click="emit('metrics', item.row)">监控</CsButton>
              <CsButton :wrap-text="false" @click="emit('open-settings', item.row)">设置</CsButton>
              <CsButton :wrap-text="false" @click="emit('restart', item.row)">重启</CsButton>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped lang="scss">
.server-table-wrap {
  width: 100%;
  overflow-x: auto;
}

.server-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-size: 13px;

  th {
    padding: 8px 10px;
    text-align: left;
    font-size: 12px;
    font-weight: 700;
    color: var(--color-text-label-g80qnu, #414d5c);
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
    white-space: nowrap;
  }

  td {
    padding: 10px;
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
    vertical-align: middle;
  }

  .col-select {
    width: 1%;
    padding-inline: 8px 4px;
    white-space: nowrap;
  }

  .col-sort {
    width: 1%;
    padding-inline: 2px 4px;
    text-align: center;
    white-space: nowrap;
  }

  .col-inbounds {
    width: 1%;
    white-space: nowrap;
  }

  .col-actions {
    width: 1%;
    white-space: nowrap;
  }
}

.server-table--dragging {
  user-select: none;
  cursor: grabbing;

  .server-row:not(.server-row--dragging):hover {
    background: transparent;
  }
}

.server-row {
  transition: background-color 0.15s ease;

  &:hover {
    background: var(--color-background-dropdown-item-hover-pvfsrg, rgba(0, 7, 22, 0.04));
  }

  &--selected {
    background: var(--x-primary-soft, #f0fbff);
  }

  &--dragging {
    position: relative;
    z-index: 10;
    background: var(--color-background-container-content-mjfil9, #ffffff);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.14);
    will-change: transform;
    transition: none;
  }
}

.server-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.server-name-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.server-name-btn {
  padding: 0;
  border: 0;
  background: transparent;
  font-family: inherit;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  text-align: start;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
}

.server-host-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  border: 0;
  background: transparent;
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  text-align: start;
  cursor: pointer;
  width: fit-content;
  max-width: 100%;

  .host-copy-icon {
    opacity: 0;
    transition: opacity 0.15s ease;
    flex: none;
  }

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);

    .host-copy-icon {
      opacity: 1;
    }
  }
}

.server-host-text {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.status-extra-badge {
  cursor: help;
}

.traffic-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  vertical-align: middle;
  white-space: nowrap;
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
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.traffic-inf {
  font-family: var(--font-family-base-c9u5cr, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
  font-size: 14px;
}

.asset-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.expire-stack {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.expire-date {
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.expire-note {
  font-size: 12px;
  line-height: 16px;
  font-weight: 500;
  cursor: help;
}

.asset-cycle-text {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
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
  color: var(--color-text-link-default-qabnuk, #006ce0);
  background: var(--x-primary-soft, #f0fbff);
  border-color: rgba(0, 108, 224, 0.25);
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

.row-actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
</style>
