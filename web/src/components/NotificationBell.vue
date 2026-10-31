<script setup lang="ts">
/**
 * 管理端顶栏提醒铃铛。
 *
 * 提醒是派生量（服务器到期、Agent/面板版本落后），后端按当前状态现算，不落库；
 * 「忽略」才有记录，且带条件指纹——条件升级时会重新出现。这里只负责轮询、展示和跳转。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CsBadge, CsIcon, CsPopover, type BadgeColor } from '@/components/cloudscape'
import { dismissNotification, getNotifications } from '@/api/admin'
import type { AdminNotificationItem } from '@/api/types'

// 与服务端版本探测同量级即可：提醒的最短变化粒度是「天」和版本号，不必高频。
const POLL_INTERVAL_MS = 60_000

const SEVERITY_LABEL: Record<AdminNotificationItem['severity'], string> = {
  critical: '严重',
  high: '重要',
  medium: '注意',
  low: '提示',
}

const router = useRouter()
const open = ref(false)
const items = ref<AdminNotificationItem[]>([])
const loading = ref(false)
const dismissing = ref<string | null>(null)

const total = computed(() => items.value.length)

const topSeverity = computed<AdminNotificationItem['severity'] | null>(() => {
  const rank = { critical: 0, high: 1, medium: 2, low: 3 } as const
  let best: AdminNotificationItem['severity'] | null = null
  for (const item of items.value) {
    if (!best || rank[item.severity] < rank[best]) best = item.severity
  }
  return best
})

const bellLabel = computed(() => (total.value > 0 ? `${total.value} 条待处理提醒` : '暂无提醒'))

function badgeColor(severity: AdminNotificationItem['severity']): BadgeColor {
  return `severity-${severity}` as BadgeColor
}

let timer: number | undefined

async function load() {
  if (loading.value) return
  loading.value = true
  try {
    const { data } = await getNotifications()
    if (data.code === 0) items.value = data.data?.items ?? []
  } catch {
    // 轮询失败不打扰管理员：保留上一次结果，等下一轮
  } finally {
    loading.value = false
  }
}

async function dismiss(item: AdminNotificationItem) {
  if (dismissing.value) return
  dismissing.value = item.key
  const previous = items.value
  // 乐观移除：条件未变化时后端也不会再返回这一条
  items.value = items.value.filter((it) => it.key !== item.key)
  try {
    await dismissNotification(item.key)
  } catch {
    items.value = previous
  } finally {
    dismissing.value = null
  }
}

function go(item: AdminNotificationItem) {
  open.value = false
  if (item.action_path) void router.push(item.action_path)
}

function onVisibilityChange() {
  if (document.visibilityState === 'visible') void load()
}

onMounted(() => {
  void load()
  timer = window.setInterval(() => {
    if (document.visibilityState === 'visible') void load()
  }, POLL_INTERVAL_MS)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <CsPopover
    v-model:visible="open"
    placement="bottom-end"
    width="min(380px, calc(100vw - 16px))"
  >
    <template #reference>
      <button
        type="button"
        class="notif-bell"
        :class="{ 'is-active': open }"
        :aria-label="bellLabel"
        :aria-expanded="open"
        :title="bellLabel"
      >
        <CsIcon name="notification" size="normal" />
        <span v-if="topSeverity" class="notif-dot" :class="`is-${topSeverity}`" aria-hidden="true" />
      </button>
    </template>

    <div class="notif-panel">
      <div class="notif-head">
        <strong>提醒</strong>
        <span v-if="total > 0" class="notif-count">{{ total }}</span>
      </div>

      <p v-if="total === 0" class="notif-empty">暂无提醒</p>

      <ul v-else class="notif-list">
        <li v-for="item in items" :key="item.key" class="notif-item">
          <button type="button" class="notif-main" @click="go(item)">
            <span class="notif-line">
              <CsBadge :color="badgeColor(item.severity)">{{ SEVERITY_LABEL[item.severity] }}</CsBadge>
              <span class="notif-title" :title="item.title">{{ item.title }}</span>
            </span>
            <span class="notif-detail" :title="item.detail">{{ item.detail }}</span>
            <span class="notif-action">{{ item.action_label }} →</span>
          </button>
          <button
            type="button"
            class="notif-dismiss"
            :disabled="dismissing === item.key"
            title="忽略这条提醒"
            aria-label="忽略这条提醒"
            @click="dismiss(item)"
          >
            ×
          </button>
        </li>
      </ul>
    </div>
  </CsPopover>
</template>

<style scoped lang="scss">
.notif-bell {
  position: relative;
  inline-size: 32px;
  block-size: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: transparent;
  color: inherit;
  cursor: pointer;
  padding: 0;

  &:hover,
  &.is-active {
    background: var(--color-background-dropdown-item-hover-0d9c28, #f0f1f4);
  }

  &:focus-visible {
    outline: 2px solid var(--color-border-item-focused-sewkzn, #006ce0);
    outline-offset: 2px;
  }
}

/* 未读即「有提醒」：颜色跟随最高严重度，形状用圆点而不是数字角标（Cloudscape 顶栏惯例）。 */
.notif-dot {
  position: absolute;
  inset-block-start: 5px;
  inset-inline-end: 5px;
  inline-size: 7px;
  block-size: 7px;
  border-radius: 50%;
  border: 1px solid var(--color-background-container-content-mjfil9, #ffffff);

  &.is-critical {
    background: var(--color-background-status-error-edh6bo, #db0000);
  }

  &.is-high {
    background: var(--color-background-status-warning-fo5lrs, #eb6f0f);
  }

  &.is-medium {
    background: var(--color-background-status-info-r8vz7c, #006ce0);
  }

  &.is-low {
    background: var(--color-background-status-neutral-g13zeq, #687078);
  }
}

.notif-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.notif-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-block-end: 8px;
  border-block-end: 1px solid var(--color-border-divider-default-oq3to1, #e9ebed);

  strong {
    font-size: 14px;
    font-weight: 700;
  }
}

.notif-count {
  min-inline-size: 20px;
  padding-inline: 6px;
  border-radius: 10px;
  background: var(--color-background-badge-grey-2psh1z, #eff1f2);
  color: var(--color-text-body-secondary-9gdsa4, #5f6b7a);
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}

.notif-empty {
  margin: 4px 0 4px;
  color: var(--color-text-body-secondary-9gdsa4, #5f6b7a);
  font-size: 13px;
}

.notif-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-block-size: min(60vh, 420px);
  overflow-y: auto;
  margin: 0;
  padding: 0;
  list-style: none;
}

.notif-item {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  border-radius: 6px;

  &:hover {
    background: var(--color-background-dropdown-item-hover-0d9c28, #f0f1f4);
  }
}

.notif-main {
  flex: 1 1 auto;
  min-inline-size: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 8px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: start;
  cursor: pointer;
  border-radius: 6px;

  &:focus-visible {
    outline: 2px solid var(--color-border-item-focused-sewkzn, #006ce0);
    outline-offset: -2px;
  }
}

.notif-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-inline-size: 0;
}

.notif-title {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notif-detail {
  font-size: 12px;
  line-height: 18px;
  color: var(--color-text-body-secondary-9gdsa4, #5f6b7a);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.notif-action {
  font-size: 12px;
  color: var(--color-text-link-default-urk3oy, #006ce0);
}

.notif-dismiss {
  flex: none;
  inline-size: 24px;
  block-size: 24px;
  margin-block-start: 6px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-body-secondary-9gdsa4, #5f6b7a);
  font-size: 16px;
  line-height: 1;
  cursor: pointer;

  &:hover {
    background: var(--color-background-dropdown-item-hover-0d9c28, #f0f1f4);
    color: var(--color-text-body-default-b2us04, #0f141a);
  }

  &:disabled {
    opacity: 0.5;
    cursor: default;
  }
}
</style>
