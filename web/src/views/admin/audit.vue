<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getAuditLogs, type AuditLog } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

async function load() {
  loading.value = true
  try {
    const { data } = await getAuditLogs(page.value, size.value)
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载审计日志失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fmtTime(t: string) {
  return t.replace('T', ' ').slice(0, 19)
}

const actionText: Record<string, string> = {
  'auth.login': '登录',
  'order.create': '下单',
  'order.confirm': '确认订单',
  'order.cancel': '取消订单',
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <el-tag :type="row.operator_type === 'admin' ? 'warning' : 'info'" size="small">
                {{ row.operator_type === 'admin' ? '管理员' : '用户' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="operator_id" label="操作者 ID" width="100" />
          <el-table-column label="动作" width="110">
            <template #default="{ row }">{{ actionText[row.action] ?? row.action }}</template>
          </el-table-column>
          <el-table-column prop="detail" label="详情" min-width="220" />
          <el-table-column prop="ip" label="IP" width="140" />
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          暂无审计日志
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <el-tag :type="row.operator_type === 'admin' ? 'warning' : 'info'" size="small">
                  {{ row.operator_type === 'admin' ? '管理员' : '用户' }} #{{ row.operator_id }}
                </el-tag>
                <span style="font-weight: 700">{{ actionText[row.action] ?? row.action }}</span>
              </div>
              <code class="cell-mono muted" style="font-size: 11.5px">{{ row.ip || '—' }}</code>
            </div>

            <div class="card-grid">
              <div class="grid-item full-width">
                <span class="item-label">操作详情</span>
                <div class="item-value" style="font-size: 12px">{{ row.detail || '—' }}</div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">发生时间</span>
                <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ fmtTime(row.created_at) }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.x-pager { display: flex; justify-content: flex-end; padding: 14px 0 4px; }
</style>