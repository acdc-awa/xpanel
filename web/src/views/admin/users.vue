<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Plus, Search, Setting, Lock, Unlock, Delete, CopyDocument, Wallet, RefreshRight, Download, MoreFilled, Loading, User } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  getUsers,
  createUser,
  toggleUser,
  resetUserTraffic,
  deleteUser,
  updateUser,
  getPlans,
  getPermissionGroups,
  type PermissionGroup,
  type Plan,
} from '@/api/admin'
import { adjustUserBalance } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import type { AdminUser } from '@/api/types'
import { formatBytes } from '@/utils/format'

const list = ref<AdminUser[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)
const keyword = ref('')

const isMobile = ref(false)
let mq: MediaQueryList | null = null
const onMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
}
onMounted(() => {
  mq = window.matchMedia('(max-width: 768px)')
  onMq(mq)
  mq.addEventListener('change', onMq)
})
onUnmounted(() => {
  mq?.removeEventListener('change', onMq)
  window.clearTimeout(searchTimer)
})

const plans = ref<Plan[]>([])
const permissionGroups = ref<PermissionGroup[]>([])

async function loadAuxData() {
  try {
    const [pRes, gRes] = await Promise.all([getPlans(), getPermissionGroups()])
    if (pRes.data.code === 0) plans.value = pRes.data.data.items
    if (gRes.data.code === 0) permissionGroups.value = gRes.data.data.items
  } catch {
    /* 忽略 */
  }
}

function planName(id: number) {
  return plans.value.find((p) => p.id === id)?.name ?? (id ? `#${id}` : '无套餐')
}

function groupName(id?: number) {
  if (!id) return '无'
  return permissionGroups.value.find((g) => g.id === id)?.name ?? `组#${id}`
}

function userGroupDisplay(u: any) {
  if (u.permission_group_id && u.permission_group_id > 0) {
    return { name: `${groupName(u.permission_group_id)} (自定义)`, type: 'primary', custom: true }
  }
  if (u.plan_id && u.plan_id > 0) {
    const p = plans.value.find((x) => x.id === u.plan_id)
    if (p && p.permission_group_id) {
      return { name: `${groupName(p.permission_group_id)} (套餐继承)`, type: 'success', custom: false }
    }
  }
  return { name: '未分配权限组', type: 'info', custom: false }
}

// ---- 调整余额（仅保留表格右键或独立调账备用） ----
const adjustOpen = ref(false)
const adjustUser = ref<AdminUser | null>(null)
const adjustForm = reactive({
  delta_yuan: 0,
  remark: '',
})
const adjusting = ref(false)

function openAdjust(row: any) {
  adjustUser.value = row
  adjustForm.delta_yuan = 0
  adjustForm.remark = ''
  adjustOpen.value = true
}

async function submitAdjust() {
  if (!adjustUser.value) return
  if (adjustForm.delta_yuan === 0) {
    ElMessage.warning('变动金额不能为 0')
    return
  }
  adjusting.value = true
  try {
    const payload = {
      amount_cents: Math.round(adjustForm.delta_yuan * 100),
      remark: adjustForm.remark || undefined,
    }
    const { data } = await adjustUserBalance(adjustUser.value.id, payload)
    if (data.code === 0) {
      ElMessage.success('余额调整成功')
      adjustOpen.value = false
      if (current.value && current.value.id === adjustUser.value.id) {
        current.value.balance_cents = data.data.new_balance
      }
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '调账失败'))
  } finally {
    adjusting.value = false
  }
}

let searchTimer: number | undefined
function onSearchInput() {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(onSearch, 300)
}

function onSearch() {
  page.value = 1
  load()
}

async function load() {
  loading.value = true
  try {
    const { data } = await getUsers(page.value, size.value, keyword.value.trim())
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

onMounted(async () => {
  await Promise.all([load(), loadAuxData()])
})

function usagePercent(u: any) {
  if (!u.total_bytes) return 0
  return Math.min(100, Math.round((u.used_bytes / u.total_bytes) * 100))
}

function fmtTime(t: string | null) {
  return t ? t.replace('T', ' ').slice(0, 16) : '—'
}

// ---- 时间快捷设定 ----
const dateShortcuts = [
  {
    text: '+1 个月 (30天)',
    value: () => {
      const d = new Date()
      d.setDate(d.getDate() + 30)
      return d
    },
  },
  {
    text: '+1 季度 (90天)',
    value: () => {
      const d = new Date()
      d.setDate(d.getDate() + 90)
      return d
    },
  },
  {
    text: '+半年 (180天)',
    value: () => {
      const d = new Date()
      d.setDate(d.getDate() + 180)
      return d
    },
  },
  {
    text: '+1 年 (365天)',
    value: () => {
      const d = new Date()
      d.setDate(d.getDate() + 365)
      return d
    },
  },
]

function addQuickExpire(days: number) {
  const base = userEditForm.expire_at ? new Date(userEditForm.expire_at) : new Date()
  const start = base.getTime() < Date.now() ? new Date() : base
  start.setDate(start.getDate() + days)
  userEditForm.expire_at = start.toISOString()
}

function clearExpire() {
  userEditForm.expire_at = ''
}

function addNewUserQuickExpire(days: number) {
  const base = newUserForm.expire_at ? new Date(newUserForm.expire_at) : new Date()
  const start = base.getTime() < Date.now() ? new Date() : base
  start.setDate(start.getDate() + days)
  newUserForm.expire_at = start.toISOString()
}

function clearNewUserExpire() {
  newUserForm.expire_at = ''
}

// ---- 详情与编辑抽屉 ----
const detailOpen = ref(false)
const current = ref<AdminUser | null>(null)
const userEditForm = reactive({
  role: 'user' as 'admin' | 'user',
  plan_id: 0,
  permission_group_id: 0,
  device_limit: 0,
  expire_at: '',
  password: '',
})
const userSaving = ref(false)

const currentPlan = computed(() => {
  if (!userEditForm.plan_id) return null
  return plans.value.find((p) => p.id === userEditForm.plan_id) || null
})

const inheritedGroupName = computed(() => {
  if (!currentPlan.value || !currentPlan.value.permission_group_id) {
    return '未分配权限组'
  }
  return groupName(currentPlan.value.permission_group_id)
})

function openDetail(row: any) {
  current.value = row
  userEditForm.role = row.role || 'user'
  userEditForm.plan_id = row.plan_id || 0
  userEditForm.permission_group_id = row.permission_group_id || 0
  userEditForm.device_limit = row.device_limit || 0
  userEditForm.expire_at = row.expire_at || ''
  userEditForm.password = ''
  detailOpen.value = true
}

async function saveUserConfig() {
  if (!current.value) return
  userSaving.value = true
  try {
    const payload = {
      role: userEditForm.role,
      plan_id: userEditForm.plan_id,
      permission_group_id: userEditForm.permission_group_id,
      device_limit: userEditForm.device_limit,
      expire_at: userEditForm.expire_at || null,
      password: userEditForm.password.trim() || undefined,
    }
    const { data } = await updateUser(current.value.id, payload)
    if (data.code === 0) {
      ElMessage.success('用户配置已更新')
      detailOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    userSaving.value = false
  }
}

// ---- 手动创建用户 ----
const newUserOpen = ref(false)
const newUserForm = reactive({
  email: '',
  password: '',
  plan_id: 0,
  permission_group_id: 0,
  device_limit: 0,
  expire_at: '',
})
const newUserCreating = ref(false)
const newUserResult = ref<{ uuid: string; email: string } | null>(null)

async function submitNewUser() {
  if (!newUserForm.email || !newUserForm.password) {
    ElMessage.warning('请填写邮箱与密码')
    return
  }
  newUserCreating.value = true
  try {
    const payload = {
      email: newUserForm.email,
      password: newUserForm.password,
      plan_id: newUserForm.plan_id || undefined,
      permission_group_id: newUserForm.permission_group_id || undefined,
      device_limit: newUserForm.device_limit || undefined,
      expire_at: newUserForm.expire_at || undefined,
    }
    const { data } = await createUser(payload)
    if (data.code === 0) {
      newUserResult.value = { uuid: data.data.uuid, email: data.data.email }
      newUserForm.email = ''
      newUserForm.password = ''
      newUserForm.plan_id = 0
      newUserForm.permission_group_id = 0
      newUserForm.device_limit = 0
      newUserForm.expire_at = ''
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建失败'))
  } finally {
    newUserCreating.value = false
  }
}

function closeNewUser() {
  newUserOpen.value = false
  newUserResult.value = null
}

async function doToggle(row: any) {
  try {
    const { data } = await toggleUser(row.id)
    if (data.code === 0) {
      ElMessage.success(data.message)
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function doResetTraffic(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认重置「${row.username}」的流量周期？重置后其已用流量清零并重新计算限额。`,
      '重置流量',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const { data } = await resetUserTraffic(row.id)
    if (data.code === 0) {
      ElMessage.success('流量已重置')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  }
}

async function doDelete(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除用户 ${row.username}？该操作不可逆。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      confirmButtonClass: 'el-button--danger',
      cancelButtonText: '取消',
    })
    const { data } = await deleteUser(row.id)
    if (data.code === 0) {
      ElMessage.success('用户已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch {
    /* 取消 */
  }
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text)
  ElMessage.success(`${label}已复制到剪贴板`)
}

// ---- 批量操作与 CSV 导出（ISSUE-17 用户管理增强） ----
const selected = ref<AdminUser[]>([])
const batchBusy = ref(false)

function csvCell(v: any) {
  const s = String(v ?? '')
  return `"${s.split('"').join('""')}"`
}

function exportCSV() {
  const rows = selected.value.length ? selected.value : list.value
  if (!rows.length) {
    ElMessage.warning('没有可导出的用户')
    return
  }
  const header = ['ID', '用户名', '邮箱', 'UUID', '角色', '状态', '套餐', '权限组', '设备限制', '余额(分)', '已用流量', '总流量', '到期时间', '注册时间']
  const lines = rows.map((u: any) =>
    [
      u.id,
      u.username,
      u.email,
      u.uuid,
      u.role,
      u.status === 1 ? '正常' : '禁用',
      planName(u.plan_id),
      userGroupDisplay(u).name,
      u.effective_device_limit || 0,
      u.balance_cents || 0,
      u.used_bytes || 0,
      u.total_bytes || 0,
      u.expire_at ? String(u.expire_at).replace('T', ' ').slice(0, 19) : '',
      u.created_at ? String(u.created_at).replace('T', ' ').slice(0, 19) : '',
    ].map(csvCell).join(','),
  )
  const csv = '\ufeff' + [header.map(csvCell).join(','), ...lines].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `users-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success(`已导出 ${rows.length} 位用户`)
}

async function batchToggle(targetStatus: 0 | 1) {
  const rows = selected.value.filter((u) => u.status !== targetStatus)
  if (!rows.length) {
    ElMessage.warning(targetStatus === 1 ? '所选用户均已启用' : '所选用户均已禁用')
    return
  }
  try {
    await ElMessageBox.confirm(
      targetStatus === 1
        ? `确认启用所选 ${rows.length} 位用户？`
        : `确认封禁所选 ${rows.length} 位用户？封禁后其会话与订阅立即失效。`,
      targetStatus === 1 ? '批量启用' : '批量封禁',
      { type: 'warning' },
    )
  } catch {
    return
  }
  batchBusy.value = true
  try {
    let ok = 0
    for (const u of rows) {
      const { data } = await toggleUser(u.id)
      if (data.code === 0) ok++
    }
    ElMessage.success(`已完成 ${ok}/${rows.length}`)
    selected.value = []
    load()
  } catch (e) {
    ElMessage.error(errMsg(e, '批量操作失败'))
  } finally {
    batchBusy.value = false
  }
}

// 移动端卡片多选辅助
function isRowSelected(row: AdminUser): boolean {
  return selected.value.some((u) => u.id === row.id)
}

function toggleSelectRow(row: AdminUser, checked: boolean) {
  if (checked) {
    if (!isRowSelected(row)) {
      selected.value = [...selected.value, row]
    }
  } else {
    selected.value = selected.value.filter((u) => u.id !== row.id)
  }
}

const isAllSelected = computed(() => {
  if (!list.value.length) return false
  return list.value.every((u) => isRowSelected(u))
})

const isIndeterminate = computed(() => {
  const count = list.value.filter((u) => isRowSelected(u)).length
  return count > 0 && count < list.value.length
})

function toggleSelectAll(checked: boolean) {
  if (checked) {
    const map = new Map<number, AdminUser>()
    for (const u of selected.value) map.set(u.id, u)
    for (const u of list.value) map.set(u.id, u)
    selected.value = Array.from(map.values())
  } else {
    const set = new Set(list.value.map((u) => u.id))
    selected.value = selected.value.filter((u) => !set.has(u.id))
  }
}

function handleCardAction(cmd: string, row: AdminUser) {
  switch (cmd) {
    case 'adjust':
      openAdjust(row)
      break
    case 'reset_traffic':
      doResetTraffic(row)
      break
    case 'toggle':
      doToggle(row)
      break
    case 'delete':
      doDelete(row)
      break
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input
          v-model="keyword"
          placeholder="搜索用户名 / 邮箱 / UUID"
          clearable
          style="width: 280px"
          @keyup.enter="onSearch"
          @clear="onSearch"
          @input="onSearchInput"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
      </div>
      <div class="toolbar-actions">
        <el-button :size="isMobile ? 'small' : 'default'" :icon="Download" @click="exportCSV">导出 CSV</el-button>
        <el-button :size="isMobile ? 'small' : 'default'" :loading="batchBusy" type="warning" @click="batchToggle(0)">批量封禁</el-button>
        <el-button :size="isMobile ? 'small' : 'default'" :loading="batchBusy" type="success" @click="batchToggle(1)">批量启用</el-button>
        <el-button :size="isMobile ? 'small' : 'default'" type="primary" :icon="Plus" @click="newUserOpen = true">添加用户</el-button>
      </div>
    </div>

    <BaseCard title="用户列表">
      <template #extra>
        <div class="user-select-bar">
          <el-checkbox
            :model-value="isAllSelected"
            :indeterminate="isIndeterminate"
            @change="(v: any) => toggleSelectAll(!!v)"
          >
            全选当前页 (已选 {{ selected.length }} / {{ total }})
          </el-checkbox>
        </div>
      </template>

      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" class="user-empty-state">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><User /></el-icon>
        <p style="margin-top: 8px">暂无匹配用户</p>
      </div>

      <!-- 全局统一卡片网格流 (自适应 1~4 列) -->
      <div v-else class="user-card-grid">
        <div
          v-for="row in list"
          :key="row.id"
          class="user-card"
          :class="{ selected: isRowSelected(row), banned: row.status !== 1 }"
        >
          <!-- 卡片头部 -->
          <div class="card-head">
            <div class="head-left">
              <el-checkbox
                :model-value="isRowSelected(row)"
                @change="(val: any) => toggleSelectRow(row, !!val)"
              />
              <div class="user-info">
                <div class="user-name-line">
                  <span class="user-id cell-mono">#{{ row.id }}</span>
                  <span class="user-name" title="点击查看与编辑详情" @click="openDetail(row)">{{ row.username }}</span>
                  <span v-if="row.role === 'admin'" class="x-chip orange" style="font-size: 10px; padding: 1px 5px">管理员</span>
                  <span class="x-chip" :class="row.status === 1 ? 'green' : 'red'" style="font-size: 10px; padding: 1px 5px">
                    {{ row.status === 1 ? '正常' : '已封禁' }}
                  </span>
                </div>
                <div class="user-email cell-mono">{{ row.email || '—' }}</div>
              </div>
            </div>
            <div class="head-right">
              <el-tooltip :content="row.status === 1 ? '点击封禁用户' : '点击解封用户'" placement="top">
                <el-switch :model-value="row.status === 1" size="small" @change="doToggle(row)" />
              </el-tooltip>
            </div>
          </div>

          <!-- 关键指标 2x2 网格 -->
          <div class="card-body-grid">
            <div class="grid-item">
              <span class="grid-label">套餐计划</span>
              <span class="grid-value">
                <span class="x-chip purple" style="font-size: 11px">
                  {{ planName(row.plan_id) }}
                </span>
              </span>
            </div>

            <div class="grid-item">
              <span class="grid-label">所属权限组</span>
              <span class="grid-value">
                <span class="x-chip" :class="userGroupDisplay(row).custom ? 'blue' : 'green'" style="font-size: 10.5px">
                  {{ userGroupDisplay(row).name }}
                </span>
              </span>
            </div>

            <div class="grid-item">
              <span class="grid-label">账户余额</span>
              <span class="grid-value balance-value cell-mono">
                ¥ {{ (((row as any).balance_cents || 0) / 100).toFixed(2) }}
              </span>
            </div>

            <div class="grid-item">
              <span class="grid-label">设备限制</span>
              <span class="grid-value device-value cell-mono">
                {{ row.is_custom_device_limit ? `${row.effective_device_limit} 台 (自定义)` : (row.effective_device_limit && row.effective_device_limit > 0) ? `${row.effective_device_limit} 台` : '不限' }}
              </span>
            </div>
          </div>

          <!-- 流量使用与到期 -->
          <div class="card-traffic">
            <div class="traffic-header">
              <span class="traffic-label">流量使用</span>
              <span class="traffic-val cell-mono">
                <strong>{{ formatBytes(row.used_bytes) }}</strong> / {{ row.total_bytes ? formatBytes(row.total_bytes) : '不限' }}
                <span v-if="row.total_bytes" class="percent">({{ usagePercent(row) }}%)</span>
              </span>
            </div>
            <el-progress
              v-if="row.total_bytes"
              :percentage="usagePercent(row)"
              :stroke-width="5"
              :show-text="false"
              :status="usagePercent(row) > 90 ? 'exception' : undefined"
            />
            <div class="traffic-expire muted cell-mono" style="font-size: 11px; margin-top: 6px">
              {{ row.expire_at ? fmtTime(row.expire_at) + ' 到期' : '长期有效' }}
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-actions">
            <el-button size="small" type="primary" plain @click="openDetail(row)">
              <el-icon><Setting /></el-icon>&nbsp;编辑配置
            </el-button>
            <el-button size="small" type="success" plain @click="openAdjust(row)">
              <el-icon><Wallet /></el-icon>&nbsp;调账
            </el-button>
            <el-dropdown trigger="click" @command="(cmd: string) => handleCardAction(cmd, row)">
              <el-button size="small" plain style="flex: none; padding: 0 8px">
                <el-icon><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="reset_traffic">
                    <el-icon><RefreshRight /></el-icon>重置流量
                  </el-dropdown-item>
                  <el-dropdown-item command="toggle">
                    <el-icon><Lock v-if="row.status === 1" /><Unlock v-else /></el-icon>
                    {{ row.status === 1 ? '封禁用户' : '解封用户' }}
                  </el-dropdown-item>
                  <el-dropdown-item divided command="delete" style="color: var(--el-color-danger)">
                    <el-icon><Delete /></el-icon>删除用户
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          :small="isMobile"
          :layout="isMobile ? 'prev, pager, next' : 'total, sizes, prev, pager, next'"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>

    <!-- 用户详情与权限配置抽屉 -->
    <el-drawer v-model="detailOpen" :title="`用户管理 · ${current?.username ?? ''}`" size="460px">
      <template v-if="current">
        <div class="detail-rows">
          <div class="row"><span class="k">用户名</span><span class="v">{{ current.username }}</span></div>
          <div class="row"><span class="k">邮箱</span><span class="v">{{ current.email || '—' }}</span></div>
          <div class="row">
            <span class="k">UUID</span>
            <code class="cell-mono" style="font-size: 11px; cursor: pointer; max-width: 240px; overflow: hidden; text-overflow: ellipsis" @click="copyText(current.uuid || '', 'UUID')">{{ current.uuid || '—' }}</code>
          </div>
          <div class="row">
            <span class="k">角色</span>
            <span class="v">
              <el-tag size="small" :type="current.role === 'admin' ? 'warning' : 'info'">
                {{ current.role === 'admin' ? '管理员' : '普通用户' }}
              </el-tag>
            </span>
          </div>
          <div class="row">
            <span class="k">账户余额</span>
            <span class="v cell-mono" style="font-weight: 700; color: #059669">
              ¥ {{ (((current as any).balance_cents || 0) / 100).toFixed(2) }}
            </span>
          </div>
          <div class="row"><span class="k">注册时间</span><span class="v">{{ fmtTime(current.created_at) }}</span></div>
        </div>

        <div class="sec-title" style="margin-top: 20px; font-weight: 600; font-size: 14px">套餐与权限设置</div>
        <p class="muted tip" style="font-size: 12px; margin-top: 4px; margin-bottom: 14px">
          用户节点权限由「生效权限组」决定（优先使用管理员指定的独立权限组，未指定时继承当前有效套餐的权限组；未分配权限组的用户无法连接任何节点）。
        </p>

        <el-form label-position="top">
          <el-form-item label="用户身份与角色">
            <el-radio-group v-model="userEditForm.role">
              <el-radio-button value="user">普通用户</el-radio-button>
              <el-radio-button value="admin">管理员</el-radio-button>
            </el-radio-group>
            <span class="muted tip" style="font-size: 12px; margin-top: 4px; display: block">
              管理员拥有登录管理后台与运维配置全权（系统必须至少保留 1 名激活状态管理员）。
            </span>
          </el-form-item>

          <el-form-item label="绑定套餐">
            <el-select v-model="userEditForm.plan_id" style="width: 100%" placeholder="选择套餐">
              <el-option :value="0" label="无套餐" />
              <el-option
                v-for="p in plans"
                :key="p.id"
                :value="p.id"
                :label="p.name"
              >
                <span>{{ p.name }}</span>
                <span class="muted" style="font-size: 12px; margin-left: 8px">({{ groupName(p.permission_group_id) }})</span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="所属权限组（覆盖套餐默认）">
            <el-select v-model="userEditForm.permission_group_id" style="width: 100%">
              <el-option :value="0" :label="`跟随套餐继承（当前: ${inheritedGroupName}）`" />
              <el-option
                v-for="g in permissionGroups"
                :key="g.id"
                :value="g.id"
                :label="g.name"
              >
                <span>{{ g.name }}</span>
                <span v-if="g.remark" class="muted" style="font-size: 12px; margin-left: 8px">({{ g.remark }})</span>
              </el-option>
            </el-select>
          </el-form-item>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="设备限制（台，0=跟随套餐）">
              <el-input-number
                v-model="userEditForm.device_limit"
                :controls="false"
                :min="0"
                :precision="0"
                placeholder="0"
                style="width: 100%"
              />
            </el-form-item>
          </div>

          <el-form-item label="到期时间">
            <el-date-picker
              v-model="userEditForm.expire_at"
              type="datetime"
              placeholder="留空表示永不过期"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :shortcuts="dateShortcuts"
              style="width: 100%"
            />
            <div class="quick-expire-bar">
              <span class="quick-label">快捷设定:</span>
              <el-button size="small" text bg @click="addQuickExpire(30)">+1个月</el-button>
              <el-button size="small" text bg @click="addQuickExpire(90)">+1季度</el-button>
              <el-button size="small" text bg @click="addQuickExpire(180)">+半年</el-button>
              <el-button size="small" text bg @click="addQuickExpire(365)">+1年</el-button>
              <el-button size="small" text bg type="danger" @click="clearExpire">永久/清空</el-button>
            </div>
          </el-form-item>

          <el-form-item label="重置密码（留空不修改）">
            <el-input v-model="userEditForm.password" type="password" show-password placeholder="输入新密码，留空保持原密码" />
          </el-form-item>

          <el-button type="primary" :loading="userSaving" style="width: 100%; margin-top: 10px" @click="saveUserConfig">
            保存用户配置
          </el-button>
        </el-form>

        <div class="detail-actions" style="margin-top: 24px; border-top: 1px dashed var(--x-border); padding-top: 16px">
          <el-button size="small" :type="current.status === 1 ? 'warning' : 'success'" @click="doToggle(current); detailOpen = false">
            {{ current.status === 1 ? '封禁该用户' : '解封该用户' }}
          </el-button>
          <el-button v-if="current.role !== 'admin'" size="small" type="danger" plain @click="doDelete(current); detailOpen = false">
            删除用户
          </el-button>
        </div>
      </template>
    </el-drawer>

    <!-- 手动创建用户弹窗 -->
    <el-dialog v-model="newUserOpen" title="添加用户" width="520px" @close="closeNewUser">
      <template v-if="!newUserResult">
        <el-form label-position="top">
          <el-form-item label="邮箱">
            <el-input v-model="newUserForm.email" placeholder="user@example.com" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="newUserForm.password" type="password" show-password placeholder="至少 8 位" />
          </el-form-item>
          <div class="form-grid-2">
            <el-form-item label="初始套餐（选填）">
              <el-select v-model="newUserForm.plan_id" style="width: 100%" placeholder="选择套餐">
                <el-option :value="0" label="无套餐" />
                <el-option v-for="p in plans" :key="p.id" :value="p.id" :label="p.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="权限组（选填）">
              <el-select v-model="newUserForm.permission_group_id" style="width: 100%" placeholder="跟随套餐">
                <el-option :value="0" label="跟随套餐" />
                <el-option v-for="g in permissionGroups" :key="g.id" :value="g.id" :label="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="设备限制（台，0=跟随套餐）">
              <el-input-number
                v-model="newUserForm.device_limit"
                :controls="false"
                :min="0"
                :precision="0"
                placeholder="0"
                style="width: 100%"
              />
            </el-form-item>
          </div>
          <el-form-item label="到期时间（选填）">
            <el-date-picker
              v-model="newUserForm.expire_at"
              type="datetime"
              placeholder="留空表示永不过期"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :shortcuts="dateShortcuts"
              style="width: 100%"
            />
            <div class="quick-expire-bar">
              <span class="quick-label">快捷设定:</span>
              <el-button size="small" text bg @click="addNewUserQuickExpire(30)">+1个月</el-button>
              <el-button size="small" text bg @click="addNewUserQuickExpire(90)">+1季度</el-button>
              <el-button size="small" text bg @click="addNewUserQuickExpire(180)">+半年</el-button>
              <el-button size="small" text bg @click="addNewUserQuickExpire(365)">+1年</el-button>
              <el-button size="small" text bg type="danger" @click="clearNewUserExpire">清空</el-button>
            </div>
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <el-alert type="success" :closable="false" show-icon title="用户已创建" style="margin-bottom: 12px" />
        <div class="secret-box">
          <div class="secret-row">
            <span class="k">邮箱</span>
            <code>{{ newUserResult.email }}</code>
          </div>
          <div class="secret-row">
            <span class="k">UUID</span>
            <code>{{ newUserResult.uuid }}</code>
            <el-button size="small" text @click="copyText(newUserResult.uuid, 'UUID')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <p class="muted" style="margin: 0; font-size: 12px">UUID 已生成，用户购买套餐或由管理员分配权限组后即可生效使用节点。</p>
        </div>
      </template>
      <template #footer>
        <template v-if="!newUserResult">
          <el-button @click="newUserOpen = false">取消</el-button>
          <el-button type="primary" :loading="newUserCreating" @click="submitNewUser">创建</el-button>
        </template>
        <template v-else>
          <el-button type="primary" @click="closeNewUser">完成</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- 独立余额调账弹窗 -->
    <el-dialog v-model="adjustOpen" title="用户余额调账" width="420px">
      <el-form label-position="top">
        <div class="adjust-header">
          <div class="user-meta">
            <span class="k">用户:</span>
            <span class="v" style="font-weight: 600">{{ adjustUser?.username }}</span>
          </div>
          <div class="user-meta">
            <span class="k">当前余额:</span>
            <span class="v cell-mono" style="font-weight: 700; color: var(--x-primary)">
              ¥ {{ (((adjustUser as any)?.balance_cents || 0) / 100).toFixed(2) }}
            </span>
          </div>
        </div>
        <el-form-item label="变动金额（元，支持负数扣款）" style="margin-top: 14px">
          <el-input-number
            v-model="adjustForm.delta_yuan"
            :controls="false"
            :precision="2"
            :step="10"
            placeholder="正数充值，负数扣减"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="调账备注（选填）">
          <el-input v-model="adjustForm.remark" placeholder="如：活动赠送、售后退款、误充扣除" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="adjustOpen = false">取消</el-button>
        <el-button type="primary" :loading="adjusting" @click="submitAdjust">确认调账</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.desktop-table-view {
  display: block;
}
.mobile-cards-view {
  display: none;
}

.traffic-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.traffic-text {
  font-size: 12px;
}
.x-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 8px 16px;
}
.detail-rows {
  background: var(--x-bg, #f4f5fb);
  border-radius: var(--x-radius-sm, 8px);
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}
.detail-rows .row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
}
.detail-rows .k {
  color: var(--x-text-2, #6b7280);
}
.detail-rows .v {
  font-weight: 500;
  color: var(--x-text, #1e2333);
}
.secret-box {
  background: var(--x-bg, #f4f5fb);
  border-radius: var(--x-radius-sm, 8px);
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.secret-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
}
.secret-row .k {
  color: var(--x-text-2, #6b7280);
}
.secret-row code {
  font-size: 12px;
}
.adjust-header {
  background: var(--x-bg, #f4f5fb);
  border-radius: 8px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}
.adjust-header .user-meta {
  display: flex;
  justify-content: space-between;
}

.quick-expire-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.quick-label {
  font-size: 12px;
  color: var(--x-text-3, #9ca3af);
  margin-right: 2px;
}

.toolbar-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* ================= 全局统一响应式卡片网格流 ================= */
.user-select-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-empty-state {
  text-align: center;
  padding: 48px 0;
  color: var(--x-text-3);
  font-size: 13.5px;
}

.user-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.user-card {
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

  &.selected {
    border-color: var(--x-primary, #6366f1);
    background: rgba(99, 102, 241, 0.02);
  }
  &.banned {
    opacity: 0.88;
    border-color: rgba(239, 68, 68, 0.25);
    background: rgba(239, 68, 68, 0.02);
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-left {
      display: flex;
      align-items: flex-start;
      gap: 10px;
      flex: 1;
      min-width: 0;
    }

    .user-info {
      flex: 1;
      min-width: 0;
    }

    .user-name-line {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .user-id {
      font-size: 11px;
      font-weight: 700;
      color: var(--x-text-3, #9ca3af);
      background: var(--x-fill-2, rgba(0, 0, 0, 0.05));
      padding: 1px 5px;
      border-radius: 4px;
    }

    .user-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
      word-break: break-all;
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
    }

    .user-email {
      font-size: 11.5px;
      color: var(--x-text-2, #6b7280);
      margin-top: 2px;
      word-break: break-all;
    }

    .head-right {
      padding-left: 8px;
      flex: none;
    }
  }

  .card-body-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      .grid-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .grid-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }

      .balance-value {
        color: #059669;
        font-weight: 700;
      }

      .device-value {
        font-size: 12px;
      }
    }
  }

  .card-traffic {
    background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
    border: 1px solid var(--x-border, #f1f3f9);
    border-radius: 6px;
    padding: 8px 10px;
    margin: 2px 0 10px;

    .traffic-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-size: 11.5px;
      margin-bottom: 5px;

      .traffic-label {
        color: var(--x-text-2, #6b7280);
      }

      .traffic-val {
        color: var(--x-text, #111827);
        .percent {
          color: var(--x-text-3, #9ca3af);
          margin-left: 2px;
        }
      }
    }
  }

  .card-actions {
    display: flex;
    gap: 6px;
    justify-content: flex-end;
    padding-top: 4px;

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
    }
  }
}

@media (max-width: 768px) {
  .x-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;

    .x-toolbar-left {
      width: 100%;
      .el-input {
        width: 100% !important;
      }
    }

    .toolbar-actions {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 6px;
      width: 100%;

      .el-button {
        margin: 0;
        width: 100%;
        height: 32px;
        font-size: 12.5px;
      }
    }
  }

  .x-pager {
    justify-content: center;
    padding: 12px 0 4px;
  }
}

@media (max-width: 640px) {
  .user-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>