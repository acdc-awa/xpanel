<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CopyDocument, Download, Monitor, Check } from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'

const auth = useAuthStore()
const qrDataUrl = ref('')
const loading = ref(false)

const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? `${location.origin}/api/v1/sub/${token}` : ''
})

onMounted(async () => {
  loading.value = true
  try {
    if (!auth.user) await auth.fetchMe()
    if (subscribeUrl.value) {
      qrDataUrl.value = await QRCode.toDataURL(subscribeUrl.value, {
        width: 220,
        margin: 1,
        color: { dark: '#1e2333', light: '#ffffff' },
      })
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成二维码失败'))
  } finally {
    loading.value = false
  }
})

function copy() {
  navigator.clipboard?.writeText(subscribeUrl.value).then(
    () => ElMessage.success('订阅链接已复制'),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <div class="sub-hero">
      <div class="sub-title">订阅中心</div>
      <p class="sub-desc">复制订阅链接或扫码导入 Clash 系客户端，即可连接全部可用节点</p>
    </div>

    <div class="x-card">
      <div class="x-card-body" style="display: flex; flex-direction: column; align-items: center; gap: 14px; padding: 22px 16px">
        <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" class="sub-qr" />
        <div v-else class="sub-qr sub-qr-empty">—</div>
        <code class="sub-url">{{ subscribeUrl || '加载中…' }}</code>
        <div style="display: flex; gap: 10px; width: 100%; max-width: 340px">
          <el-button type="primary" style="flex: 1" @click="copy"><el-icon><CopyDocument /></el-icon>&nbsp;复制链接</el-button>
        </div>
      </div>
    </div>

    <!-- 使用说明 -->
    <div class="x-card" style="margin-top: 14px">
      <div class="x-card-head"><span>导入教程</span></div>
      <div class="sub-steps">
        <div class="sub-step"><span class="step-num">1</span><span>安装 Clash 系客户端（Clash Verge / Mihomo / Stash 等）</span></div>
        <div class="sub-step"><span class="step-num">2</span><span>复制上方订阅链接，在客户端「订阅」中添加</span></div>
        <div class="sub-step"><span class="step-num">3</span><span>选择「🚀 节点选择」或「♻️ 自动选择」节点组，开启系统代理</span></div>
      </div>
      <p class="muted" style="font-size: 12px; padding: 0 16px 14px">
        流量用量与到期时间会显示在首页与账户页；支持自动更新（ETag 增量）。
      </p>
    </div>

    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 14px">
      <router-link to="/dashboard"><el-button style="width: 100%">返回首页</el-button></router-link>
      <router-link to="/account"><el-button style="width: 100%">我的账户</el-button></router-link>
    </div>
  </div>
</template>

<style scoped lang="scss">
.sub-hero { margin-bottom: 14px; }
.sub-title { font-size: 19px; font-weight: 700; }
.sub-desc { color: var(--x-text-3); font-size: 13px; margin-top: 4px; }
.sub-qr {
  width: 220px;
  height: 220px;
  border-radius: 12px;
  background: #fff;
  border: 1px solid var(--x-border);
}
.sub-qr-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--x-text-3);
  font-size: 32px;
}
.sub-url {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--x-text-2);
  word-break: break-all;
  background: var(--x-bg);
  border-radius: 8px;
  padding: 10px 12px;
  width: 100%;
  max-width: 340px;
}
.sub-steps { display: grid; gap: 12px; padding: 14px 16px; }
.sub-step { display: flex; gap: 10px; align-items: flex-start; font-size: 13.5px; }
.step-num {
  flex: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--x-primary);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>