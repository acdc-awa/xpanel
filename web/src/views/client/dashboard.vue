<script setup lang="ts">
import { computed } from 'vue'
import { mockClient, mockNotices, mockServers } from '@/mock/data'
import { formatGb } from '@/utils/format'

const usagePercent = computed(() => Math.min(100, Math.round((mockClient.usedGb / mockClient.totalGb) * 100)))

const statusText: Record<string, { dot: string; text: string }> = {
  online: { dot: 'online', text: '在线' },
  connecting: { dot: 'connecting', text: '维护中' },
  offline: { dot: 'offline', text: '离线' },
}
</script>

<template>
  <div class="x-client-body">
    <div class="x-dash-grid">
      <div>
        <!-- 用量卡片 -->
        <div class="x-usage-hero">
          <div class="x-plan-name">🚀 {{ mockClient.planName }} · 剩余 {{ formatGb(mockClient.totalGb - mockClient.usedGb) }}</div>
          <el-progress :percentage="usagePercent" :show-text="false" :stroke-width="8" color="#fff" class="hero-progress" />
          <div class="x-plan-meta">
            <span>已用 {{ formatGb(mockClient.usedGb) }} / {{ formatGb(mockClient.totalGb) }}</span>
            <span>到期 {{ mockClient.expireAt }}</span>
          </div>
        </div>

        <!-- 快捷操作 -->
        <div class="x-card">
          <div class="x-card-body" style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px">
            <el-button type="primary" size="large">订阅中心</el-button>
            <el-button size="large">购买套餐</el-button>
          </div>
        </div>
      </div>

      <div>
        <!-- 节点状态 -->
        <div class="x-card">
          <div class="x-card-head">
            <span>节点状态</span>
            <span class="muted" style="font-size: 12px; font-weight: 400">{{ mockClient.onlineNodes }} / {{ mockClient.totalNodes }} 在线</span>
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
</style>
