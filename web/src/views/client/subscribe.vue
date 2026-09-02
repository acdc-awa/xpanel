<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CopyDocument,
  Download,
  Cellphone,
  Promotion,
  View,
} from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { buildSubscribeUrl } from '@/config/site'
import { errMsg } from '@/api/http'

const auth = useAuthStore()
const site = useSiteStore()
const qrDataUrl = ref('')
const loading = ref(false)
const qrModalOpen = ref(false)

const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token, site.subscribeUrl, site.subscribePath) : ''
})

// 一键唤醒 Mihomo Scheme
const mihomoSchemeUrl = computed(() => {
  if (!subscribeUrl.value) return ''
  return `clash://install-config?url=${encodeURIComponent(subscribeUrl.value)}&name=${encodeURIComponent('XrayPanel')}`
})

onMounted(async () => {
  loading.value = true
  try {
    if (!auth.user) await auth.fetchMe()
    if (subscribeUrl.value) {
      qrDataUrl.value = await QRCode.toDataURL(subscribeUrl.value, {
        width: 260,
        margin: 2,
        color: { dark: '#171b2e', light: '#ffffff' },
      })
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成二维码失败'))
  } finally {
    loading.value = false
  }
})

function copyText(text: string, label: string) {
  if (!text) return
  navigator.clipboard?.writeText(text).then(
    () => ElMessage.success(`${label}已复制到剪贴板`),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function importToMihomo() {
  if (!mihomoSchemeUrl.value) return
  window.location.href = mihomoSchemeUrl.value
  ElMessage.info('正在尝试唤醒 Mihomo / Clash 客户端，若未响应请手动复制订阅地址')
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
    desc: '基于 Mihomo 核心，现代极简 UI，性能卓越，全协议完美兼容与流媒体策略组分流。',
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
    name: 'Flclash',
    tag: '全平台',
    platforms: ['Android', 'iOS', 'Windows', 'macOS'],
    desc: '基于 Flutter 的跨平台客户端，轻量美观，内存占用低，移动端体验极佳。',
    url: 'https://github.com/chen08209/FlClash',
  },
  {
    name: 'Stash',
    tag: 'iOS 推荐',
    platforms: ['iOS', 'iPadOS', 'macOS'],
    desc: '苹果生态顶级的规则分流代理客户端，全面支持按需连接与自动化策略。',
    url: 'https://stash.ws/',
  },
]
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <!-- 头部横幅 -->
    <div class="sub-hero">
      <div class="sub-badge"><el-icon><Promotion /></el-icon>&nbsp;Mihomo 核心托管</div>
      <h1 class="sub-title">Mihomo 订阅中心</h1>
      <p class="sub-desc">专为 Mihomo / Clash 核心深度优化，包含策略分流、自动故障转移与 VLESS REALITY 落地链路。</p>
    </div>

    <!-- 核心一键导入卡片 -->
    <div class="x-card primary-card">
      <div class="x-card-body">
        <div class="sub-main-grid">
          <!-- 左侧：一键唤醒与快捷复制 -->
          <div class="sub-left">
            <div class="sub-url-box">
              <span class="sub-url-label">Mihomo 订阅端点地址</span>
              <code class="sub-url-code cell-mono">{{ subscribeUrl || '正在生成订阅凭据…' }}</code>
            </div>

            <div class="sub-action-buttons">
              <el-button type="primary" size="large" class="glow-btn" @click="importToMihomo">
                <el-icon><Promotion /></el-icon>&nbsp;一键导入 Mihomo 客户端
              </el-button>
              <el-button size="large" @click="copyText(subscribeUrl, 'Mihomo 订阅地址')">
                <el-icon><CopyDocument /></el-icon>&nbsp;复制订阅地址
              </el-button>
            </div>

            <div class="sub-minor-actions">
              <el-button text size="small" @click="qrModalOpen = true">
                <el-icon><Cellphone /></el-icon>&nbsp;手机扫码导入
              </el-button>
            </div>
          </div>

          <!-- 右侧：二维码微缩卡片 -->
          <div class="sub-right" @click="qrModalOpen = true">
            <div class="qr-wrap" title="点击放大查看">
              <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" class="sub-qr" />
              <div v-else class="sub-qr-empty">—</div>
              <span class="qr-tip"><el-icon><View /></el-icon>&nbsp;扫码导入</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 推荐客户端生态 -->
    <div class="section-title">
      <span>推荐客户端（Mihomo 核心生态）</span>
    </div>

    <div class="client-grid">
      <div v-for="app in clashApps" :key="app.name" class="client-card">
        <div>
          <div class="client-head">
            <div class="client-meta">
              <div class="client-name">
                {{ app.name }}
                <span class="client-tag">{{ app.tag }}</span>
              </div>
              <div class="client-platform-tags">
                <span v-for="plat in app.platforms" :key="plat" class="platform-chip">{{ plat }}</span>
              </div>
            </div>
          </div>
          <p class="client-desc">{{ app.desc }}</p>
        </div>
        <div class="client-footer">
          <a v-if="app.url" :href="app.url" target="_blank" rel="noreferrer" class="client-link">
            <el-icon><Download /></el-icon>&nbsp;获取客户端
          </a>
        </div>
      </div>
    </div>

    <!-- 快速接入说明 -->
    <div class="x-card" style="margin-top: 20px">
      <div class="x-card-head"><span>快速接入指引</span></div>
      <div class="sub-steps">
        <div class="sub-step">
          <span class="step-num">1</span>
          <div class="step-info">
            <div class="step-title">获取客户端</div>
            <div class="step-desc">Windows / macOS 推荐使用 <b>Clash Verge Rev</b> 或 <b>Mihomo Party</b>；Android 推荐 <b>Flclash</b>；iOS 推荐 <b>Stash</b>。</div>
          </div>
        </div>
        <div class="sub-step">
          <span class="step-num">2</span>
          <div class="step-info">
            <div class="step-title">同步订阅配置</div>
            <div class="step-desc">点击上方「一键导入 Mihomo 客户端」，或复制订阅端点地址至客户端配置管理中粘贴保存并更新。</div>
          </div>
        </div>
        <div class="sub-step">
          <span class="step-num">3</span>
          <div class="step-info">
            <div class="step-title">启用系统代理</div>
            <div class="step-desc">在客户端代理分组中选择节点或开启「自动故障转移 (Fallback)」，打开「系统代理 (System Proxy)」开关即可顺畅连接。</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 二维码放大弹窗 -->
    <el-dialog v-model="qrModalOpen" title="手机扫码导入订阅" width="340px" append-to-body center>
      <div style="display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 10px 0">
        <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" style="width: 220px; height: 220px; border-radius: 8px; border: 1px solid var(--x-border)" />
        <p class="muted" style="font-size: 12px; text-align: center">
          使用手机端 Mihomo 客户端（如 Stash、Flclash）扫码即可自动添加配置。
        </p>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.sub-hero {
  margin-bottom: 18px;
  .sub-badge {
    display: inline-flex;
    align-items: center;
    padding: 3px 10px;
    background: var(--x-primary-soft);
    color: var(--x-primary);
    border-radius: 20px;
    font-size: 12px;
    font-weight: 600;
    margin-bottom: 8px;
  }
  .sub-title {
    font-size: 22px;
    font-weight: 800;
    color: var(--x-text);
    margin: 0;
  }
  .sub-desc {
    color: var(--x-text-2);
    font-size: 13px;
    margin-top: 4px;
    line-height: 1.5;
  }
}

.primary-card {
  border-color: rgba(99, 102, 241, 0.25);
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.06);
}

.sub-main-grid {
  display: flex;
  gap: 20px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;

  .sub-left {
    flex: 1;
    min-width: 260px;
  }
  .sub-right {
    flex: none;
    cursor: pointer;
  }
}

.sub-url-box {
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 12px 14px;
  margin-bottom: 14px;
  .sub-url-label {
    display: block;
    font-size: 11px;
    letter-spacing: 0.3px;
    color: var(--x-text-3);
    font-weight: 600;
    margin-bottom: 4px;
  }
  .sub-url-code {
    display: block;
    word-break: break-all;
    font-size: 12.5px;
    color: var(--x-text);
  }
}

.sub-action-buttons {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;

  .glow-btn {
    box-shadow: 0 2px 10px rgba(99, 102, 241, 0.25);
  }
}

.sub-minor-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  transition: all 0.2s ease;
  &:hover {
    border-color: var(--x-primary);
    transform: translateY(-2px);
    box-shadow: var(--x-shadow-md);
  }
  .sub-qr {
    width: 100px;
    height: 100px;
    border-radius: 6px;
  }
  .sub-qr-empty {
    width: 100px;
    height: 100px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--x-text-3);
  }
  .qr-tip {
    font-size: 11px;
    color: var(--x-text-2);
    margin-top: 6px;
    display: flex;
    align-items: center;
  }
}

.section-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--x-text);
  margin: 20px 0 12px;
}

.client-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 12px;
}

.client-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 16px;
  box-shadow: var(--x-shadow);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  transition: all 0.2s ease;

  &:hover {
    transform: translateY(-2px);
    border-color: rgba(99, 102, 241, 0.35);
    box-shadow: var(--x-shadow-md);
  }

  .client-head {
    display: flex;
    gap: 10px;
    align-items: flex-start;
  }
  .client-meta {
    min-width: 0;
    flex: 1;
  }
  .client-name {
    font-size: 14.5px;
    font-weight: 700;
    color: var(--x-text);
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .client-tag {
    font-size: 10.5px;
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--x-primary-soft);
    color: var(--x-primary);
    font-weight: 600;
  }
  .client-platform-tags {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    margin-top: 6px;

    .platform-chip {
      font-size: 10.5px;
      padding: 1px 5px;
      border-radius: 3px;
      background: var(--x-card-soft);
      border: 1px solid var(--x-border);
      color: var(--x-text-3);
    }
  }
  .client-desc {
    font-size: 12px;
    color: var(--x-text-2);
    line-height: 1.5;
    margin: 10px 0 12px;
  }
  .client-footer {
    display: flex;
    justify-content: flex-end;
  }
  .client-link {
    font-size: 12px;
    color: var(--x-primary);
    font-weight: 600;
    display: flex;
    align-items: center;
    &:hover {
      text-decoration: underline;
    }
  }
}

.sub-steps {
  padding: 14px 16px;
  display: grid;
  gap: 12px;
}
.sub-step {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  .step-num {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--x-primary);
    color: #fff;
    font-size: 12px;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    margin-top: 2px;
  }
  .step-info {
    flex: 1;
  }
  .step-title {
    font-size: 13.5px;
    font-weight: 600;
    color: var(--x-text);
  }
  .step-desc {
    font-size: 12px;
    color: var(--x-text-2);
    margin-top: 2px;
    line-height: 1.5;
  }
}
</style>