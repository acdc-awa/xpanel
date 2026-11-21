<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsColumnLayout,
  CsContainer,
  CsHeader,
  CsIcon,
  CsModal,
  CsProgressBar,
  CsSpaceBetween,
  CsStatusIndicator,
  toast,
} from '@/components/cloudscape'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { buildSubscribeUrl } from '@/config/site'
import { errMsg } from '@/api/http'
import { formatBytes, formatDate } from '@/utils/format'
import { renderMarkdown } from '@/utils/markdown'
import { getMyServers, getUserNotices, type MyServerItem } from '@/api/user'
import type { NoticeItem } from '@/api/types'

const router = useRouter()
const auth = useAuthStore()
const site = useSiteStore()
const loading = ref(false)

// plan_name 由 /user/me 等接口返回；套餐已删时为空，回退显示编号
const planLabel = computed(() => {
  if (auth.user?.plan_name) return `已购套餐 ${auth.user.plan_name}`
  if (auth.user?.plan_id) return `已购套餐 #${auth.user.plan_id}`
  return '暂无生效套餐'
})
const usedBytes = computed(() => auth.user?.used_bytes ?? 0)
const totalBytes = computed(() => auth.user?.total_bytes ?? 0)
const remainBytes = computed(() => Math.max(0, totalBytes.value - usedBytes.value))
const usagePercent = computed(() => {
  if (!totalBytes.value) return 0
  return Math.min(100, Math.round((usedBytes.value / totalBytes.value) * 100))
})
const totalText = computed(() => (totalBytes.value ? formatBytes(totalBytes.value) : '0 B'))
const remainText = computed(() => (totalBytes.value ? formatBytes(remainBytes.value) : '0 B'))
const usedText = computed(() => formatBytes(usedBytes.value))

const expireText = computed(() => {
  const t = auth.user?.expire_at
  // 到期退订后 plan_id 归零、expire_at 保留过去的到期时间：提示「已到期」而非「未开通」。
  if (!auth.user?.plan_id) return isExpired.value ? '已到期' : '未开通套餐'
  if (!t) return '永久有效'
  return formatDate(t)
})

const isExpired = computed(() => {
  const t = auth.user?.expire_at
  return !!t && new Date(t).getTime() < Date.now()
})

const isPlanActive = computed(() => {
  if (!auth.user?.plan_id) return false
  if (isExpired.value) return false
  return true
})

const isExhausted = computed(
  () => isPlanActive.value && totalBytes.value > 0 && usedBytes.value >= totalBytes.value,
)

const progressStatus = computed<'error' | 'in-progress'>(() => {
  if (usagePercent.value > 90) return 'error'
  return 'in-progress'
})

const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token, site.subscribeUrl, site.subscribePath) : ''
})

onMounted(async () => {
  loadServers()
  loadNotices()
  loading.value = true
  try {
    await auth.fetchMe()
  } catch (e) {
    toast.error(errMsg(e, '加载用户信息失败'))
  } finally {
    loading.value = false
  }
})

function copySub() {
  if (!subscribeUrl.value) {
    toast.warning('暂无可用订阅地址')
    return
  }
  navigator.clipboard?.writeText(subscribeUrl.value).then(
    () => toast.success('Mihomo 订阅地址已复制到剪贴板'),
    () => toast.warning('复制失败，请手动复制'),
  )
}

function importMihomo() {
  if (!subscribeUrl.value) {
    toast.warning('暂无可用订阅地址')
    return
  }
  const url = `clash://install-config?url=${encodeURIComponent(subscribeUrl.value)}&name=XrayPanel`
  window.location.href = url
  toast.info('正在唤醒 Mihomo / Clash 客户端，若未响应请手动复制订阅地址…')
}

const servers = ref<MyServerItem[]>([])

async function loadServers() {
  try {
    const { data } = await getMyServers()
    if (data.code === 0) {
      servers.value = data.data.items
    }
  } catch {
    servers.value = []
  }
}

// 公告与使用指南（同一接口返回，按 category 分流）
const notices = ref<NoticeItem[]>([])
const noticeLoading = ref(false)
const selectedNotice = ref<NoticeItem | null>(null)
const noticeModalOpen = ref(false)
const popupNotice = ref<NoticeItem | null>(null)
const popupOpen = ref(false)

const systemNotices = computed(() => notices.value.filter((n) => n.category !== 'guide'))
const guides = computed(() => notices.value.filter((n) => n.category === 'guide'))

async function loadNotices() {
  noticeLoading.value = true
  try {
    const { data } = await getUserNotices()
    if (data.code === 0) {
      notices.value = data.data
      checkPopupNotice(data.data)
    }
  } catch {
    notices.value = []
  } finally {
    noticeLoading.value = false
  }
}

function checkPopupNotice(list: NoticeItem[]) {
  const target = list.find((n) => n.is_popup)
  if (!target) return
  const key = `xpanel_popup_notice_${target.id}`
  const lastReadTime = localStorage.getItem(key)
  if (!lastReadTime || lastReadTime !== target.updated_at) {
    popupNotice.value = target
    popupOpen.value = true
  }
}

function handleClosePopup() {
  if (popupNotice.value) {
    const key = `xpanel_popup_notice_${popupNotice.value.id}`
    localStorage.setItem(key, popupNotice.value.updated_at)
  }
  popupOpen.value = false
}

function viewNotice(n: NoticeItem) {
  selectedNotice.value = n
  noticeModalOpen.value = true
}
</script>

<template>
  <CsSpaceBetween direction="vertical" size="l">
    <!-- 到期 / 流量耗尽 / 未开通提示 -->
    <CsAlert v-if="isExpired && auth.user?.plan_id" type="error" header="套餐已到期">
      当前套餐已到期，节点转发已暂停。请续费或重新订购套餐以恢复使用。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">前往续费/订购</CsButton>
      </template>
    </CsAlert>
    <CsAlert v-else-if="isExpired" type="error" header="套餐已到期">
      当前套餐已到期并已自动取消，节点转发已暂停。请重新订购套餐以恢复使用。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">前往订购</CsButton>
      </template>
    </CsAlert>
    <CsAlert v-else-if="isExhausted" type="error" header="流量已用尽">
      当前套餐流量已用尽，节点转发已暂停。续费或订购其他套餐后将重置流量并恢复使用。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">前往续费/订购</CsButton>
      </template>
    </CsAlert>
    <CsAlert v-else-if="!auth.user?.plan_id" type="info" header="尚未订购套餐">
      订购套餐后即可获取节点订阅并开始使用。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">选购套餐</CsButton>
      </template>
    </CsAlert>

    <div class="dash-grid">
      <!-- 左侧主栏：用量概览 + 快捷入口 -->
      <CsSpaceBetween direction="vertical" size="l">
        <CsContainer>
          <template #header>
            <CsHeader variant="h2">
              {{ planLabel }}
            </CsHeader>
          </template>

          <CsSpaceBetween direction="vertical" size="l">
            <CsColumnLayout :columns="3" variant="text-grid">
              <div class="stat-item">
                <div class="stat-label">已用流量</div>
                <div class="stat-value cell-mono">{{ loading ? '…' : usedText }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">剩余可用</div>
                <div class="stat-value cell-mono is-remain">{{ loading ? '…' : remainText }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">套餐总量</div>
                <div class="stat-value cell-mono">{{ loading ? '…' : totalText }}</div>
              </div>
            </CsColumnLayout>

            <CsProgressBar
              :value="usagePercent"
              :status="progressStatus"
              label="流量使用进度"
              :additional-info="`有效期：${expireText}`"
            />

            <div class="plan-actions">
              <CsButton variant="primary" icon-name="external" :wrap-text="false" @click="importMihomo">
                一键导入 Clash
              </CsButton>
              <CsButton icon-name="copy" :Clashwrap-text="false" @click="copySub">复制订阅地址</CsButton>
              <CsButton :wrap-text="false" @click="router.push('/shop')">选购/续费</CsButton>
            </div>
          </CsSpaceBetween>
        </CsContainer>

        <!-- 快捷入口 -->
        <div class="dash-tiles-grid">
          <CsContainer class="tile-card" @click="router.push('/subscribe')">
            <div class="tile-row">
              <div class="tile-icon">
                <CsIcon name="share" size="medium" />
              </div>
              <div class="tile-body">
                <div class="tile-title">订阅中心</div>
              </div>
            </div>
          </CsContainer>

          <CsContainer class="tile-card" @click="router.push('/shop')">
            <div class="tile-row">
              <div class="tile-icon">
                <CsIcon name="add-plus" size="medium" />
              </div>
              <div class="tile-body">
                <div class="tile-title">套餐商店</div>
              </div>
            </div>
          </CsContainer>
        </div>
      </CsSpaceBetween>

      <!-- 右侧栏：节点可用性 + 系统公告 + 使用指南 -->
      <CsSpaceBetween direction="vertical" size="l">
        <CsContainer>
          <template #header>
            <CsHeader variant="h2" >节点可用性</CsHeader>
          </template>
          <div v-if="servers.length" class="node-list">
            <div v-for="s in servers" :key="s.id" class="node-row">
              <div class="node-name">
                <span>{{ s.location || '未命名节点' }}</span>
                <span class="node-sub">{{ s.name }}</span>
              </div>
              <CsStatusIndicator :type="s.online ? 'success' : 'stopped'">
                {{ s.online ? '在线' : '离线' }}
              </CsStatusIndicator>
            </div>
          </div>
          <CsBox v-else color="text-body-secondary" font-size="body-s">
            暂无可用节点
          </CsBox>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2">系统公告</CsHeader>
          </template>
          <CsBox v-if="noticeLoading" color="text-body-secondary" font-size="body-s">
            加载公告中…
          </CsBox>
          <CsBox v-else-if="!systemNotices.length" color="text-body-secondary" font-size="body-s">
            暂无系统公告
          </CsBox>
          <div v-else class="notice-list">
            <button
              v-for="n in systemNotices"
              :key="n.id"
              type="button"
              class="notice-row"
              @click="viewNotice(n)"
            >
              <div class="notice-title">
                <CsBadge v-if="n.is_pinned" color="red">置顶</CsBadge>
                <span>{{ n.title }}</span>
              </div>
              <div class="notice-date cell-mono">{{ formatDate(n.created_at || n.updated_at) }}</div>
            </button>
          </div>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2">使用指南</CsHeader>
          </template>
          <CsBox v-if="noticeLoading" color="text-body-secondary" font-size="body-s">
            加载指南中…
          </CsBox>
          <CsBox v-else-if="!guides.length" color="text-body-secondary" font-size="body-s">
            暂无使用指南
          </CsBox>
          <div v-else class="notice-list">
            <button
              v-for="g in guides"
              :key="g.id"
              type="button"
              class="notice-row"
              @click="viewNotice(g)"
            >
              <div class="notice-title">
                <CsBadge v-if="g.is_pinned" color="red">置顶</CsBadge>
                <span>{{ g.title }}</span>
              </div>
            </button>
          </div>
        </CsContainer>
      </CsSpaceBetween>
    </div>

    <!-- 公告详情弹窗 -->
    <CsModal
      v-model:visible="noticeModalOpen"
      :header="selectedNotice?.title || '公告详情'"
      size="medium"
      :close-on-click-overlay="false"
    >
      <div v-if="selectedNotice" class="notice-detail">
        <div class="notice-meta">
          <CsBadge v-if="selectedNotice.is_pinned" color="red">置顶</CsBadge>
          <span class="notice-time">发布于 {{ formatDate(selectedNotice.created_at) }}</span>
        </div>
        <div class="notice-body markdown-content" v-html="renderMarkdown(selectedNotice.content)" />
      </div>
      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton variant="primary" @click="noticeModalOpen = false">我知道了</CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>

    <!-- 首页重要公告强提醒弹窗 -->
    <CsModal
      v-model:visible="popupOpen"
      :header="popupNotice?.title || '重要通知'"
      size="medium"
      :close-on-click-overlay="false"
      @update:visible="(v) => !v && handleClosePopup()"
    >
      <div v-if="popupNotice" class="notice-detail">
        <div class="notice-meta">
          <CsBadge color="blue">重要通知</CsBadge>
          <span class="notice-time">{{ formatDate(popupNotice.updated_at) }}</span>
        </div>
        <div class="notice-body markdown-content" v-html="renderMarkdown(popupNotice.content)" />
      </div>
      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton variant="primary" @click="handleClosePopup">我知道了</CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.dash-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.55fr) minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-label {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: var(--x-text);

  &.is-remain {
    color: var(--x-primary);
  }
}

.plan-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tile-card {
  cursor: pointer;
}

.dash-tiles-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.tile-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tile-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  inline-size: 40px;
  block-size: 40px;
  border-radius: 8px;
  background-color: var(--x-primary-soft);
  color: var(--x-primary);
  flex-shrink: 0;
}

.tile-body {
  min-inline-size: 0;
}

.tile-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text);
}

.node-list {
  display: flex;
  flex-direction: column;
}

.node-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--x-border);

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  &:first-child {
    padding-top: 0;
  }
}

.node-name {
  display: flex;
  flex-direction: column;
  min-inline-size: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text);
}

.node-sub {
  font-size: 12px;
  font-weight: 400;
  color: var(--x-text-secondary);
}

.notice-list {
  display: flex;
  flex-direction: column;
}

.notice-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  inline-size: 100%;
  padding: 10px 0;
  border: none;
  border-bottom: 1px solid var(--x-border);
  background: transparent;
  text-align: left;
  cursor: pointer;

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  &:first-child {
    padding-top: 0;
  }

  &:hover .notice-title {
    color: var(--x-primary);
  }
}

.notice-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text);
  transition: color 0.15s ease;
}

.notice-date {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.notice-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.notice-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.notice-time {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.notice-body {
  font-size: 14px;
  line-height: 1.65;
  color: var(--x-text);
}

@media (max-width: $bp-two-pane) {
  .dash-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: $bp-tablet-max) {
  .plan-actions {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));

    > :first-child {
      grid-column: 1 / -1;
    }

    > *:not(#\9) {
      inline-size: 100%;
      text-align: center;
    }
  }

  .dash-tiles-grid {
    grid-template-columns: 1fr;
  }
}
</style>
