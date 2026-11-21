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
  CsModal,
  CsSpaceBetween,
  CsTabs,
  toast,
  messageBox,
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
const activeGuideId = ref('')

const guideTabs = computed(() =>
  guides.value.map((g) => ({
    id: String(g.id),
    label: g.title,
  })),
)

const currentGuide = computed(() => {
  if (!guides.value.length) return null
  return guides.value.find((g) => String(g.id) === activeGuideId.value) ?? guides.value[0]
})

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
    toast.error(errMsg(e, '加载订阅信息失败'))
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
      activeGuideId.value = guides.value[0] ? String(guides.value[0].id) : ''
    }
  } catch {
    guides.value = []
    activeGuideId.value = ''
  } finally {
    guideLoading.value = false
  }
}

function copyText(text: string, label: string) {
  if (!text) {
    toast.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  navigator.clipboard?.writeText(text).then(
    () => toast.success(`${label}已复制到剪贴板`),
    () => toast.warning('复制失败，请手动复制'),
  )
}

function importToMihomo() {
  if (!mihomoSchemeUrl.value) {
    toast.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  window.location.href = mihomoSchemeUrl.value
  toast.info('正在唤醒 Mihomo / Clash 客户端，若未响应请手动复制订阅地址…')
}

function openQrModal() {
  if (!subscribeUrl.value) {
    toast.warning('暂无可用订阅地址，请先订购有效套餐')
    return
  }
  qrModalOpen.value = true
}

async function onResetSubscribe() {
  try {
    await messageBox.confirm(
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
      toast.success('订阅地址与连接凭据已重置，请在客户端中更新订阅')
      await auth.fetchMe()
      await refreshQr()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '重置订阅地址失败'))
  } finally {
    resettingSub.value = false
  }
}
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

          <div class="sub-actions">
            <CsButton variant="primary" icon-name="external" :wrap-text="false" @click="importToMihomo">
              一键导入 Clash 客户端
            </CsButton>
            <CsButton icon-name="copy" :wrap-text="false" @click="copyText(subscribeUrl, 'Mihomo 订阅地址')">
              复制订阅地址
            </CsButton>
            <CsButton :loading="resettingSub" :wrap-text="false" @click="onResetSubscribe">
              重置订阅地址
            </CsButton>
          </div>
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

    <!-- 使用指南（由管理端客制化，直接摊开展示） -->
    <CsContainer>
      <template #header>
        <CsHeader variant="h2">使用指南</CsHeader>
      </template>
      <CsBox v-if="guideLoading" color="text-body-secondary" font-size="body-s">
        加载指南中…
      </CsBox>
      <CsBox v-else-if="!currentGuide" color="text-body-secondary" font-size="body-s">
        暂无使用指南
      </CsBox>
      <CsSpaceBetween v-else direction="vertical" size="m">
        <CsTabs
          v-if="guides.length > 1"
          v-model="activeGuideId"
          :tabs="guideTabs"
          disable-content-paddings
        />
        <div v-else class="guide-single-head">
          <CsBadge v-if="currentGuide.is_pinned" color="red">置顶</CsBadge>
          <span class="guide-single-title">{{ currentGuide.title }}</span>
        </div>
        <div
          class="guide-body markdown-content"
          v-html="renderMarkdown(currentGuide.content)"
        />
      </CsSpaceBetween>
    </CsContainer>

    <!-- 二维码放大弹窗 -->
    <CsModal
      v-model:visible="qrModalOpen"
      header="手机扫码导入订阅"
      size="small"
    >
      <div class="qr-modal-body">
        <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" class="qr-modal-img" />
        <p class="qr-modal-tip">使用支持 Clash / Mihomo 协议的客户端扫描上方二维码即可导入订阅。</p>
      </div>
      <template #footer>
        <CsButton variant="primary" @click="qrModalOpen = false">关闭</CsButton>
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

.sub-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
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
  inline-size: 132px;
  block-size: 132px;
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

.guide-single-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.guide-single-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--x-text);
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
  inline-size: 242px;
  block-size: 242px;
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
  .sub-main-grid {
    grid-template-columns: 1fr;
  }

  .sub-actions {
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
}
</style>
