<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Promotion,
  ShoppingBag,
  CopyDocument,
  Document,
  Bell,
  Connection,
  Calendar,
  CircleCheckFilled,
  Wallet,
  Cellphone,
  WarningFilled,
  InfoFilled,
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { buildSubscribeUrl } from '@/config/site'
import { errMsg } from '@/api/http'
import { formatBytes, formatDate } from '@/utils/format'
import { renderMarkdown } from '@/utils/markdown'
import { getMyServers, getUserNotices, type MyServerItem } from '@/api/user'
import type { NoticeItem } from '@/api/types'

const auth = useAuthStore()
const site = useSiteStore()
const loading = ref(false)

const balanceYuan = computed(() => {
  const cents = auth.user?.balance_cents ?? 0
  return (cents / 100).toFixed(2)
})

const deviceLimitText = computed(() => {
  const lim = auth.user?.device_limit || auth.user?.effective_device_limit || 0
  return lim > 0 ? `${lim} 台设备` : '不限设备'
})

const planLabel = computed(() => (auth.user?.plan_id ? `已购套餐 #${auth.user.plan_id}` : '暂无生效套餐'))
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
  if (!t) return auth.user?.plan_id ? '永久有效' : '未开通套餐'
  return String(t).replace('T', ' ').slice(0, 16)
})

// U23：到期/未开通判定（用于横幅 CTA，不再让无套餐用户看到误导性的「永久有效」）
const isExpired = computed(() => {
  const t = auth.user?.expire_at
  return !!t && new Date(t).getTime() < Date.now()
})

const daysLeft = computed(() => {
  if (!auth.user?.expire_at) return null
  const exp = new Date(auth.user.expire_at).getTime()
  const now = Date.now()
  const diff = Math.ceil((exp - now) / (1000 * 60 * 60 * 24))
  return diff > 0 ? diff : 0
})

const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token, site.subscribeUrl, site.subscribePath) : ''
})

onMounted(async () => {
  loadServers()
  // ISSUE-16：登录响应可能只是浅层摘要，首页挂载始终拉取 /user/me，保证首屏数据与刷新后一致。
  loading.value = true
  try {
    await auth.fetchMe()
  } catch (e) {
    ElMessage.error(errMsg(e, '加载用户信息失败'))
  } finally {
    loading.value = false
  }
})

function copySub() {
  if (!subscribeUrl.value) {
    ElMessage.warning('暂无可用订阅链接')
    return
  }
  navigator.clipboard?.writeText(subscribeUrl.value).then(
    () => ElMessage.success('Clash 订阅链接已复制'),
    () => ElMessage.warning('复制失败，请前往订阅中心手动复制'),
  )
}

function importClash() {
  if (!subscribeUrl.value) {
    ElMessage.warning('暂无可用订阅链接')
    return
  }
  const url = `clash://install-config?url=${encodeURIComponent(subscribeUrl.value)}&name=XrayPanel`
  window.location.href = url
  ElMessage.info('正在唤醒 Clash 客户端…')
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

const statusText: Record<string, { dot: string; text: string }> = {
  online: { dot: 'online', text: '在线' },
  connecting: { dot: 'connecting', text: '维护中' },
  offline: { dot: 'offline', text: '离线' },
}

// 公告系统（真实接口 + 弹窗与强提醒）
const notices = ref<NoticeItem[]>([])
const noticeLoading = ref(false)
const selectedNotice = ref<NoticeItem | null>(null)
const noticeModalOpen = ref(false)
const popupNotice = ref<NoticeItem | null>(null)
const popupOpen = ref(false)

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

onMounted(() => {
  loadServers()
  loadNotices()
})
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <div class="x-dash-grid">
      <!-- 左侧主栏：用量卡片 + 快捷操作 -->
      <div>
        <!-- U23：到期/未开通 CTA 横幅（替代误导性的「永久有效」展示） -->
        <div v-if="isExpired" class="dash-alert danger">
          <el-icon><WarningFilled /></el-icon>&nbsp;
          <span>套餐已到期，节点将无法使用，请及时续费</span>
          <router-link to="/shop">
            <el-button size="small" type="primary" round>立即续费</el-button>
          </router-link>
        </div>
        <div v-else-if="!auth.user?.plan_id" class="dash-alert">
          <el-icon><InfoFilled /></el-icon>&nbsp;
          <span>尚未开通套餐，开通后即可获取节点订阅</span>
          <router-link to="/shop">
            <el-button size="small" type="primary" round>去开通</el-button>
          </router-link>
        </div>

        <!-- 现代化科技感用量 Hero 卡片 -->
        <div class="dash-hero">
          <div class="hero-top">
            <div class="hero-plan-badge">
              <el-icon><CircleCheckFilled /></el-icon>&nbsp;{{ planLabel }}
            </div>
            <div class="hero-top-right">
              <div class="hero-wallet-tag">
                <el-icon><Wallet /></el-icon>&nbsp;余额 ¥ {{ balanceYuan }}
              </div>
              <div class="hero-wallet-tag" style="background: rgba(56, 189, 248, 0.12); color: #38bdf8; border-color: rgba(56, 189, 248, 0.25)">
                <el-icon><Cellphone /></el-icon>&nbsp;{{ deviceLimitText }}
              </div>
              <div v-if="daysLeft !== null" class="hero-days-tag">
                <el-icon><Calendar /></el-icon>&nbsp;剩余 {{ daysLeft }} 天到期
              </div>
            </div>
          </div>

          <div class="hero-main-stats">
            <div class="stat-col">
              <span class="stat-lbl">已用流量</span>
              <span class="stat-val used">{{ usedText }}</span>
            </div>
            <div class="stat-divider" />
            <div class="stat-col">
              <span class="stat-lbl">剩余可用</span>
              <span class="stat-val remain">{{ remainText }}</span>
            </div>
            <div class="stat-divider" />
            <div class="stat-col">
              <span class="stat-lbl">套餐总量</span>
              <span class="stat-val total">{{ totalText }}</span>
            </div>
          </div>

          <!-- 进度条 -->
          <div class="hero-progress-wrap">
            <div class="progress-bar-bg">
              <div class="progress-bar-fill" :style="{ width: usagePercent + '%' }" />
            </div>
            <div class="progress-meta">
              <span>使用占比: {{ usagePercent }}%</span>
              <span>到期时间: {{ expireText }}</span>
            </div>
          </div>

          <!-- 卡片内快捷操作栏 -->
          <div class="hero-actions">
            <el-button type="primary" class="hero-btn-primary" @click="importClash">
              <el-icon><Promotion /></el-icon>&nbsp;一键导入 Clash
            </el-button>
            <el-button class="hero-btn-glass" @click="copySub">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制订阅
            </el-button>
            <router-link to="/shop">
              <el-button class="hero-btn-glass">
                <el-icon><ShoppingBag /></el-icon>&nbsp;购买/续费
              </el-button>
            </router-link>
          </div>
        </div>

        <!-- 快捷入口磁贴 -->
        <div class="dash-tiles">
          <router-link to="/subscribe" class="dash-tile">
            <div class="tile-icon purple"><el-icon><Promotion /></el-icon></div>
            <div class="tile-info">
              <div class="tile-title">订阅中心</div>
              <div class="tile-desc">Clash / Mihomo 导入与各平台客户端</div>
            </div>
          </router-link>
          <router-link to="/shop" class="dash-tile">
            <div class="tile-icon green"><el-icon><ShoppingBag /></el-icon></div>
            <div class="tile-info">
              <div class="tile-title">套餐商店</div>
              <div class="tile-desc">高速节点与无限流量包选购</div>
            </div>
          </router-link>
        </div>
      </div>

      <!-- 右侧侧边栏：节点状态 + 公告 + 教程 -->
      <div>
        <!-- 节点状态 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Connection /></el-icon>&nbsp;节点可用性</span>
            <span class="muted" style="font-size: 12px">心跳 90s 内为在线</span>
          </div>
          <div style="padding: 8px 16px">
            <div v-for="s in servers" :key="s.id" class="x-row-line">
              <span class="k">
                <span class="x-status-dot" :class="s.online ? 'online' : 'offline'" />
                {{ s.location || '未知地区' }} · {{ s.name }}
              </span>
              <span class="v muted" style="font-size: 12px">{{ s.online ? '在线' : '离线' }}</span>
            </div>
            <p v-if="!servers.length" class="muted" style="font-size: 12px; padding: 4px 0">
              暂无可用节点（需购买套餐后自动授权）
            </p>
          </div>
        </div>

        <!-- 公告栏 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Bell /></el-icon>&nbsp;最新公告</span>
          </div>
          <div v-if="noticeLoading" style="padding: 16px; text-align: center" class="muted">
            加载公告中...
          </div>
          <div v-else-if="!notices.length" style="padding: 16px; text-align: center" class="muted">
            暂无系统公告
          </div>
          <div
            v-for="n in notices"
            :key="n.id"
            class="x-notice-item cursor-pointer"
            @click="viewNotice(n)"
          >
            <div class="x-notice-title">
              <el-tag v-if="n.is_pinned" size="small" type="danger" effect="dark" class="mr-1">置顶</el-tag>
              {{ n.title }}
            </div>
            <div class="x-notice-date">{{ formatDate(n.created_at || n.updated_at) }}</div>
          </div>
        </div>

        <!-- 帮助文档 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Document /></el-icon>&nbsp;使用帮助</span>
          </div>
          <div style="display: grid; gap: 10px; padding: 14px 16px">
            <router-link to="/subscribe" class="muted help-link">
              Clash Verge Rev 快速导入教程
            </router-link>
            <router-link to="/subscribe" class="muted help-link">
              苹果 iOS Stash 配置指南
            </router-link>
          </div>
        </div>
      </div>
    </div>

    <!-- 公告详情弹窗 -->
    <el-dialog
      v-model="noticeModalOpen"
      :title="selectedNotice?.title || '公告详情'"
      width="580px"
      destroy-on-close
    >
      <div v-if="selectedNotice" class="notice-detail-dialog">
        <div class="notice-meta">
          <el-tag v-if="selectedNotice.is_pinned" size="small" type="danger" effect="dark" class="mr-1">置顶</el-tag>
          <span class="notice-time muted">发布于 {{ formatDate(selectedNotice.created_at) }}</span>
        </div>
        <div class="notice-body markdown-content" v-html="renderMarkdown(selectedNotice.content)" />
      </div>
      <template #footer>
        <el-button type="primary" @click="noticeModalOpen = false">我知道了</el-button>
      </template>
    </el-dialog>

    <!-- 首页重要公告强提醒弹窗 -->
    <el-dialog
      v-model="popupOpen"
      :title="popupNotice?.title || '重要通知'"
      width="540px"
      :before-close="handleClosePopup"
      destroy-on-close
    >
      <div v-if="popupNotice" class="notice-detail-dialog">
        <div class="notice-meta">
          <el-tag size="small" type="warning" effect="dark" class="mr-1">重要通知</el-tag>
          <span class="notice-time muted">{{ formatDate(popupNotice.updated_at) }}</span>
        </div>
        <div class="notice-body popup-emphasis markdown-content" v-html="renderMarkdown(popupNotice.content)" />
      </div>
      <template #footer>
        <el-button type="primary" @click="handleClosePopup">已了解并关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.dash-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-radius: var(--x-radius);
  margin-bottom: 14px;
  font-size: 13.5px;
  background: rgba(56, 189, 248, 0.1);
  border: 1px solid rgba(56, 189, 248, 0.3);
  color: var(--x-text);
  > span { flex: 1; }
  &.danger {
    background: rgba(244, 63, 94, 0.1);
    border-color: rgba(244, 63, 94, 0.35);
    color: #fb7185;
  }
}
.dash-hero {
  background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%);
  border-radius: var(--x-radius);
  padding: 24px;
  color: #fff;
  box-shadow: 0 10px 30px rgba(79, 70, 229, 0.25);
  margin-bottom: 16px;
  position: relative;
  overflow: hidden;

  &::after {
    content: '';
    position: absolute;
    top: -50px;
    right: -50px;
    width: 200px;
    height: 200px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(255, 255, 255, 0.15) 0%, transparent 70%);
    pointer-events: none;
  }
}

.hero-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;

  .hero-plan-badge {
    display: inline-flex;
    align-items: center;
    background: rgba(255, 255, 255, 0.18);
    backdrop-filter: blur(8px);
    padding: 4px 12px;
    border-radius: 20px;
    font-size: 13px;
    font-weight: 600;
  }

  .hero-top-right {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .hero-wallet-tag {
    display: inline-flex;
    align-items: center;
    background: rgba(254, 240, 138, 0.25);
    border: 1px solid rgba(254, 240, 138, 0.4);
    padding: 3px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 700;
    color: #fef08a;
    font-family: var(--x-font-mono);
  }

  .hero-days-tag {
    display: inline-flex;
    align-items: center;
    background: rgba(16, 185, 129, 0.25);
    border: 1px solid rgba(16, 185, 129, 0.4);
    padding: 3px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 600;
    color: #a7f3d0;
  }
}

.hero-main-stats {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 18px;

  .stat-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .stat-divider {
    width: 1px;
    height: 36px;
    background: rgba(255, 255, 255, 0.15);
    margin: 0 12px;
  }

  .stat-lbl {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.75);
  }

  .stat-val {
    font-size: 20px;
    font-weight: 800;
    font-family: var(--x-font-mono);
    &.used {
      color: #fde047;
    }
    &.remain {
      color: #ffffff;
    }
    &.total {
      color: rgba(255, 255, 255, 0.9);
    }
  }
}

.hero-progress-wrap {
  margin-bottom: 22px;

  .progress-bar-bg {
    height: 8px;
    background: rgba(255, 255, 255, 0.2);
    border-radius: 4px;
    overflow: hidden;
  }

  .progress-bar-fill {
    height: 100%;
    background: linear-gradient(90deg, #38bdf8 0%, #fde047 100%);
    border-radius: 4px;
    transition: width 0.4s ease;
  }

  .progress-meta {
    display: flex;
    justify-content: space-between;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.75);
    margin-top: 6px;
    font-family: var(--x-font-mono);
  }
}

.hero-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;

  .hero-btn-primary {
    background: #ffffff;
    color: #4f46e5;
    border: none;
    font-weight: 700;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.15);
    &:hover {
      background: #f1f5f9;
      color: #4338ca;
    }
  }

  .hero-btn-glass {
    background: rgba(255, 255, 255, 0.15);
    border: 1px solid rgba(255, 255, 255, 0.3);
    color: #fff;
    font-weight: 600;
    &:hover {
      background: rgba(255, 255, 255, 0.25);
    }
  }
}

.dash-tiles {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-bottom: 16px;

  .dash-tile {
    background: var(--x-card);
    border: 1px solid var(--x-border);
    border-radius: var(--x-radius);
    padding: 16px;
    box-shadow: var(--x-shadow);
    display: flex;
    gap: 14px;
    align-items: center;
    transition: all 0.2s ease;

    &:hover {
      transform: translateY(-2px);
      box-shadow: var(--x-shadow-md);
      border-color: rgba(99, 102, 241, 0.3);
    }

    .tile-icon {
      width: 44px;
      height: 44px;
      border-radius: 12px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 22px;
      flex: none;

      &.purple {
        background: var(--x-primary-soft);
        color: var(--x-primary);
      }
      &.green {
        background: var(--x-success-soft);
        color: var(--x-success);
      }
    }

    .tile-title {
      font-size: 14px;
      font-weight: 700;
      color: var(--x-text);
    }
    .tile-desc {
      font-size: 12px;
      color: var(--x-text-3);
      margin-top: 2px;
    }
  }
}

.help-link {
  font-size: 13px;
  display: flex;
  align-items: center;
  transition: color 0.15s;
  &:hover {
    color: var(--x-primary);
  }
}

.demo-tag {
  font-size: 11px;
  color: var(--x-success);
  background: var(--x-success-soft);
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 6px;
  padding: 1px 7px;
  font-weight: 500;
}

.cursor-pointer {
  cursor: pointer;
  transition: background-color 0.15s ease;
  &:hover {
    background-color: var(--x-fill-2, rgba(0, 0, 0, 0.03));
  }
}

.mr-1 {
  margin-right: 4px;
}

.notice-detail-dialog {
  display: flex;
  flex-direction: column;
  gap: 12px;

  .notice-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  .notice-body {
    font-size: 14px;
    line-height: 1.65;
    color: var(--x-text);
    word-break: break-word;
    background: var(--x-fill-1, #f8fafc);
    border: 1px solid var(--x-border-light, #e2e8f0);
    border-radius: 8px;
    padding: 12px 16px;
    max-height: 480px;
    overflow-y: auto;

    &.popup-emphasis {
      border-left: 4px solid var(--x-warning, #f59e0b);
      background: var(--x-fill-1, #fffbeb);
    }
  }
}
</style>