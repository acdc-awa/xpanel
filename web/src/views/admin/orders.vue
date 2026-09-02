<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh, Tickets, Loading } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getOrders, type Order } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

async function load() {
  loading.value = true
  try {
    const { data } = await getOrders(page.value, size.value)
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载订单失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  return String(t).replace('T', ' ').slice(0, 16)
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard title="服务订购记录">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Tickets /></el-icon>
        <p style="margin-top: 8px">暂无订购记录</p>
      </div>

      <!-- 全局统一订单卡片网格流 (自适应 1~4 列) -->
      <div v-else class="order-card-grid">
        <div v-for="row in list" :key="row.id" class="order-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="plan-name">{{ row.plan_name }}</span>
              <span class="x-chip" :class="row.status === 'paid' ? 'green' : 'orange'" style="font-size: 10.5px">
                {{ row.status === 'paid' ? '已生效' : (row.status === 'pending' ? '待支付' : '已取消') }}
              </span>
            </div>
            <span class="cell-mono font-14" style="font-weight: 700; color: #059669">
              ¥ {{ (row.amount_cents / 100).toFixed(2) }}
            </span>
          </div>

          <!-- 订单属性网格 -->
          <div class="card-grid">
            <div class="grid-item">
              <span class="item-label">订购用户</span>
              <div class="item-value">
                <span class="x-chip purple" style="font-size: 11px">{{ row.username }}</span>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">支付方式</span>
              <div class="item-value">
                <span v-if="row.payment_method === 'balance'" class="x-chip green" style="font-size: 10.5px">
                  余额支付
                </span>
                <span v-else class="x-chip blue" style="font-size: 10.5px">
                  {{ row.payment_method }}
                </span>
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">订单流水号</span>
              <div class="item-value">
                <code class="cell-mono font-11 muted">{{ row.order_no }}</code>
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">支付生效时间</span>
              <div class="item-value cell-mono muted font-11">
                {{ row.paid_at ? fmtTime(row.paid_at) : fmtTime(row.created_at) }}
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
.cell-mono { font-family: var(--x-font-mono); font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }

/* ================= 全局统一订单卡片网格流 ================= */
.order-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.order-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: var(--x-radius, 10px);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  flex-direction: column;
  justify-content: space-between;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    box-shadow: var(--x-shadow-md);
    transform: translateY(-1px);
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .plan-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0 2px;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }
}

.x-pager {
  display: flex;
  justify-content: flex-end;
  padding: 14px 0 0;
  margin-top: 16px;
  border-top: 1px solid var(--x-border-light, #f1f5f9);
}
.x-toolbar-hint { font-size: 13px; color: var(--x-text-3); margin-right: 8px; }

@media (max-width: 640px) {
  .order-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>