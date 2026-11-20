<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsContainer,
  CsHeader,
  CsLink,
  CsModal,
  CsSpaceBetween,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { getUserNotices, resetSubscribeToken } from '@/api/user'
import type { NoticeItem } from '@/api/types'
import { renderMarkdown } from '@/utils/markdown'
import { buildSubscribeUrl } from '@/config/site'
import { errMsg } from '@/api/http'

const router = useRouter()
const auth = useAuthStore()
const site = useSiteStore()
const qrDataUrl = ref('')
const loading = ref(false)
const qrModalOpen = ref(false)
const resettingSub = ref(false)
const guides = ref<NoticeItem[]>([])
const guideLoading = ref(false)
const selectedGuide = ref<NoticeItem | null>(null)
const guideModalOpen = ref(false)

const hasPlan = computed(() => {
  if (!auth.user?.plan_id) return false
  const exp = auth.user?.expire_at
  return !(exp && new Date(exp).getTime() < Date.now())
})

const usedBytes = computed(() => auth.user?.used_bytes ?? 0)
const totalBytes = computed(() => auth.user?.total_bytes ?? 0)
const isExhausted = computed(
  () => hasPlan.value && totalBytes.value > 0 && usedBytes.value >= totalBytes.value,
)

const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token, site.subscribeUrl, site.subscribePath) : ''
})

const mihomoSchemeUrl = computed(() => {
  if (!subscribeUrl.value) return ''
  return `clash://install-config?url=${encodeURIComponent(subscribeUrl.value)}&name=${encodeURIComponent('XrayPanel')}`
})

async function refreshQr() {
  if (subscribeUrl.value) {
    try {
      qrDataUrl.value = await QRCode.toDataURL(subscribeUrl.value, {
        width: 260,
        margin: 2,
        color: { dark: '#171b2e', light: '#ffffff' },
      })
    } catch {
      qrDataUrl.value = ''
    }
  } else {
    qrDataUrl.value = ''
  }
}

onMounted(async () => {
  loading.value = true
  loadGuides()
  try {
    if (!auth.user) await auth.fetchMe()
    await refreshQr()
  } catch (e) {
    ElMessage.error(errMsg(e, '加载订阅信息失败'))
  } finally {
    loading.value = false
  }
})

async function loadGuides() {
  guideLoading.value = true
  try {
    const { data } = await getUserNotices()
    if (data.code === 0) {
      guides.value = data.data.filter((n) => n.category === 'guide')
    }
  } catch {
    guides.value = []
  } finally {
    guideLoading.value = false
  }
}

function viewGuide(g: NoticeItem) {
  selectedGuide.value = g
  guideModalOpen.value = true
}

function copyText(text: string, label: string) {
  if (!text) {
    ElMessage.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  navigator.clipboard?.writeText(text).then(
    () => ElMessage.success(`${label}已复制到剪贴板`),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function importToMihomo() {
  if (!mihomoSchemeUrl.value) {
    ElMessage.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  window.location.href = mihomoSchemeUrl.value
  ElMessage.info('正在唤醒 Mihomo / Clash 客户端，若未响应请手动复制订阅地址…')
}

function openQrModal() {
  if (!subscribeUrl.value) {
    ElMessage.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  qrModalOpen.value = true
}

async function onResetSubscribe() {
  try {
    await ElMessageBox.confirm(
      '重置后，原订阅地址与已连接的节点凭据将立即失效，所有客户端需重新同步或更新订阅后方可继续使用。确定重置订阅地址？',
      '重置订阅地址',
      { type: 'warning', confirmButtonText: '确定重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  resettingSub.value = true
  try {
    const { data } = await resetSubscribeToken()
    if (data.code === 0) {
      ElMessage.success('订阅地址与连接凭据已重置，请在客户端中更新订阅')
      await auth.fetchMe()
      await refreshQr()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置订阅地址失败'))
  } finally {
    resettingSub.value = false
  }
}

interface ClientApp {
  name: string
  tag: string
  platforms: string[]
  desc: string
  url?: string
}

const clashApps: ClientApp[] = [
  {
    name: 'Clash Verge Rev',
    tag: '首选推荐',
    platforms: ['Windows', 'macOS', 'Linux'],
    desc: '基于 Mihomo 核心，现代极简 UI，全协议兼容，支持流媒体策略组分流。',
    url: 'https://github.com/clash-verge-rev/clash-verge-rev',
  },
  {
    name: 'Mihomo Party',
    tag: '优雅开源',
    platforms: ['Windows', 'macOS', 'Linux'],
    desc: '专为 Mihomo 定制的优雅桌面客户端，内置丰富分流规则、节点延迟测速与拓扑视图。',
    url: 'https://github.com/mihomo-party-org/mihomo-party',
  },
  {
    name: 'FlClash',
    tag: '全平台',
    platforms: ['Android', 'iOS', 'Windows', 'macOS'],
    desc: '基于 Flutter 的跨平台客户端，轻量美观，内存占用低，适配移动端。',
    url: 'https://github.com/chen08209/FlClash',
  },
  {
    name: 'Stash',
    tag: 'iOS 推荐',
    platforms: ['iOS', 'iPadOS', 'macOS'],
    desc: '适配苹果生态的规则分流代理客户端，全面支持按需连接与自动化策略。',
    url: 'https://stash.ws/',
  },
]
</script>

<template>
  <CsSpaceBetween direction="vertical" size="l">
    <CsHeader variant="h1">
      订阅中心
    </CsHeader>

    <CsAlert v-if="isExhausted" type="error" header="流量已用尽">
      当前套餐流量已用尽，节点转发已暂停。续费或订购其他套餐后将重置流量并恢复使用。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">前往续费/订购</CsButton>
      </template>
    </CsAlert>

    <CsAlert v-else-if="!hasPlan" type="info" header="当前无有效套餐">
      您当前未订购套餐或套餐已到期，重新订购后系统将自动为您下发订阅地址并分配节点线路。
      <template #action>
        <CsButton variant="primary" @click="router.push('/shop')">前往选购套餐</CsButton>
      </template>
    </CsAlert>

    <!-- 核心订阅卡片 -->
    <CsContainer>
      <template #header>
        <CsHeader variant="h2">订阅地址</CsHeader>
      </template>

      <div class="sub-main-grid">
        <CsSpaceBetween direction="vertical" size="m">
          <div class="sub-url-box">
            <div class="sub-url-label">Mihomo 订阅地址</div>
            <code class="sub-url-code cell-mono">
              {{
                loading
                  ? '正在加载订阅地址…'
                  : subscribeUrl || (hasPlan ? '正在生成订阅地址…' : '暂无有效订阅（请先订购套餐）')
              }}
            </code>
          </div>

          <CsSpaceBetween direction="horizontal" size="xs">
            <CsButton variant="primary" icon-name="external" @click="importToMihomo">
              一键导入 Mihomo 客户端
            </CsButton>
            <CsButton icon-name="copy" @click="copyText(subscribeUrl, 'Mihomo 订阅地址')">
              复制订阅地址
            </CsButton>
            <CsButton @click="openQrModal">手机扫码导入</CsButton>
            <CsButton :loading="resettingSub" @click="onResetSubscribe">重置订阅地址</CsButton>
          </CsSpaceBetween>
        </CsSpaceBetween>

        <div class="sub-qr-panel" @click="openQrModal">
          <div class="qr-wrap" title="点击放大查看">
            <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" class="sub-qr" />
            <div v-else class="sub-qr-empty">—</div>
          </div>
          <span class="qr-tip">点击放大扫码</span>
        </div>
      </div>
    </CsContainer>

    <!-- 推荐客户端 -->
    <CsSpaceBetween direction="vertical" size="m">
      <CsHeader variant="h2">推荐客户端（Mihomo 生态）</CsHeader>
      <div class="client-grid">
        <CsContainer v-for="app in clashApps" :key="app.name">
          <template #header>
            <CsHeader variant="h3">
              {{ app.name }}
              <template #actions>
                <CsBadge color="blue">{{ app.tag }}</CsBadge>
              </template>
            </CsHeader>
          </template>
          <CsSpaceBetween direction="vertical" size="s">
            <div class="platform-list">
              <CsBadge v-for="plat in app.platforms" :key="plat" color="grey">{{ plat }}</CsBadge>
            </div>
            <div class="client-desc">{{ app.desc }}</div>
          </CsSpaceBetween>
          <template #footer>
            <CsLink v-if="app.url" :href="app.url" external>获取客户端</CsLink>
          </template>
        </CsContainer>
      </div>
    </CsSpaceBetween>

    <!-- 使用指南（由管理端客制化） -->
    <CsContainer>
      <template #header>
        <CsHeader variant="h2">使用指南</CsHeader>
      </template>
      <CsBox v-if="guideLoading" color="text-body-secondary" font-size="body-s">
        加载指南中…
      </CsBox>
      <CsBox v-else-if="!guides.length" color="text-body-secondary" font-size="body-s">
        暂无使用指南
      </CsBox>
      <div v-else class="guide-list">
        <button
          v-for="g in guides"
          :key="g.id"
          type="button"
          class="guide-row"
          @click="viewGuide(g)"
        >
          <div class="guide-title">
            <CsBadge v-if="g.is_pinned" color="red">置顶</CsBadge>
            <span>{{ g.title }}</span>
          </div>
          <span class="guide-action">查看详情</span>
        </button>
      </div>
    </CsContainer>

    <!-- 指南详情弹窗 -->
    <CsModal
      v-model:visible="guideModalOpen"
      :header="selectedGuide?.title || '使用指南'"
      size="medium"
      :close-on-click-overlay="false"
    >
      <div
        v-if="selectedGuide"
        class="guide-body markdown-content"
        v-html="renderMarkdown(selectedGuide.content)"
      />
      <template #footer>
        <CsButton variant="primary" @click="guideModalOpen = false">我知道了</CsButton>
      </template>
    </CsModal>

    <!-- 二维码放大弹窗 -->
    <CsModal
      v-model:visible="qrModalOpen"
      header="手机扫码导入订阅"
      size="small"
      :close-on-click-overlay="false"
    >
      <div class="qr-modal-body">
        <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" class="qr-modal-img" />
        <p class="qr-modal-tip">使用支持 Clash / Mihomo 协议的客户端扫描上方二维码即可导入订阅。</p>
      </div>
      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="copyText(subscribeUrl, 'Mihomo 订阅地址')">复制订阅地址</CsButton>
          <CsButton variant="primary" @click="qrModalOpen = false">关闭</CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.sub-main-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 20px;
  align-items: center;
}

.sub-url-box {
  padding: 12px 14px;
  border: 1px solid var(--x-border);
  border-radius: 8px;
}

.sub-url-label {
  font-size: 12px;
  color: var(--x-text-secondary);
  margin-bottom: 6px;
}

.sub-url-code {
  display: block;
  font-size: 13px;
  color: var(--x-text);
  word-break: break-all;
}

.sub-qr-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}

.qr-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  inline-size: 120px;
  block-size: 120px;
  padding: 6px;
  border: 1px solid var(--x-border);
  border-radius: 8px;
  background-color: #ffffff;
}

.sub-qr {
  inline-size: 100%;
  block-size: 100%;
  object-fit: contain;
}

.sub-qr-empty {
  font-size: 14px;
  color: var(--x-text-secondary);
}

.qr-tip {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.client-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.platform-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.client-desc {
  font-size: 13px;
  line-height: 1.55;
  color: var(--x-text-secondary);
}

.guide-list {
  display: flex;
  flex-direction: column;
}

.guide-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
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

  &:hover .guide-title {
    color: var(--x-primary);
  }
}

.guide-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text);
  transition: color 0.15s ease;
}

.guide-action {
  font-size: 12px;
  color: var(--x-primary);
  flex-shrink: 0;
}

.guide-body {
  font-size: 14px;
  line-height: 1.65;
  color: var(--x-text);
}

.qr-modal-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
}

.qr-modal-img {
  inline-size: 220px;
  block-size: 220px;
  border-radius: 8px;
  background-color: #ffffff;
  padding: 8px;
  border: 1px solid var(--x-border);
}

.qr-modal-tip {
  margin: 0;
  font-size: 13px;
  color: var(--x-text-secondary);
  text-align: center;
}

@media (max-width: $bp-tablet-max) {
  .sub-main-grid,
  .client-grid {
    grid-template-columns: 1fr;
  }
}
</style>
