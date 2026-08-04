<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Search, View, Key, Ticket } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getUsers, createInvitations } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { AdminUser } from '@/api/types'
import { formatBytes } from '@/utils/format'

const list = ref<AdminUser[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await getUsers(page.value, size.value)
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载用户失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function usagePercent(u: any) {
  if (!u.total_bytes) return 0
  return Math.min(100, Math.round((u.used_bytes / u.total_bytes) * 100))
}

function fmtTime(t: string | null) {
  return t ? t.replace('T', ' ').slice(0, 16) : '—'
}

// ---- 详情抽屉 ----
const detailOpen = ref(false)
const current = ref<AdminUser | null>(null)
function openDetail(row: any) {
  current.value = row
  detailOpen.value = true
}

// ---- 生成邀请码 ----
const invOpen = ref(false)
const invForm = reactive({ count: 5, expires: '' })
const invCreating = ref(false)
const generated = ref<string[]>([])

async function createInv() {
  invCreating.value = true
  try {
    const { data } = await createInvitations(invForm.count, invForm.expires)
    if (data.code === 0) {
      generated.value = data.data.codes
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    invCreating.value = false
  }
}

async function copyCodes() {
  try {
    await navigator.clipboard.writeText(generated.value.join('\n'))
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选择')
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input v-model="keyword" placeholder="搜索用户名 / 邮箱" :prefix-icon="Search" clearable style="width: 240px" @keyup.enter="load" />
        <el-button @click="load">搜索</el-button>
      </div>
      <el-button type="primary" @click="invOpen = true"><el-icon><Ticket /></el-icon>&nbsp;生成邀请码</el-button>
    </div>

    <BaseCard>
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="id" label="ID" width="80">
          <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" min-width="110">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.username }}</span></template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180">
          <template #default="{ row }"><code class="cell-mono">{{ row.email || '—' }}</code></template>
        </el-table-column>
        <el-table-column label="角色" width="90">
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'warning' : 'info'" size="small">{{ row.role === 'admin' ? '管理员' : '用户' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="套餐" width="100">
          <template #default="{ row }">
            <span class="muted">{{ row.plan_id ? `#${row.plan_id}` : '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="剩余流量" min-width="160">
          <template #default="{ row }">
            <div v-if="row.total_bytes > 0" class="usage-cell">
              <el-progress :percentage="usagePercent(row)" :stroke-width="8" :show-text="false" style="width: 80px" />
              <span class="muted">{{ formatBytes(Math.max(0, row.total_bytes - row.used_bytes)) }}</span>
            </div>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="到期时间" width="150">
          <template #default="{ row }">{{ fmtTime(row.expire_at) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '正常' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text @click="openDetail(row)"><el-icon><View /></el-icon></el-button>
          </template>
        </el-table-column>
      </el-table>

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

    <!-- 用户详情抽屉 -->
    <el-drawer v-model="detailOpen" :title="`用户详情 · ${current?.username ?? ''}`" size="420px">
      <template v-if="current">
        <div class="detail-rows">
          <div class="row"><span class="k">用户名</span><span class="v">{{ current.username }}</span></div>
          <div class="row"><span class="k">邮箱</span><span class="v">{{ current.email || '—' }}</span></div>
          <div class="row"><span class="k">角色</span><span class="v">{{ current.role === 'admin' ? '管理员' : '用户' }}</span></div>
          <div class="row"><span class="k">套餐</span><span class="v">{{ current.plan_id ? `#${current.plan_id}` : '—' }}</span></div>
          <div class="row"><span class="k">到期时间</span><span class="v">{{ fmtTime(current.expire_at) }}</span></div>
          <div class="row"><span class="k">注册时间</span><span class="v">{{ fmtTime(current.created_at) }}</span></div>
        </div>
        <div class="detail-actions">
          <el-button size="small" disabled><el-icon><Key /></el-icon>&nbsp;重置订阅 Token</el-button>
          <el-button size="small" disabled>调整套餐</el-button>
          <el-button size="small" type="danger" plain disabled>封禁</el-button>
          <p class="muted" style="width: 100%">以上操作随 P4/P5 管理端完善后开放</p>
        </div>
      </template>
    </el-drawer>

    <!-- 生成邀请码弹窗 -->
    <el-dialog v-model="invOpen" title="生成邀请码" width="440px">
      <template v-if="!generated.length">
        <el-form label-position="top">
          <el-form-item label="生成数量">
            <el-input-number v-model="invForm.count" :min="1" :max="100" style="width: 100%" />
          </el-form-item>
          <el-form-item label="过期时间（选填，RFC3339，如 2026-12-31T23:59:59+08:00）">
            <el-input v-model="invForm.expires" placeholder="留空 = 永不过期" />
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <p class="muted" style="margin: 0 0 8px">已生成 {{ generated.length }} 个邀请码（一次性使用）：</p>
        <div class="inv-codes">
          <code v-for="c in generated" :key="c" class="inv-code">{{ c }}</code>
        </div>
      </template>
      <template #footer>
        <template v-if="!generated.length">
          <el-button @click="invOpen = false">取消</el-button>
          <el-button type="primary" :loading="invCreating" @click="createInv">生成</el-button>
        </template>
        <template v-else>
          <el-button @click="invOpen = false; generated = []">完成</el-button>
          <el-button type="primary" @click="copyCodes">复制全部</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.x-pager { display: flex; justify-content: flex-end; padding: 14px 0 4px; }
.detail-rows .row { display: flex; align-items: center; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid var(--x-border); font-size: 13.5px; }
.detail-rows .k { color: var(--x-text-2); }
.detail-rows .v { font-weight: 500; }
.detail-actions { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 16px; }
.inv-codes { display: grid; gap: 8px; max-height: 260px; overflow: auto; }
.inv-code {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 13px;
  background: var(--x-primary-soft);
  color: var(--x-primary);
  border-radius: 8px;
  padding: 10px 12px;
  word-break: break-all;
}
</style>