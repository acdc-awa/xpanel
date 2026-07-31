<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Search, Edit, Delete, Lightning } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { mockPlans } from '@/mock/data'
import { formatMoney } from '@/utils/format'

const activeTab = ref('plans')
const subscribePath = ref('/api/v1/client/subscribe')
const cacheSeconds = ref(60)
</script>

<template>
  <div class="x-page">
    <el-tabs v-model="activeTab">
      <el-tab-pane label="套餐列表" name="plans">
        <div class="x-toolbar">
          <el-input placeholder="搜索套餐名称" :prefix-icon="Search" clearable style="width: 240px" />
          <el-button type="primary"><el-icon><Plus /></el-icon>&nbsp;新建套餐</el-button>
        </div>

        <BaseCard>
          <el-table :data="mockPlans">
            <el-table-column prop="name" label="名称" min-width="120">
              <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
            </el-table-column>
            <el-table-column label="价格" width="100">
              <template #default="{ row }">{{ formatMoney(row.price) }}</template>
            </el-table-column>
            <el-table-column label="流量" width="110">
              <template #default="{ row }">{{ row.trafficGb >= 1024 ? `${(row.trafficGb / 1024).toFixed(0)} TB` : `${row.trafficGb} GB` }}</template>
            </el-table-column>
            <el-table-column label="周期" width="100">
              <template #default="{ row }">{{ row.durationDays }} 天</template>
            </el-table-column>
            <el-table-column prop="speedLimit" label="限速" width="110">
              <template #default="{ row }"><span class="muted">{{ row.speedLimit ?? '不限' }}</span></template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '上架' : '下架' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="110" fixed="right">
              <template #default>
                <el-button size="small" text><el-icon><Edit /></el-icon></el-button>
                <el-button size="small" text type="danger"><el-icon><Delete /></el-icon></el-button>
              </template>
            </el-table-column>
          </el-table>
        </BaseCard>
      </el-tab-pane>

      <el-tab-pane label="订阅设置" name="subscription">
        <BaseCard title="订阅配置">
          <el-form label-width="110px" style="max-width: 520px">
            <el-form-item label="订阅路径">
              <el-input v-model="subscribePath" />
            </el-form-item>
            <el-form-item label="缓存时间（秒）">
              <el-input-number v-model="cacheSeconds" :min="0" :max="3600" />
            </el-form-item>
            <el-form-item label="说明">
              <div class="x-pill-note" style="width: 100%">
                <el-icon><Lightning /></el-icon>
                <span>UA 识别：含 clash / mihomo / stash / verge 返回 Clash YAML，其他返回 Base64 链接列表。每个用户拥有独立 subscribe_token，可自行重置。</span>
              </div>
            </el-form-item>
            <el-form-item><el-button type="primary">保存</el-button></el-form-item>
          </el-form>
        </BaseCard>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
</style>
