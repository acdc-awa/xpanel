<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { MQ, useMediaQuery } from '@/utils/media'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCheckbox,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsLoading,
  CsModal,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSegmentedControl,
  CsSelect,
  CsTable,
  CsToggle,
  CsToolbar,
} from '@/components/cloudscape'
import type { SegmentedOption, TableColumn } from '@/components/cloudscape'
import {
  getUsers,
  createUser,
  toggleUser,
  resetUserTraffic,
  deleteUser,
  updateUser,
  getPlans,
  getPermissionGroups,
  getUserSubscribeToken,
  resetUserSubscribeToken,
  type PermissionGroup,
  type Plan,
} from '@/api/admin'
import { adjustUserBalance } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import type { AdminUser } from '@/api/types'
import { formatBytes, formatMoney } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'
import { buildSubscribeUrl } from '@/config/site'
import { useSiteStore } from '@/stores/site'
import { useAuthStore } from '@/stores/auth'

const list = ref<AdminUser[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(10)
const keyword = ref('')
const statusFilter = ref<'' | '0' | '1'>('')
const planFilter = ref<'' | number>('')
const groupFilter = ref<'' | number>('')
const roleFilter = ref<'' | 'user' | 'admin'>('')

const isMobile = useMediaQuery(MQ.tablet)
onUnmounted(() => {
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

// CsSelect 只读 CsOption 的 label，选项里的副标题在这里拼成文本；
// 传 options 时子节点不参与选项收集，所以「无套餐 / 跟随套餐」也要并入数组。
const planOptions = computed(() => [
  { label: '无套餐', value: 0 },
  ...plans.value.map((p) => ({ label: `${p.name}（${groupName(p.permission_group_id)}）`, value: p.id })),
])

const groupOptions = computed(() => [
  { label: `跟随套餐继承（当前: ${inheritedGroupName.value}）`, value: 0 },
  ...permissionGroups.value.map((g) => ({
    label: g.remark ? `${g.name}（${g.remark}）` : g.name,
    value: g.id,
  })),
])

const roleOptions: SegmentedOption[] = [
  { id: 'user', text: '普通用户' },
  { id: 'admin', text: '管理员' },
]

// 新建用户只需名称，不展示权限组副标题
const newUserPlanOptions = computed(() => [
  { label: '无套餐', value: 0 },
  ...plans.value.map((p) => ({ label: p.name, value: p.id })),
])

const newUserGroupOptions = computed(() => [
  { label: '跟随套餐', value: 0 },
  ...permissionGroups.value.map((g) => ({ label: g.name, value: g.id })),
])

const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '正常', value: '1' },
  { label: '已封禁', value: '0' },
]

const planFilterOptions = computed(() => [
  { label: '全部套餐', value: '' },
  { label: '无套餐', value: 0 },
  ...plans.value.map((p) => ({ label: p.name, value: p.id })),
])

const groupFilterOptions = computed(() => [
  { label: '全部权限组', value: '' },
  { label: '未分配权限组', value: 0 },
  ...permissionGroups.value.map((g) => ({ label: g.name, value: g.id })),
])

const roleFilterOptions = [
  { label: '全部角色', value: '' },
  { label: '普通用户', value: 'user' },
  { label: '管理员', value: 'admin' },
]

const hasActiveFilters = computed(
  () =>
    keyword.value.trim() !== '' ||
    statusFilter.value !== '' ||
    planFilter.value !== '' ||
    groupFilter.value !== '' ||
    roleFilter.value !== '',
)

function groupName(id?: number) {
  if (!id) return '无'
  return permissionGroups.value.find((g) => g.id === id)?.name ?? `组#${id}`
}

function userGroupDisplay(u: any) {
  if (u.permission_group_id && u.permission_group_id > 0) {
    return { name: `${groupName(u.permission_group_id)} (自定义)`, type: 'primary', custom: true }
  }
  // 套餐继承读用户行快照推导的 effective_group_id（后端已下发），不读实时套餐——
  // 「改套餐权限组未勾同步」时存量用户实际生效组不变，显示不得漂移
  if (u.effective_group_id && u.effective_group_id > 0) {
    return { name: `${groupName(u.effective_group_id)} (套餐继承)`, type: 'success', custom: false }
  }
  return { name: '无', type: 'info', custom: false }
}

// 「未分配权限组」是告警态而不是健康态：绿色只留给「已继承套餐权限组」
function userGroupBadgeColor(u: any): 'blue' | 'green' | 'grey' {
  const d = userGroupDisplay(u)
  if (d.custom) return 'blue'
  return d.type === 'success' ? 'green' : 'grey'
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
    ElMessage.error(errMsg(e, '调整余额失败'))
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

function onFilterChange() {
  page.value = 1
  load()
}

function resetFilters() {
  window.clearTimeout(searchTimer)
  keyword.value = ''
  statusFilter.value = ''
  planFilter.value = ''
  groupFilter.value = ''
  roleFilter.value = ''
  page.value = 1
  load()
}

async function load() {
  loading.value = true
  try {
    const { data } = await getUsers(page.value, size.value, keyword.value.trim(), {
      status: statusFilter.value,
      plan_id: planFilter.value,
      group_id: groupFilter.value,
      role: roleFilter.value,
    })
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

const RING_RADIUS = 7.5
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS

function usagePercent(u: any) {
  if (!u.total_bytes) return 0
  return Math.min(100, Math.round((u.used_bytes / u.total_bytes) * 100))
}

function ringDashOffset(u: any) {
  if (!u.total_bytes || u.used_bytes <= 0) return RING_CIRCUMFERENCE
  const ratio = Math.min(1, u.used_bytes / u.total_bytes)
  return RING_CIRCUMFERENCE * (1 - ratio)
}

// 时间展示统一走展示时区：后端存 UTC，直接截断字符串会把 UTC 墙钟当本地时间显示
function fmtTime(t: string | null) {
  return formatDateTime(t, '—')
}

// ---- 时间快捷设定 ----
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
  email: '',
  role: 'user' as 'admin' | 'user',
  plan_id: 0,
  permission_group_id: 0,
  expire_at: '',
  password: '',
  remark: '',
  auto_renew_expire: false,
  auto_renew_exhaust: false,
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
  userEditForm.email = row.email || ''
  userEditForm.role = row.role || 'user'
  userEditForm.plan_id = row.plan_id || 0
  userEditForm.permission_group_id = row.permission_group_id || 0
  userEditForm.expire_at = row.expire_at || ''
  userEditForm.password = ''
  userEditForm.remark = row.remark || ''
  userEditForm.auto_renew_expire = !!row.auto_renew_expire
  userEditForm.auto_renew_exhaust = !!row.auto_renew_exhaust
  detailOpen.value = true
}

async function saveUserConfig() {
  if (!current.value) return
  const trimmedEmail = userEditForm.email.trim()
  if (!trimmedEmail) {
    ElMessage.warning('用户邮箱不能为空')
    return
  }
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  if (!emailRegex.test(trimmedEmail)) {
    ElMessage.warning('请输入有效的邮箱地址')
    return
  }
  userSaving.value = true
  try {
    const payload = {
      email: trimmedEmail,
      role: userEditForm.role,
      plan_id: userEditForm.plan_id,
      permission_group_id: userEditForm.permission_group_id,
      expire_at: userEditForm.expire_at || null,
      password: userEditForm.password.trim() || undefined,
      remark: userEditForm.remark,
      auto_renew_expire: userEditForm.auto_renew_expire,
      auto_renew_exhaust: userEditForm.auto_renew_exhaust,
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
  expire_at: '',
  remark: '',
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
      expire_at: newUserForm.expire_at || undefined,
      remark: newUserForm.remark.trim() || undefined,
    }
    const { data } = await createUser(payload)
    if (data.code === 0) {
      newUserResult.value = { uuid: data.data.uuid, email: data.data.email }
      newUserForm.email = ''
      newUserForm.password = ''
      newUserForm.plan_id = 0
      newUserForm.permission_group_id = 0
      newUserForm.expire_at = ''
      newUserForm.remark = ''
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
      `确认重置「${row.email}」的流量周期？重置后其已用流量清零并重新计算限额。`,
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
      if (current.value && current.value.id === row.id) {
        current.value.used_bytes = 0
        current.value.up_bytes = 0
        current.value.down_bytes = 0
      }
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
    await ElMessageBox.confirm(`确定删除用户 ${row.email}？该操作不可逆。`, '删除确认', {
      type: 'error',
      confirmButtonText: '删除',
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

// 详情弹窗的底部动作：先关弹窗再执行，避免确认框被弹窗遮罩挡住
function toggleFromDetail() {
  if (!current.value) return
  detailOpen.value = false
  doToggle(current.value)
}

function deleteFromDetail() {
  if (!current.value) return
  detailOpen.value = false
  doDelete(current.value)
}

// ---- 订阅信息查看/重置（2026-09-04：管理员轮换用户订阅凭据） ----
const siteStore = useSiteStore()
// ---- 自我守卫（2026-09-04）：管理员不能封禁自己，前端直接禁用封禁触点 ----
const authStore = useAuthStore()
const currentUserId = computed(() => authStore.user?.id ?? 0)
function isSelf(row: { id: number }) {
  return row.id === currentUserId.value
}
const subInfoOpen = ref(false)
const subInfoRow = ref<AdminUser | null>(null)
const subInfoToken = ref('')
const subInfoLoading = ref(false)
const subInfoResetting = ref(false)

const subInfoUrl = computed(() =>
  subInfoToken.value ? buildSubscribeUrl(subInfoToken.value, siteStore.subscribeUrl, siteStore.subscribePath) : '',
)

async function openSubscribeInfo(row: AdminUser) {
  subInfoRow.value = row
  subInfoToken.value = ''
  subInfoOpen.value = true
  subInfoLoading.value = true
  try {
    const { data } = await getUserSubscribeToken(row.id)
    if (data.code === 0) {
      subInfoToken.value = data.data.subscribe_token
    } else {
      ElMessage.error(data.message)
      subInfoOpen.value = false
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '获取订阅信息失败'))
    subInfoOpen.value = false
  } finally {
    subInfoLoading.value = false
  }
}

async function doResetSubscribeToken() {
  const row = subInfoRow.value
  if (!row) return
  try {
    await ElMessageBox.confirm(
      '重置后该用户的原订阅地址及连接密钥（UUID）将立即失效，在线服务器将实时断开旧凭据连接。确定重置？',
      '重置订阅地址',
      { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  subInfoResetting.value = true
  try {
    const { data } = await resetUserSubscribeToken(row.id)
    if (data.code === 0) {
      subInfoToken.value = data.data.subscribe_token
      if (data.data.uuid) {
        row.uuid = data.data.uuid
      }
      ElMessage.success('订阅地址及连接密钥已重置')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置订阅地址失败'))
  } finally {
    subInfoResetting.value = false
  }
}

// ---- 批量操作与 CSV 导出（显式进入多选模式） ----
const selectMode = ref(false)
const selected = ref<AdminUser[]>([])
const batchBusy = ref(false)

function enterSelectMode() {
  selectMode.value = true
}

function exitSelectMode() {
  selectMode.value = false
  selected.value = []
}

function csvCell(v: any) {
  const s = String(v ?? '')
  return `"${s.split('"').join('""')}"`
}

function exportCSV() {
  const rows = selectMode.value ? selected.value : list.value
  if (!rows.length) {
    ElMessage.warning('没有可导出的用户')
    return
  }
  const header = ['ID', '邮箱', 'UUID', '角色', '状态', '套餐', '权限组', '余额(分)', '已用流量', '总流量', '到期时间', '注册时间', '备注']
  const lines = rows.map((u: any) =>
    [
      u.id,
      u.email,
      u.uuid,
      u.role,
      u.status === 1 ? '正常' : '禁用',
      planName(u.plan_id),
      userGroupDisplay(u).name,
      u.balance_cents || 0,
      u.used_bytes || 0,
      u.total_bytes || 0,
      formatDateTime(u.expire_at, ''),
      formatDateTime(u.created_at, ''),
      u.remark || '',
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
  let rows = selected.value.filter((u) => u.status !== targetStatus)
  // 自我守卫：批量封禁自动剔除当前登录账号（后端同样拒绝，前端提前跳过并提示）
  const skippedSelf = targetStatus === 0 ? rows.filter((u) => isSelf(u)).length : 0
  rows = rows.filter((u) => !isSelf(u))
  if (!rows.length) {
    ElMessage.warning(skippedSelf > 0 ? '所选用户中仅剩当前登录账号，不能封禁自己' : targetStatus === 1 ? '所选用户均已启用' : '所选用户均已禁用')
    return
  }
  try {
    await ElMessageBox.confirm(
      targetStatus === 1
        ? `确认启用所选 ${rows.length} 位用户？`
        : `确认封禁所选 ${rows.length} 位用户？封禁后其会话与订阅立即失效。${skippedSelf > 0 ? '（当前登录账号已自动跳过）' : ''}`,
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
    if (ok > 0) {
      selected.value = []
      selectMode.value = false
      load()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '批量操作失败'))
  } finally {
    batchBusy.value = false
  }
}

// 表格多选辅助
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

const tableColumns = computed<TableColumn[]>(() => {
  const cols: TableColumn[] = [
    { id: 'id', header: '编号' },
    { id: 'user', header: '用户' },
    { id: 'status', header: '状态' },
    { id: 'plan', header: '套餐' },
    { id: 'group', header: '权限组' },
    { id: 'traffic', header: '流量' },
    { id: 'expire_at', header: '到期时间' },
    { id: 'balance', header: '余额' },
    { id: 'actions', header: '操作', align: 'end' },
  ]
  if (selectMode.value) {
    cols.unshift({ id: 'select', width: 42, align: 'center' })
  }
  return cols
})
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsInput
        v-model="keyword"
        placeholder="搜索邮箱 / UUID / 备注"
        clearable
        prefix-icon="search"
        class="filter-search"
        @keyup.enter="onSearch"
        @clear="onSearch"
        @input="onSearchInput"
      />
      <CsSelect
        v-model="statusFilter"
        :options="statusFilterOptions"
        placeholder="全部状态"
        aria-label="按状态筛选"
        class="filter-select filter-select--sm"
        @change="onFilterChange"
      />
      <CsSelect
        v-model="planFilter"
        :options="planFilterOptions"
        placeholder="全部套餐"
        aria-label="按套餐筛选"
        class="filter-select filter-select--md"
        @change="onFilterChange"
      />
      <CsSelect
        v-model="groupFilter"
        :options="groupFilterOptions"
        placeholder="全部权限组"
        aria-label="按权限组筛选"
        class="filter-select filter-select--md"
        @change="onFilterChange"
      />
      <CsSelect
        v-model="roleFilter"
        :options="roleFilterOptions"
        placeholder="全部角色"
        aria-label="按角色筛选"
        class="filter-select filter-select--sm"
        @change="onFilterChange"
      />
      <CsButton v-if="hasActiveFilters" @click="resetFilters">重置</CsButton>
      <template #actions>
        <template v-if="!selectMode">
          <CsButton icon-name="check" @click="enterSelectMode">批量操作</CsButton>
          <CsButton icon-name="download" @click="exportCSV">导出 CSV</CsButton>
          <CsButton variant="primary" icon-name="add-plus" @click="newUserOpen = true">新增用户</CsButton>
        </template>
        <template v-else>
          <span class="select-count">已选 {{ selected.length }} 项</span>
          <CsButton :loading="batchBusy" :disabled="selected.length === 0" @click="batchToggle(0)">批量封禁</CsButton>
          <CsButton :loading="batchBusy" :disabled="selected.length === 0" @click="batchToggle(1)">批量启用</CsButton>
          <CsButton icon-name="download" :disabled="selected.length === 0" @click="exportCSV">导出所选</CsButton>
          <CsButton @click="exitSelectMode">退出多选</CsButton>
        </template>
      </template>
    </CsToolbar>

    <CsContainer header="用户列表">
      <CsTable
        compact
        :wrap-lines="false"
        :items="list"
        :columns="tableColumns"
        :loading="loading"
        loading-text="加载中…"
        track-by="id"
        :empty-text="hasActiveFilters ? '没有符合筛选条件的用户' : '暂无用户'"
      >
        <template #header-select>
          <CsCheckbox
            :model-value="isAllSelected"
            :indeterminate="isIndeterminate"
            aria-label="全选当前页"
            @change="(v: boolean) => toggleSelectAll(!!v)"
          />
        </template>

        <template #cell-select="{ row }">
          <CsCheckbox
            :model-value="isRowSelected(row)"
            :aria-label="`选择 ${row.email}`"
            @change="(val: boolean) => toggleSelectRow(row, !!val)"
          />
        </template>

        <template #cell-id="{ row }">
          <span class="user-id">#{{ row.id }}</span>
        </template>

        <template #cell-user="{ row }">
          <div class="user-cell">
            <div class="user-cell-main">
              <button
                type="button"
                class="user-email-btn"
                :class="{ 'is-muted': row.status !== 1 }"
                title="点击查看与编辑详情"
                @click="openDetail(row)"
              >
                {{ row.email }}
              </button>
              <CsBadge v-if="row.role === 'admin'" class="status-badge" color="severity-medium">管理员</CsBadge>
            </div>
            <div v-if="row.remark" class="user-remark" :title="row.remark">备注：{{ row.remark }}</div>
          </div>
        </template>

        <template #cell-status="{ row }">
          <CsBadge class="status-badge" :color="row.status === 1 ? 'green' : 'red'">
            {{ row.status === 1 ? '正常' : '已封禁' }}
          </CsBadge>
        </template>

        <template #cell-plan="{ row }">
          <span :class="{ 'cell-muted': !row.plan_id || row.status !== 1 }">
            {{ planName(row.plan_id) }}
          </span>
        </template>

        <template #cell-group="{ row }">
          <CsBadge class="status-badge" :color="userGroupBadgeColor(row)">
            {{ userGroupDisplay(row).name }}
          </CsBadge>
        </template>

        <template #cell-traffic="{ row }">
          <div
            class="traffic-cell"
            :title="row.total_bytes ? `已用 ${usagePercent(row)}%` : '不限流量'"
          >
            <svg
              v-if="row.total_bytes"
              class="traffic-ring"
              :class="{ 'is-danger': usagePercent(row) > 90 }"
              viewBox="0 0 20 20"
              aria-hidden="true"
            >
              <circle class="traffic-ring-track" cx="10" cy="10" r="7.5" />
              <circle
                v-if="row.used_bytes > 0"
                class="traffic-ring-fill"
                cx="10"
                cy="10"
                r="7.5"
                transform="rotate(-90 10 10)"
                :stroke-dasharray="RING_CIRCUMFERENCE"
                :stroke-dashoffset="ringDashOffset(row)"
              />
            </svg>
            <span class="traffic-text cell-mono" :class="{ 'cell-muted': row.status !== 1 }">
              <strong>{{ formatBytes(row.used_bytes) }}</strong> / {{ row.total_bytes ? formatBytes(row.total_bytes) : '不限' }}
            </span>
          </div>
        </template>

        <template #cell-expire_at="{ row }">
          <span v-if="row.expire_at" class="cell-mono" :class="{ 'cell-muted': row.status !== 1 }">
            {{ fmtTime(row.expire_at) }}
          </span>
          <span v-else class="cell-muted">长期有效</span>
        </template>

        <template #cell-balance="{ row }">
          <span class="cell-mono balance-cell" :class="{ 'cell-muted': row.status !== 1 }">
            {{ formatMoney((row as any).balance_cents || 0) }}
          </span>
        </template>

        <template #cell-actions="{ row }">
          <CsButton :wrap-text="false" @click="openDetail(row)">编辑</CsButton>
        </template>
      </CsTable>

      <template #footer>
        <CsPaginationBar v-if="total > 0">
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[10, 20, 50]"
            :layout="isMobile ? 'prev, pager, next' : 'total, sizes, prev, pager, next'"
            @current-change="load"
            @size-change="() => { page = 1; load() }"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <!-- 用户详情与权限配置弹窗 -->
    <CsModal v-model:visible="detailOpen" :header="`编辑用户：${current?.email ?? ''}`" size="large" scrollable :close-on-click-overlay="false">
      <template v-if="current">
        <div class="user-summary">
          <div class="sum-item">
            <span class="sum-label">账户余额</span>
            <span class="sum-value sum-inline">
              <span class="cs-text-mono balance-text">{{ formatMoney((current as any).balance_cents || 0) }}</span>
              <CsButton variant="inline-link" @click="openAdjust(current)">调整余额</CsButton>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">套餐用量</span>
            <span class="sum-value sum-inline">
              <span class="cs-text-mono sum-truncate">
                {{ formatBytes(current.used_bytes) }} / {{ current.total_bytes ? formatBytes(current.total_bytes) : '不限' }}
              </span>
              <CsButton variant="inline-link" @click="doResetTraffic(current)">重置流量</CsButton>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">UUID</span>
            <span class="sum-value sum-inline">
              <code class="cs-text-mono sum-truncate" :title="current.uuid">{{ current.uuid || '—' }}</code>
              <CsButton
                v-if="current.uuid"
                variant="inline-icon"
                icon-name="copy"
                aria-label="复制 UUID"
                @click="copyText(current.uuid, 'UUID')"
              />
              <CsButton variant="inline-link" @click="openSubscribeInfo(current)">订阅地址</CsButton>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">账号概况</span>
            <span class="sum-value sum-inline">
              <span class="sum-id">#{{ current.id }}</span>
              <span class="cell-mono">{{ fmtTime(current.created_at) }}</span>
            </span>
          </div>
        </div>

        <CsForm label-position="top">
          <div class="edit-sections">
            <div class="edit-section">
            <div class="edit-section-title">账号信息</div>
            <div class="form-grid-2">
              <CsFormItem>
                <template #label>
                  登录邮箱
                  <TipIcon
                    type="warn"
                    content="邮箱即登录账号。修改后该用户所有会话将被登出，需用新邮箱重新登录。"
                  />
                </template>
                <CsInput v-model="userEditForm.email" placeholder="user@example.com" clearable />
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  用户身份与角色
                  <TipIcon content="管理员拥有登录管理后台与运维配置全权（系统必须至少保留 1 名激活状态管理员）。" />
                </template>
                <CsSegmentedControl v-model="userEditForm.role" :options="roleOptions" />
              </CsFormItem>

              <CsFormItem label="重置密码（留空不修改）">
                <CsInput v-model="userEditForm.password" type="password" show-password placeholder="输入新密码，留空保持原密码" />
              </CsFormItem>

              <CsFormItem label="备注">
                <CsInput
                  v-model="userEditForm.remark"
                  :maxlength="255"
                  clearable
                  placeholder="仅管理员可见，如渠道来源、联系信息等"
                />
              </CsFormItem>
            </div>
          </div>

          <div class="edit-section">
            <div class="edit-section-title">
              套餐与权限
              <TipIcon content="用户节点权限由「生效权限组」决定（优先使用管理员指定的独立权限组，未指定时继承当前有效套餐的权限组；未分配权限组的用户无法连接任何节点）。" />
            </div>

            <div class="form-grid-2">
              <CsFormItem label="绑定套餐">
                <CsSelect v-model="userEditForm.plan_id" style="width: 100%" placeholder="选择套餐" :options="planOptions" />
              </CsFormItem>

              <CsFormItem label="所属权限组（覆盖套餐默认）">
                <CsSelect
                  v-model="userEditForm.permission_group_id"
                  style="width: 100%"
                  :options="groupOptions"
                />
              </CsFormItem>
            </div>

            <CsFormItem label="到期时间">
              <CsDatePicker
                v-model="userEditForm.expire_at"
                type="datetime"
                placeholder="留空表示永不过期"
                value-format="YYYY-MM-DDTHH:mm:ssZ"
                style="width: 100%"
              />
              <div class="quick-expire-bar">
                <span class="quick-label">快捷设定:</span>
                <CsButton @click="addQuickExpire(30)">+1个月</CsButton>
                <CsButton @click="addQuickExpire(90)">+1季度</CsButton>
                <CsButton @click="addQuickExpire(180)">+半年</CsButton>
                <CsButton @click="addQuickExpire(365)">+1年</CsButton>
                <CsButton @click="clearExpire">永久/清空</CsButton>
              </div>
            </CsFormItem>

            <CsFormItem label="自动续费">
              <div class="autorenew-admin-grid">
                <div class="autorenew-admin-card">
                  <div>
                    <div class="autorenew-title">到期自动续费</div>
                    <div class="autorenew-desc">到期时用余额自动续费，失败则取消套餐</div>
                  </div>
                  <CsToggle v-model="userEditForm.auto_renew_expire" />
                </div>
                <div class="autorenew-admin-card">
                  <div>
                    <div class="autorenew-title">流量耗尽自动续费</div>
                    <div class="autorenew-desc">流量用尽后用余额续购并重置流量</div>
                  </div>
                  <CsToggle v-model="userEditForm.auto_renew_exhaust" />
                </div>
              </div>
            </CsFormItem>
          </div>
          </div>
        </CsForm>
      </template>

      <template #footer>
        <div class="detail-footer">
          <div class="detail-footer-left">
            <CsButton :disabled="current ? isSelf(current) : true" @click="toggleFromDetail">
              {{ current?.status === 1 ? '封禁该用户' : '解封该用户' }}
            </CsButton>
            <CsButton v-if="current && current.role !== 'admin'" variant="danger-outline" @click="deleteFromDetail">删除用户</CsButton>
          </div>
          <div class="detail-footer-right">
            <CsButton @click="detailOpen = false">取消</CsButton>
            <CsButton variant="primary" :loading="userSaving" @click="saveUserConfig">保存用户配置</CsButton>
          </div>
        </div>
      </template>
    </CsModal>

    <!-- 手动创建用户弹窗 -->
    <CsModal v-model:visible="newUserOpen" header="新建用户" size="medium" @close="closeNewUser" :close-on-click-overlay="false">
      <template v-if="!newUserResult">
        <CsForm label-position="top">
          <CsFormItem label="邮箱">
            <CsInput v-model="newUserForm.email" placeholder="user@example.com" />
          </CsFormItem>
          <CsFormItem label="密码">
            <CsInput v-model="newUserForm.password" type="password" show-password placeholder="至少 8 位" />
          </CsFormItem>
          <div class="form-grid-2">
            <CsFormItem label="初始套餐（选填）">
              <CsSelect v-model="newUserForm.plan_id" style="width: 100%" placeholder="选择套餐" :options="newUserPlanOptions" />
            </CsFormItem>
            <CsFormItem label="权限组（选填）">
              <CsSelect v-model="newUserForm.permission_group_id" style="width: 100%" placeholder="跟随套餐" :options="newUserGroupOptions" />
            </CsFormItem>
          </div>
          <CsFormItem label="到期时间（选填）">
            <CsDatePicker
              v-model="newUserForm.expire_at"
              type="datetime"
              placeholder="留空表示永不过期"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              style="width: 100%"
            />
            <div class="quick-expire-bar">
              <span class="quick-label">快捷设定:</span>
              <CsButton @click="addNewUserQuickExpire(30)">+1个月</CsButton>
              <CsButton @click="addNewUserQuickExpire(90)">+1季度</CsButton>
              <CsButton @click="addNewUserQuickExpire(180)">+半年</CsButton>
              <CsButton @click="addNewUserQuickExpire(365)">+1年</CsButton>
              <CsButton @click="clearNewUserExpire">清空</CsButton>
            </div>
          </CsFormItem>
          <CsFormItem label="备注（选填）">
            <CsInput
              v-model="newUserForm.remark"
              type="textarea"
              :rows="2"
              :maxlength="255"
              show-word-limit
              placeholder="仅管理员可见，如渠道来源、联系信息等"
            />
          </CsFormItem>
        </CsForm>
      </template>
      <template v-else>
        <CsAlert type="success" header="用户已创建" style="margin-bottom: 12px" />
        <div class="secret-box">
          <div class="secret-row">
            <span class="k">邮箱</span>
            <code>{{ newUserResult.email }}</code>
          </div>
          <div class="secret-row">
            <span class="k">UUID</span>
            <code>{{ newUserResult.uuid }}</code>
            <CsButton variant="inline-icon" icon-name="copy" aria-label="复制 UUID" @click="copyText(newUserResult.uuid, 'UUID')" />
          </div>
          <p class="cs-text-secondary" style="margin: 0; font-size: 12px">UUID 已生成，用户购买套餐或由管理员分配权限组后即可生效使用节点。</p>
        </div>
      </template>
      <template #footer>
        <template v-if="!newUserResult">
          <CsButton @click="newUserOpen = false">取消</CsButton>
          <CsButton variant="primary" :loading="newUserCreating" @click="submitNewUser">创建</CsButton>
        </template>
        <template v-else>
          <CsButton variant="primary" @click="closeNewUser">完成</CsButton>
        </template>
      </template>
    </CsModal>

    <!-- 独立余额调账弹窗 -->
    <CsModal v-model:visible="adjustOpen" header="调整用户余额" size="small" :close-on-click-overlay="false">
      <CsForm label-position="top">
        <div class="adjust-header">
          <div class="user-meta">
            <span class="k">用户:</span>
            <span class="v" style="font-weight: 600">{{ adjustUser?.email }}</span>
          </div>
          <div class="user-meta">
            <span class="k">当前余额:</span>
            <span class="v cs-text-mono" style="font-weight: 700; color: var(--color-text-link-default-qabnuk)">
              {{ formatMoney((adjustUser as any)?.balance_cents || 0) }}
            </span>
          </div>
        </div>
        <CsFormItem label="变动金额（元，支持负数扣款）" style="margin-top: 14px">
          <CsInputNumber
            v-model="adjustForm.delta_yuan"
            :controls="false"
            :precision="2"
            :step="10"
            placeholder="正数充值，负数扣减"
            style="width: 100%"
          />
        </CsFormItem>
        <CsFormItem label="调整备注（选填）">
          <CsInput v-model="adjustForm.remark" placeholder="如：活动赠送、售后退款、误充扣除" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="adjustOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="adjusting" @click="submitAdjust">确认调整</CsButton>
      </template>
    </CsModal>

    <!-- 订阅信息查看/重置弹窗 -->
    <CsModal v-model:visible="subInfoOpen" header="订阅信息" size="medium">
      <CsLoading :loading="subInfoLoading" class="sub-info-body">
        <div class="user-meta" style="margin-bottom: 12px">
          <span class="k">用户:</span>
          <span class="v" style="font-weight: 600">{{ subInfoRow?.email }}</span>
        </div>
        <!-- label 置顶独立成行：避免长 label 横向占宽挤压输入框（桌面），并消除窄屏下的行内溢出（移动） -->
        <CsForm label-position="top">
          <CsFormItem label="订阅地址（发给用户，客户端添加订阅用）">
            <div class="sub-url-row">
              <CsInput :model-value="subInfoUrl" readonly class="cs-text-mono" />
              <CsButton :disabled="!subInfoUrl" icon-name="copy" @click="copyText(subInfoUrl, '订阅地址')">复制</CsButton>
            </div>
          </CsFormItem>
        </CsForm>
      </CsLoading>
      <template #footer>
        <CsButton @click="subInfoOpen = false">关闭</CsButton>
        <CsButton :loading="subInfoResetting" :disabled="subInfoLoading" @click="doResetSubscribeToken">
          重置订阅地址
        </CsButton>
        <TipIcon
          type="warn"
          content="重置后该用户的原订阅地址及连接密钥（UUID）将立即失效，在线服务器将实时断开旧凭据连接，用户需重新同步订阅。"
        />
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
.user-summary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 24px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 14px;
}

.sum-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  min-width: 0;
  font-size: 13px;
}

.sum-label {
  flex: none;
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb);
}

.sum-value {
  min-width: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-body-default-b2us04);
  text-align: end;

  code {
    font-size: 12px;
  }
}

/* 值里带操作按钮时横向排布；按钮与固定标签不收缩，需要截断的文本使用 .sum-truncate */
.sum-inline {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;

  > :deep([class*='awsui_button_']) {
    flex: none;
  }
}

.sum-truncate {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sum-id {
  flex: none;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb);
}

.balance-text {
  font-weight: 700;
  color: #059669;
}

.edit-sections {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.edit-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  background: var(--color-background-container-content-mjfil9, transparent);

  :deep(.cs-form-item) {
    margin-block-end: 0;
  }
}

.edit-section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04);
}

.form-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
}

.detail-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  /* 占满 footer 宽度，左侧危险动作与右侧主操作才能分列两端 */
  width: 100%;
}

.detail-footer-left,
.detail-footer-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.secret-box {
  background: var(--color-background-cell-shaded-twxsbz);
  border-radius: var(--border-radius-input-k1q34z);
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
  color: var(--color-text-body-secondary-66ygbb);
}
.secret-row code {
  font-size: 12px;
}
.adjust-header {
  background: var(--color-background-cell-shaded-twxsbz);
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

.sub-info-body .user-meta {
  display: flex;
  gap: 8px;
  font-size: 13px;
  .k {
    color: var(--color-text-body-secondary-66ygbb);
    flex: none;
  }
  .v {
    flex: 1;
    min-width: 0;
    word-break: break-all; // 长邮箱/长 URL 换行，防止溢出弹窗被裁切
  }
}
.sub-url-row {
  display: flex;
  gap: 8px;
  width: 100%;
  :deep(.cs-input-root) {
    flex: 1;
    min-width: 0; // 弹性收缩，保证复制按钮在窄屏下不被挤出弹窗
  }
}
.autorenew-admin-grid {
  width: 100%;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 12px;
}

.autorenew-admin-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  background: var(--color-background-cell-shaded-twxsbz);
}

.autorenew-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04);
}

.autorenew-desc {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb);
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
  color: var(--color-text-body-secondary-66ygbb);
  margin-right: 2px;
}

/* ================= 用户表格与工具栏 ================= */
.filter-search {
  width: 220px;
}

.filter-select--sm {
  width: 115px;
}

.filter-select--md {
  width: 145px;
}

.select-count {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-inline-end: 4px;
}

.user-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.user-cell-main {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.user-email-btn {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font-family: inherit;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04, #0f141a);
  text-align: start;
  cursor: pointer;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
    text-decoration: underline;
  }

  &.is-muted {
    color: var(--color-text-status-inactive-fqjc97, #656871);
  }
}

.user-id {
  flex: none;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.user-remark {
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.status-badge {
  white-space: nowrap;
}

.traffic-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  vertical-align: middle;
}

.traffic-ring {
  width: 18px;
  height: 18px;
  flex: none;
}

.traffic-ring-track {
  fill: none;
  stroke: var(--color-border-divider-default-diwd4w, #e9ebed);
  stroke-width: 2.5;
}

.traffic-ring-fill {
  fill: none;
  stroke: var(--color-text-link-default-qabnuk, #006ce0);
  stroke-width: 2.5;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.2s ease;
}

.traffic-ring.is-danger .traffic-ring-fill {
  stroke: var(--color-text-status-error-ri38gt, #d91515);
}

.traffic-text {
  font-variant-numeric: tabular-nums;
}

.balance-cell {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

@media (max-width: $bp-tablet-max) {
  .cs-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-xs-m9b7jf, 8px);
  }

  :deep(.cs-toolbar-main) {
    inline-size: 100%;

    .filter-search {
      inline-size: 100% !important;
    }

    .filter-select {
      flex: 1 1 calc(50% - 4px);
      min-inline-size: 120px;
      inline-size: auto;
    }
  }

  :deep(.cs-toolbar-actions) {
    display: flex;
    flex-wrap: wrap;
    inline-size: 100%;
    gap: 6px;
  }

  .user-summary,
  .form-grid-2,
  .autorenew-admin-grid {
    grid-template-columns: 1fr;
  }
}
</style>