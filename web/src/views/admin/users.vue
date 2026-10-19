<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsButtonDropdown,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsCheckbox,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsLoading,
  CsModal,
  CsPagination,
  CsProgressBar,
  CsSegmentedControl,
  CsSelect,
  CsSpinner,
  CsToggle,
  CsTooltip,
} from '@/components/cloudscape'
import type { SegmentedOption } from '@/components/cloudscape'
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
import { formatBytes } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'
import { buildSubscribeUrl } from '@/config/site'
import { useSiteStore } from '@/stores/site'
import { useAuthStore } from '@/stores/auth'

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
  device_limit: 0,
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
  userEditForm.device_limit = row.device_limit || 0
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
      device_limit: userEditForm.device_limit,
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
  device_limit: 0,
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
      device_limit: newUserForm.device_limit || undefined,
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
      newUserForm.device_limit = 0
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
    await ElMessageBox.confirm(`确定删除用户 ${row.username}？该操作不可逆。`, '删除确认', {
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
      ElMessage.success('订阅地址及连接密钥已重置，已向在线服务器同步')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置订阅地址失败'))
  } finally {
    subInfoResetting.value = false
  }
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
  const header = ['ID', '用户名', '邮箱', 'UUID', '角色', '状态', '套餐', '权限组', '设备限制', '余额(分)', '已用流量', '总流量', '到期时间', '注册时间', '备注']
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

function handleCardAction(cmd: string | number, row: AdminUser) {
  switch (cmd) {
    case 'adjust':
      openAdjust(row)
      break
    case 'subscribe':
      openSubscribeInfo(row)
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

// CsButtonDropdown 的条目是数据驱动，禁用态与文案要按行走
function cardMenuItems(row: AdminUser) {
  return [
    { id: 'subscribe', text: '订阅信息', iconName: 'external' },
    { id: 'reset_traffic', text: '重置流量', iconName: 'refresh' },
    {
      id: 'toggle',
      text: row.status === 1 ? '封禁用户' : '解封用户',
      iconName: row.status === 1 ? 'lock-private' : 'unlocked',
      disabled: isSelf(row),
    },
    { id: 'delete', text: '删除用户', iconName: 'remove', dividerBefore: true },
  ]
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <CsInput
          v-model="keyword"
          placeholder="搜索用户名 / 邮箱 / UUID / 备注"
          clearable
          prefix-icon="search"
          style="width: 280px"
          @keyup.enter="onSearch"
          @clear="onSearch"
          @input="onSearchInput"
        />
      </div>
      <div class="toolbar-actions">
        <CsButton icon-name="download" @click="exportCSV">导出 CSV</CsButton>
        <CsButton :loading="batchBusy" @click="batchToggle(0)">批量封禁</CsButton>
        <CsButton :loading="batchBusy" @click="batchToggle(1)">批量启用</CsButton>
        <CsButton variant="primary" icon-name="add-plus" @click="newUserOpen = true">新增用户</CsButton>
      </div>
    </div>

    <CsContainer header="用户列表">
      <template #actions>
        <div class="user-select-bar">
          <CsCheckbox
            :model-value="isAllSelected"
            :indeterminate="isIndeterminate"
            @change="(v: boolean) => toggleSelectAll(!!v)"
          >
            全选当前页 (已选 {{ selected.length }} / {{ total }})
          </CsCheckbox>
        </div>
      </template>

      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="list.length === 0" class="user-empty-state">
        <CsIcon name="user-profile" size="large" />
        <p style="margin-top: 8px">暂无匹配用户</p>
      </div>

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，正文 14/20，额度作为卡片主指标 -->
      <CsCardGrid v-else>
        <CsCard
          v-for="row in list"
          :key="row.id"
          :class="{ 'user-card--selected': isRowSelected(row), 'user-card--banned': row.status !== 1 }"
        >
          <template #header>
            <div class="user-head">
              <CsCheckbox
                :model-value="isRowSelected(row)"
                :aria-label="`选择 ${row.username}`"
                @change="(val: boolean) => toggleSelectRow(row, !!val)"
              />
              <button type="button" class="user-name" title="点击查看与编辑详情" @click="openDetail(row)">
                {{ row.username }}
              </button>
            </div>
            <div class="user-meta">
              <span class="user-id">#{{ row.id }}</span>
              <CsBadge v-if="row.role === 'admin'" color="severity-medium">管理员</CsBadge>
              <CsBadge :color="row.status === 1 ? 'green' : 'red'">
                {{ row.status === 1 ? '正常' : '已封禁' }}
              </CsBadge>
            </div>
            <div class="user-email">{{ row.email || '—' }}</div>
            <div v-if="row.remark" class="user-remark" :title="row.remark">备注：{{ row.remark }}</div>
          </template>

          <template #actions>
            <CsTooltip
              :content="isSelf(row) ? '不能封禁当前登录的账号' : row.status === 1 ? '点击封禁用户' : '点击解封用户'"
              placement="top"
            >
              <CsToggle
                :model-value="row.status === 1"
                :disabled="isSelf(row)"
                :aria-label="`切换 ${row.username} 的启用状态`"
                @change="doToggle(row)"
              />
            </CsTooltip>
          </template>

          <CsCardFacts>
            <CsCardFact label="套餐">
              <CsBadge color="blue">{{ planName(row.plan_id) }}</CsBadge>
            </CsCardFact>
            <CsCardFact label="所属权限组">
              <CsBadge :color="userGroupDisplay(row).custom ? 'blue' : 'green'">
                {{ userGroupDisplay(row).name }}
              </CsBadge>
            </CsCardFact>
            <CsCardFact label="账户余额" mono>
              <span class="balance-value">¥ {{ (((row as any).balance_cents || 0) / 100).toFixed(2) }}</span>
            </CsCardFact>
            <CsCardFact
              label="设备限制"
              mono
              :value="row.is_custom_device_limit ? `${row.effective_device_limit} 台 (自定义)` : (row.effective_device_limit && row.effective_device_limit > 0) ? `${row.effective_device_limit} 台` : '不限'"
            />
          </CsCardFacts>

          <div class="user-traffic">
            <div class="traffic-head">
              <span>流量使用</span>
              <span class="traffic-value">
                <strong>{{ formatBytes(row.used_bytes) }}</strong> /
                {{ row.total_bytes ? formatBytes(row.total_bytes) : '不限' }}
                <span v-if="row.total_bytes" class="traffic-percent">({{ usagePercent(row) }}%)</span>
              </span>
            </div>
            <CsProgressBar
              v-if="row.total_bytes"
              :value="usagePercent(row)"
              :bar-height="5"
              hide-text
              :color="usagePercent(row) > 90 ? 'var(--color-text-status-error-ri38gt)' : undefined"
              :aria-label="`流量已用 ${usagePercent(row)}%`"
            />
            <div class="traffic-expire">{{ row.expire_at ? fmtTime(row.expire_at) + ' 到期' : '长期有效' }}</div>
          </div>

          <template #footer>
            <CsButton icon-name="settings" @click="openDetail(row)">编辑配置</CsButton>
            <CsButton icon-name="ticket" @click="openAdjust(row)">调整余额</CsButton>
            <CsButtonDropdown
              class="card-more"
              icon-only
              icon-name="ellipsis"
              aria-label="更多操作"
              :items="cardMenuItems(row)"
              @select="(cmd) => handleCardAction(cmd, row)"
            />
          </template>
        </CsCard>
      </CsCardGrid>

      <div class="x-pager">
        <CsPagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          :layout="isMobile ? 'prev, pager, next' : 'total, sizes, prev, pager, next'"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </CsContainer>

    <!-- 用户详情与权限配置弹窗 -->
    <CsModal v-model:visible="detailOpen" :header="`编辑用户 · ${current?.username ?? ''}`" size="large" scrollable>
      <template v-if="current">
        <div class="user-summary">
          <div class="sum-item">
            <span class="sum-label">用户名</span>
            <span class="sum-value">{{ current.username }}</span>
          </div>
          <div class="sum-item">
            <span class="sum-label">邮箱</span>
            <span class="sum-value">{{ current.email || '—' }}</span>
          </div>
          <div class="sum-item">
            <span class="sum-label">UUID</span>
            <span class="sum-value sum-inline">
              <code class="cell-mono" :title="current.uuid">{{ current.uuid || '—' }}</code>
              <CsButton
                v-if="current.uuid"
                variant="inline-icon"
                icon-name="copy"
                aria-label="复制 UUID"
                @click="copyText(current.uuid, 'UUID')"
              />
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">角色</span>
            <span class="sum-value">
              <CsBadge :color="current.role === 'admin' ? 'severity-medium' : 'blue'">
                {{ current.role === 'admin' ? '管理员' : '普通用户' }}
              </CsBadge>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">账户余额</span>
            <span class="sum-value sum-inline">
              <span class="cell-mono balance-text">¥ {{ (((current as any).balance_cents || 0) / 100).toFixed(2) }}</span>
              <CsButton variant="inline-link" @click="openAdjust(current)">调整余额</CsButton>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">套餐用量</span>
            <span class="sum-value sum-inline">
              <span class="cell-mono">
                {{ formatBytes(current.used_bytes) }} / {{ current.total_bytes ? formatBytes(current.total_bytes) : '不限' }}
              </span>
              <CsButton variant="inline-link" @click="doResetTraffic(current)">重置流量</CsButton>
            </span>
          </div>
          <div class="sum-item">
            <span class="sum-label">注册时间</span>
            <span class="sum-value cell-mono">{{ fmtTime(current.created_at) }}</span>
          </div>
        </div>

        <div class="sec-title">
          套餐与权限设置
          <TipIcon content="用户节点权限由「生效权限组」决定（优先使用管理员指定的独立权限组，未指定时继承当前有效套餐的权限组；未分配权限组的用户无法连接任何节点）。" />
        </div>

        <CsForm label-position="top">
          <div class="form-grid-2">
            <CsFormItem>
              <template #label>
                登录邮箱（用户名）
                <TipIcon
                  type="warn"
                  content="修改后用户的系统邮箱与登录用户名将同步更新，该用户所有会话将被登出，需用新邮箱重新登录。"
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

            <CsFormItem label="设备限制（台，0=跟随套餐）">
              <CsInputNumber
                v-model="userEditForm.device_limit"
                :controls="false"
                :min="0"
                :precision="0"
                placeholder="0"
                style="width: 100%"
              />
            </CsFormItem>

            <CsFormItem label="重置密码（留空不修改）">
              <CsInput v-model="userEditForm.password" type="password" show-password placeholder="输入新密码，留空保持原密码" />
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

          <CsFormItem label="备注">
            <CsInput
              v-model="userEditForm.remark"
              type="textarea"
              :rows="2"
              :maxlength="255"
              show-word-limit
              placeholder="仅管理员可见的用户备注，如渠道来源、联系信息等"
            />
          </CsFormItem>

          <CsFormItem label="自动续费">
            <div class="autorenew-admin-box">
              <div class="autorenew-admin-row">
                <div>
                  <div style="font-size: 13px; font-weight: 600">到期自动续费</div>
                  <div class="muted" style="font-size: 11.5px">到期前 1 小时内用余额续购当前套餐</div>
                </div>
                <CsToggle v-model="userEditForm.auto_renew_expire" />
              </div>
              <div class="autorenew-admin-row">
                <div>
                  <div style="font-size: 13px; font-weight: 600">流量耗尽自动续费</div>
                  <div class="muted" style="font-size: 11.5px">流量用尽后用余额续购（重新计时+清零流量）</div>
                </div>
                <CsToggle v-model="userEditForm.auto_renew_exhaust" />
              </div>
            </div>
          </CsFormItem>
        </CsForm>
      </template>

      <template #footer>
        <div class="detail-footer">
          <div class="detail-footer-left">
            <CsButton :disabled="current ? isSelf(current) : true" @click="toggleFromDetail">
              {{ current?.status === 1 ? '封禁该用户' : '解封该用户' }}
            </CsButton>
            <CsButton v-if="current && current.role !== 'admin'" @click="deleteFromDetail">删除用户</CsButton>
          </div>
          <div class="detail-footer-right">
            <CsButton @click="detailOpen = false">取消</CsButton>
            <CsButton variant="primary" :loading="userSaving" @click="saveUserConfig">保存用户配置</CsButton>
          </div>
        </div>
      </template>
    </CsModal>

    <!-- 手动创建用户弹窗 -->
    <CsModal v-model:visible="newUserOpen" header="新建用户" size="medium" @close="closeNewUser">
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
            <CsFormItem label="设备限制（台，0=跟随套餐）">
              <CsInputNumber
                v-model="newUserForm.device_limit"
                :controls="false"
                :min="0"
                :precision="0"
                placeholder="0"
                style="width: 100%"
              />
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
          <p class="muted" style="margin: 0; font-size: 12px">UUID 已生成，用户购买套餐或由管理员分配权限组后即可生效使用节点。</p>
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
    <CsModal v-model:visible="adjustOpen" header="调整用户余额" size="small">
      <CsForm label-position="top">
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
              <CsInput :model-value="subInfoUrl" readonly class="cell-mono" />
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
.user-summary {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 0 24px;
  background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: 8px;
  padding: 14px 16px;
  margin-bottom: 20px;
}

.sum-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  font-size: 13px;
  padding: 5px 0;
  border-bottom: 1px dashed var(--x-border, #e5e7eb);

  &:last-child {
    border-bottom: none;
  }
}

.sum-label {
  flex: none;
  font-size: 12px;
  color: var(--x-text-2, #6b7280);
}

.sum-value {
  min-width: 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--x-text, #1e2333);
  text-align: end;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  code {
    font-size: 12.5px;
  }
}

/* 值里带操作按钮时改为横向排布；按钮不收缩，文本负责截断 */
.sum-inline {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  white-space: normal;

  > code,
  > span:first-child {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  > :deep([class*='awsui_button_']) {
    flex: none;
  }
}

.balance-text {
  font-weight: 700;
  color: #059669;
}

.sec-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text, #1e2333);
}

.form-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
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

@media (max-width: 720px) {
  .form-grid-2 {
    grid-template-columns: 1fr;
  }
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

.sub-info-body .user-meta {
  display: flex;
  gap: 8px;
  font-size: 13px;
  .k {
    color: var(--x-text-3, #909399);
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
.autorenew-admin-box {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--x-border, #e4e7ed);
  border-radius: 8px;

  .autorenew-admin-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
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

/* ================= 用户卡片（官方 ItemCard 口径） ================= */
.user-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-name {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font: inherit;
  color: inherit;
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.user-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.user-id {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.user-email {
  margin-block-start: 4px;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

.user-remark {
  margin-block-start: 4px;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 选中 / 封禁只用官方高亮边框语义表达，不另造一层卡片底色 */
.user-card--selected :deep(.awsui_inner-card_n1fwt_1f9u9_138) {
  border-color: var(--color-border-item-selected-7104rh, #006ce0) !important;
}

.user-card--banned {
  opacity: 0.88;
}

.balance-value {
  color: var(--color-text-status-success-1i2jf8, #00802f);
  font-weight: 700;
}

.user-traffic {
  margin-block-start: 12px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

.traffic-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin-block-end: 6px;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.traffic-value {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-default-b2us04, #0f141a);
  text-align: end;
}

.traffic-percent {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-inline-start: 2px;
}

.traffic-expire {
  margin-block-start: 6px;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

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

@media (max-width: 768px) {
  .x-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;

    .x-toolbar-left {
      width: 100%;
      :deep(.cs-input-root) {
        width: 100% !important;
      }
    }

    .toolbar-actions {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 6px;
      width: 100%;

      :deep([class*='awsui_button_']) {
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
  .user-traffic {
    padding: 8px 10px;
  }
}
</style>