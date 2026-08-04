<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import { mockNotices, mockServers } from '@/mock/data'

const auth = useAuthStore()
const loading = ref(false)

// 套餐/到期/流量来自 /user/me（P2 已接入真实流量统计）
const planLabel = computed(() => (auth.user?.plan_id ? `套餐 #${auth.user.plan_id}` : '暂无套餐'))
const usedBytes = computed(() => auth.user?.used_bytes ?? 0)
const totalBytes = computed(() => auth.user?.total_bytes ?? 0)
const remainBytes = computed(() => Math.max(0, totalBytes.value - usedBytes.value))
const usagePercent = computed(() => {
  if (!totalBytes.value) return 0
  return Math.min(100, Math.round((usedBytes.value / totalBytes.value) * 100))
})
const totalText = computed(() => (totalBytes.value ? formatBytes(totalBytes.value) : '—'))
const remainText = computed(() => (totalBytes.value ? `剩余 ${formatBytes(remainBytes.value)}` : '未购买套餐'))
const expireText = computed(() => {
  const t = auth.user?.expire_at
  return t ? String(t).replace('T', ' ').slice(0, 16) : '—'
})

onMounted(async () => {
  if (auth.user) return
  loading.value = true
  try {
    await auth.fetchMe()
  } catch (e) {
    ElMessage.error(errMsg(e, '加载用户信息失败'))
  } finally {
    loading.value = false
  }
})

const statusText: Record<string, { dot: string; text: string }> = {
  online: { dot: 'online', text: '在线' },
  connecting: { dot: 'connecting', text: '维护中' },
  offline: { dot: 'offline', text: '离线' },
}
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <div class="x-dash-grid">
      <div>
        <!-- 用量卡片 -->
        <div class="x-usage-hero">
          <div class="x-plan-name">🚀 {{ planLabel }} · {{ remainText }}</div>
          <el-progress :percentage="usagePercent" :show-text="false" :stroke-width="8" color="#fff" class="hero-progress" />
          <div class="x-plan-meta">
            <span>已用 {{ formatBytes(usedBytes) }} / {{ totalText }}</span>
            <span>到期 {{ expireText }}</span>
          </div>
        </div>

        <!-- 快捷操作 -->
        <div class="x-card">
          <div class="x-card-body" style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px">
            <el-button type="primary" size="large">订阅中心</el-button>
            <router-link to="/shop"><el-button size="large" style="width: 100%">购买套餐</el-button></router-link>
          </div>
        </div>
      </div>

      <div>
        <!-- 节点状态 -->
        <div class="x-card">
          <div class="x-card-head">
            <span>节点状态</span>
            <span class="demo-tag">演示数据</span>
          </div>
          <div style="padding: 6px 16px">
            <div v-for="s in mockServers" :key="s.id" class="x-row-line">
              <span class="k"><span class="x-status-dot" :class="statusText[s.status].dot" />{{ s.location }} {{ s.name }}</span>
              <span class="v muted">{{ statusText[s.status].text }}</span>
            </div>
          </div>
        </div>

        <!-- 使用帮助 -->
        <div class="x-card">
          <div class="x-card-head"><span>使用帮助</span></div>
          <div style="display: grid; gap: 8px; padding: 10px 16px">
            <a class="muted" style="font-size: 13px" href="#">📖 客户端使用教程（Clash 导入）</a>
            <a class="muted" style="font-size: 13px" href="#">💬 联系客服 / 提交工单</a>
          </div>
        </div>

        <!-- 公告 -->
        <div class="x-card">
          <div class="x-card-head"><span>公告</span><a class="muted" style="font-size: 12px" href="#">更多</a></div>
          <div v-for="n in mockNotices" :key="n.id" class="x-notice-item">
            <div class="x-notice-title">{{ n.title }}</div>
            <div class="x-notice-date">{{ n.date }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.hero-progress { margin-top: 10px; --el-progress-bg-color: #fff; }
.muted { color: var(--x-text-3); }
.demo-tag {
  font-size: 11px;
  color: var(--x-warning);
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.35);
  border-radius: 6px;
  padding: 1px 7px;
  font-weight: 400;
}
</style>