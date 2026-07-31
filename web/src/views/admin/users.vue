<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Search, View, Edit, Delete, Key, Refresh } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { mockUsers, type PanelUser } from '@/mock/data'
import { formatGb } from '@/utils/format'

const detailOpen = ref(false)
const current = ref<PanelUser | null>(null)

function openDetail(row: any) {
  current.value = row
  detailOpen.value = true
}

function usagePercent(u: any) {
  if (!u.totalGb) return 0
  return Math.min(100, Math.round((u.usedGb / u.totalGb) * 100))
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input placeholder="搜索用户名 / 邮箱" :prefix-icon="Search" clearable style="width: 240px" />
        <el-select placeholder="全部状态" clearable style="width: 130px">
          <el-option label="正常" value="normal" />
          <el-option label="封禁" value="banned" />
        </el-select>
      </div>
      <el-button type="primary"><el-icon><Plus /></el-icon>&nbsp;新增用户</el-button>
    </div>

    <BaseCard>
      <el-table :data="mockUsers">
        <el-table-column prop="id" label="ID" width="80">
          <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" min-width="110">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.username }}</span></template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180">
          <template #default="{ row }"><code class="cell-mono">{{ row.email }}</code></template>
        </el-table-column>
        <el-table-column prop="plan" label="套餐" width="110" />
        <el-table-column label="剩余流量" min-width="160">
          <template #default="{ row }">
            <div v-if="row.totalGb" class="usage-cell">
              <el-progress :percentage="usagePercent(row)" :stroke-width="8" :show-text="false" style="width: 80px" />
              <span class="muted">{{ formatGb(Math.max(0, row.totalGb - row.usedGb)) }}</span>
            </div>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="expireAt" label="到期时间" width="110" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'normal' ? 'success' : 'danger'" size="small">{{ row.status === 'normal' ? '正常' : '封禁' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text @click="openDetail(row)"><el-icon><View /></el-icon></el-button>
            <el-button size="small" text><el-icon><Edit /></el-icon></el-button>
            <el-button size="small" text type="danger"><el-icon><Delete /></el-icon></el-button>
          </template>
        </el-table-column>
      </el-table>
    </BaseCard>

    <!-- 用户详情抽屉 -->
    <el-drawer v-model="detailOpen" :title="`用户详情 · ${current?.username ?? ''}`" size="420px">
      <template v-if="current">
        <div class="detail-rows">
          <div class="row"><span class="k">用户名</span><span class="v">{{ current.username }}</span></div>
          <div class="row"><span class="k">邮箱</span><span class="v">{{ current.email }}</span></div>
          <div class="row"><span class="k">套餐</span><span class="v">{{ current.plan }}</span></div>
          <div class="row"><span class="k">剩余流量</span><span class="v">{{ formatGb(Math.max(0, current.totalGb - current.usedGb)) }} / {{ formatGb(current.totalGb) }}</span></div>
          <div class="row"><span class="k">到期时间</span><span class="v">{{ current.expireAt }}</span></div>
          <div class="row"><span class="k">订阅 Token</span><span class="v"><code class="cell-mono">sk_••••••••9f0e</code></span></div>
        </div>
        <div class="detail-actions">
          <el-button size="small"><el-icon><Key /></el-icon>&nbsp;重置订阅 Token</el-button>
          <el-button size="small"><el-icon><Refresh /></el-icon>&nbsp;调整套餐</el-button>
          <el-button size="small" type="danger" plain><el-icon><View /></el-icon>&nbsp;封禁</el-button>
        </div>
      </template>
      <template #footer>
        <el-button @click="detailOpen = false">关闭</el-button>
        <el-button type="primary" @click="detailOpen = false">保存修改</el-button>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.usage-cell { display: flex; align-items: center; gap: 8px; }
.detail-rows .row { display: flex; align-items: center; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid var(--x-border); font-size: 13.5px; }
.detail-rows .k { color: var(--x-text-2); }
.detail-rows .v { font-weight: 500; }
.detail-actions { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 16px; }
</style>

