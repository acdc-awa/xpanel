<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CsButton,
  CsContainer,
  CsDatePicker,
  CsInput,
  CsModal,
  CsOption,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsSpinner,
  CsTable,
  CsTabs,
  CsToolbar,
} from '@/components/cloudscape'
import type { TabItem, TableColumn } from '@/components/cloudscape'
import { exportAuditLogs, getAuditLog, getAuditLogs, type AuditLog } from '@/api/admin'
import { getActionMeta, renderAudit, type AuditView } from '@/utils/auditRender'
import { formatDateTime } from '@/utils/timezone'
import { errMsg } from '@/api/http'
import { toast } from '@/components/cloudscape'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(10)

const category = ref('')
const keyword = ref('')
const operatorType = ref('')
const operatorName = ref('')
const targetType = ref('')
const result = ref('')
const startTime = ref<string | null>(null)
const endTime = ref<string | null>(null)

const exporting = ref(false)

// 结束时间按「所选那一秒的末日」提交：日期选择器只到秒，直接 toISOString 会把
// 23:59:59 之后的操作排除在外（看起来像「明明刚发生却搜不到」）。
function endTimeISO(v: string | null) {
  if (!v) return undefined
  return new Date(new Date(v).getTime() + 999).toISOString()
}

function currentQuery() {
  return {
    category: category.value || undefined,
    keyword: keyword.value.trim() || undefined,
    operator_type: operatorType.value || undefined,
    operator_name: operatorName.value || undefined,
    target_type: targetType.value || undefined,
    result: (result.value || undefined) as 'success' | 'fail' | undefined,
    start_time: startTime.value ? new Date(startTime.value).toISOString() : undefined,
    end_time: endTimeISO(endTime.value),
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await getAuditLogs(page.value, size.value, currentQuery())
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '加载审计日志失败'))
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  page.value = 1
  load()
}

function resetFilters() {
  category.value = ''
  keyword.value = ''
  operatorType.value = ''
  operatorName.value = ''
  targetType.value = ''
  result.value = ''
  startTime.value = null
  endTime.value = null
  page.value = 1
  load()
}

// 「刷新」也回到第一页：否则新增日志会让后续页内容整体后移，看起来像数据错乱。
function refresh() {
  page.value = 1
  load()
}

// 按操作人筛选（点卡片上的邮箱即可）
function filterByOperator(name?: string) {
  if (!name) return
  operatorName.value = name
  onFilterChange()
}

async function doExport() {
  exporting.value = true
  try {
    const { data } = await exportAuditLogs(currentQuery())
    const url = URL.createObjectURL(data)
    const a = document.createElement('a')
    a.href = url
    a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
    toast.success('已导出当前筛选结果')
  } catch (e) {
    toast.error(errMsg(e, '导出失败'))
  } finally {
    exporting.value = false
  }
}

onMounted(load)

// 翻译层：每行一次解析（摘要 + 字段表 + 大文本预览 + 分类元信息）
interface RowView {
  id: number
  row: AuditLog
  view: AuditView
  categoryName: string
  categoryColor: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  title: string
}

// 分类展示口径由后端单源给出（category + category_label）；颜色按模块固定。
// 旧数据（无 category 字段）回退到 action 推导的 getActionMeta，保证历史记录不丢角标。
const CATEGORY_META: Record<string, { color: RowView['categoryColor'] }> = {
  servers: { color: 'primary' },
  users: { color: 'success' },
  billing: { color: 'warning' },
  inbounds: { color: 'info' },
  settings: { color: 'danger' },
  auth: { color: 'info' },
  other: { color: 'info' },
}

const rows = computed<RowView[]>(() =>
  list.value.map((row) => {
    // 列表对超长 detail 做过截断（不是合法 JSON）：列表只给提示，完整内容走「查看原文」
    const view = row.detail_truncated
      ? { ...renderAudit(row.action, '', row.target_name ?? ''), summary: '内容较长，已截断；点「查看原文」加载全文' }
      : renderAudit(row.action, row.detail, row.target_name ?? '')
    const meta = getActionMeta(row.action, view.method, row.detail)
    const categoryMeta = row.category ? CATEGORY_META[row.category] : undefined
    return {
      id: row.id,
      row,
      view,
      categoryName: row.category_label || meta.categoryName,
      categoryColor: categoryMeta?.color ?? meta.categoryColor,
      title: meta.title,
    }
  }),
)

const auditColumns: TableColumn[] = [
  { id: 'created_at', header: '时间' },
  { id: 'category', header: '分类' },
  { id: 'action', header: '操作', minWidth: '240px' },
  { id: 'operator', header: '操作人' },
  { id: 'target', header: '目标' },
  { id: 'result', header: '结果' },
  { id: 'ip', header: 'IP' },
  { id: 'actions', header: '操作', width: '1%' },
]

const hasFilters = computed(() =>
  Boolean(
    category.value ||
      keyword.value.trim() ||
      operatorType.value ||
      operatorName.value ||
      targetType.value ||
      result.value ||
      startTime.value ||
      endTime.value,
  ),
)

const emptyText = computed(() => (hasFilters.value ? '无符合条件的审计日志' : '暂无审计日志'))

function hasDistinctSummary(rv: RowView) {
  const s = rv.view.summary?.trim()
  return Boolean(s && s !== '—' && s !== rv.title)
}

function actionTooltip(rv: RowView) {
  const parts = [rv.title]
  if (hasDistinctSummary(rv)) parts.push(rv.view.summary)
  if (rv.row.action) parts.push(`动作码：${rv.row.action}`)
  return parts.join('\n')
}

// 目标类型展示名（与后端 target_type 取值一一对应）
const TARGET_LABELS: Record<string, string> = {
  user: '用户',
  server: '服务器',
  inbound: '入站',
  plan: '套餐',
  order: '订单',
  cert: '证书',
  ca_cert: 'CA 证书',
  permission_group: '权限组',
  access_point: '接入点',
  sub_template: '订阅模板',
  notice: '公告',
  gift_card: '礼品卡',
  invitation: '邀请码',
}
function targetLabel(t?: string) {
  if (!t) return ''
  return TARGET_LABELS[t] ?? t
}

const detailModalOpen = ref(false)
const detailTab = ref('translated')
const detailTabs: TabItem[] = [
  { id: 'translated', label: '翻译视图' },
  { id: 'raw', label: '原文' },
]
const currentView = ref<AuditView | null>(null)
const detailLoading = ref(false)

// 打开详情：列表里的 detail 超过 8 KB 会被后端截断，此时按需拉完整原文再渲染。
async function openDetail(row: AuditLog) {
  detailModalOpen.value = true
  detailTab.value = 'translated'
  currentView.value = renderAudit(row.action, row.detail, row.target_name ?? '')
  if (!row.detail_truncated) return
  detailLoading.value = true
  try {
    const { data } = await getAuditLog(row.id)
    if (data.code === 0) currentView.value = renderAudit(data.data.action, data.data.detail, data.data.target_name ?? '')
  } catch (e) {
    toast.error(errMsg(e, '加载完整日志失败'))
  } finally {
    detailLoading.value = false
  }
}

// 后端时间是 UTC（进程固定 time.Local=UTC），必须按面板「展示时区」渲染，
// 直接字符串截断会让非 UTC 时区的管理员看到 8 小时偏差。
function fmtTime(t: string) {
  return formatDateTime(t, '')
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect v-model="category" placeholder="全部分类" clearable style="width: min(100%, 140px)" @change="onFilterChange">
          <CsOption label="全部分类" value="" />
          <CsOption label="服务器管理" value="servers" />
          <CsOption label="用户管理" value="users" />
          <CsOption label="财务套餐" value="billing" />
          <CsOption label="入站证书" value="inbounds" />
          <CsOption label="系统设置" value="settings" />
          <CsOption label="身份认证" value="auth" />
          <CsOption label="其他" value="other" />
        </CsSelect>
        <CsSelect v-model="operatorType" placeholder="操作人类型" clearable style="width: min(100%, 140px)" @change="onFilterChange">
          <CsOption label="全部类型" value="" />
          <CsOption label="管理员" value="admin" />
          <CsOption label="普通用户" value="user" />
          <CsOption label="系统任务" value="system" />
        </CsSelect>
        <CsSelect v-model="targetType" placeholder="目标类型" clearable style="width: min(100%, 140px)" @change="onFilterChange">
          <CsOption label="全部目标" value="" />
          <CsOption label="用户" value="user" />
          <CsOption label="服务器" value="server" />
          <CsOption label="入站" value="inbound" />
          <CsOption label="套餐" value="plan" />
          <CsOption label="订单" value="order" />
          <CsOption label="证书" value="cert" />
          <CsOption label="CA 证书" value="ca_cert" />
          <CsOption label="权限组" value="permission_group" />
          <CsOption label="接入点" value="access_point" />
          <CsOption label="订阅模板" value="sub_template" />
          <CsOption label="公告" value="notice" />
          <CsOption label="礼品卡" value="gift_card" />
          <CsOption label="邀请码" value="invitation" />
        </CsSelect>
        <CsSelect v-model="result" placeholder="结果" clearable style="width: min(100%, 120px)" @change="onFilterChange">
          <CsOption label="全部结果" value="" />
          <CsOption label="成功" value="success" />
          <CsOption label="失败" value="fail" />
        </CsSelect>
        <div class="range-filter">
          <CsDatePicker
            v-model="startTime"
            type="datetime"
            placeholder="开始时间"
            style="width: min(100%, 165px)"
            @change="onFilterChange"
          />
          <span class="range-sep">至</span>
          <CsDatePicker
            v-model="endTime"
            type="datetime"
            placeholder="结束时间"
            style="width: min(100%, 165px)"
            @change="onFilterChange"
          />
        </div>
        <CsInput
          v-model="keyword"
          placeholder="搜索操作内容 / 路径 / IP / 操作人 / 目标"
          clearable
          prefix-icon="search"
          style="width: min(100%, 260px)"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        />
        <CsButton v-if="hasFilters" @click="resetFilters">重置</CsButton>
        <CsButton icon-name="refresh" @click="refresh">刷新</CsButton>
        <CsButton icon-name="download" :loading="exporting" @click="doExport">导出</CsButton>
    </CsToolbar>

    <CsContainer header="系统审计日志">
      <CsTable
        compact
        :wrap-lines="false"
        :items="rows"
        :columns="auditColumns"
        :loading="loading"
        loading-text="加载中…"
        track-by="id"
        :empty-text="emptyText"
      >
        <template #cell-created_at="{ row }">
          <span class="cell-mono">{{ fmtTime(row.row.created_at) }}</span>
        </template>
        <template #cell-category="{ row }">
          <span :class="`category-${row.categoryColor}`">{{ row.categoryName }}</span>
        </template>
        <template #cell-action="{ row }">
          <div class="audit-action-cell" :title="actionTooltip(row)">
            <span class="audit-action-title">{{ row.title }}</span>
            <span v-if="hasDistinctSummary(row)" class="audit-action-summary">{{ row.view.summary }}</span>
          </div>
        </template>
        <template #cell-operator="{ row }">
          <div class="audit-operator-cell">
            <span class="audit-operator-role">
              {{ row.row.operator_type === 'admin' ? '管理员' : row.row.operator_type === 'system' ? '系统' : '用户' }}
            </span>
            <button
              v-if="row.row.operator_type !== 'system'"
              type="button"
              class="audit-operator--link"
              :title="row.row.operator_name ? `按操作人 ${row.row.operator_name} 筛选` : undefined"
              @click="filterByOperator(row.row.operator_name)"
            >
              {{ row.row.operator_name || `#${row.row.operator_id}` }}
            </button>
          </div>
        </template>
        <template #cell-target="{ row }">
          <span v-if="row.row.target_type" :title="[targetLabel(row.row.target_type), row.row.target_name, row.row.target_id ? `#${row.row.target_id}` : ''].filter(Boolean).join(' ')">
            {{ targetLabel(row.row.target_type) }}
            <template v-if="row.row.target_name"> {{ row.row.target_name }}</template>
            <span v-if="row.row.target_id" class="cell-mono cell-muted"> #{{ row.row.target_id }}</span>
          </span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-result="{ row }">
          <span v-if="(row.row.status ?? 0) >= 400" class="status-fail">失败 {{ row.row.status }}</span>
          <span v-else-if="(row.row.status ?? 0) > 0" class="status-success">成功</span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-ip="{ row }">
          <span class="cell-mono" :class="{ 'cell-muted': !row.row.ip }" :title="row.row.ip || undefined">
            {{ row.row.ip || '—' }}
          </span>
        </template>
        <template #cell-actions="{ row }">
          <CsButton variant="inline-link" :wrap-text="false" @click="openDetail(row.row)">
            {{ row.row.detail_truncated ? '查看原文' : '详情' }}
          </CsButton>
        </template>
      </CsTable>

      <template #footer>
        <CsPaginationBar v-if="total > 0">
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[10, 20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            simple
            @current-change="load"
            @size-change="() => { page = 1; load() }"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <!-- 详情弹窗：翻译视图 / 原文 -->
    <CsModal v-model:visible="detailModalOpen" header="操作详情" size="large" scrollable>
      <div v-if="detailLoading" class="state-block"><CsSpinner size="large" /></div>
      <CsTabs v-else-if="currentView" v-model="detailTab" :tabs="detailTabs">
        <div v-if="detailTab === 'translated'">
          <div class="audit-modal-summary">{{ currentView.summary || '—' }}</div>
          <div v-if="currentView.fields.length" class="modal-fields">
            <div v-for="(f, i) in currentView.fields" :key="i" class="modal-field-row">
              <span class="mf-label">{{ f.label }}</span>
              <span class="mf-value" :class="{ 'cell-mono': f.mono }">{{ f.value }}</span>
            </div>
          </div>
          <div v-for="(t, i) in currentView.texts" :key="i" class="modal-text">
            <div class="mt-head">
              {{ t.title }}<span class="mt-meta">{{ t.meta }}</span>
            </div>
            <pre class="audit-pre">{{ t.preview }}</pre>
          </div>
          <div v-if="!currentView.fields.length && !currentView.texts.length" style="color: var(--color-text-body-secondary-66ygbb); font-size: 13px; padding: 8px 0">
            无结构化字段，可切到「原文」查看。
          </div>
        </div>
        <div v-else>
          <pre class="audit-pre">{{ currentView.raw }}</pre>
        </div>
      </CsTabs>
      <template #footer>
        <CsButton @click="detailModalOpen = false">关闭</CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">

.range-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  max-width: 100%;

  @media (max-width: 767px) {
    width: 100%;

    > * {
      flex: 1 1 140px;
      min-width: 0;
    }

    .range-sep {
      flex: 0 0 auto;
    }
  }
}

.range-sep {
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb);
}

.state-block {
  padding: 48px 0;
  text-align: center;
}

/* ================= 审计日志列表单元格 ================= */
.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.category-primary {
  color: var(--color-text-status-info-su6k9n, #0972d3);
}

.category-success {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.category-warning {
  color: var(--color-text-status-warning-fabq6h, #855900);
}

.category-danger {
  color: var(--color-text-status-error-ri38gt, #d91515);
}

.category-info {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.status-success {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.status-fail {
  color: var(--color-text-status-error-ri38gt, #d91515);
  font-weight: 600;
}

.audit-action-cell {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-inline-size: 420px;
}

.audit-action-title {
  font-weight: 600;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.audit-action-summary {
  margin-inline-start: 8px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.audit-operator-cell {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  max-inline-size: 100%;
  overflow: hidden;
}

.audit-operator-role {
  flex: 0 0 auto;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

/* 操作人可点：按该邮箱筛选（搜索框搜不到名字的旧问题从此有入口） */
.audit-operator--link {
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  color: var(--color-text-link-default-qabnuk, #0972d3);
  cursor: pointer;
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-operator--link:hover {
  text-decoration: underline;
}


/* 详情弹窗：翻译视图 */
.audit-modal-summary {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04);
  padding: 2px 0 10px;
  word-break: break-all;
}

.modal-fields {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  overflow: hidden;
}

.modal-field-row {
  display: flex;
  gap: 10px;
  padding: 6px 10px;
  font-size: 13px;
  line-height: 1.5;

  &:nth-child(odd) {
    background: var(--color-background-cell-shaded-twxsbz);
  }

  .mf-label {
    flex: 0 0 128px;
    color: var(--color-text-body-secondary-66ygbb);
  }

  .mf-value {
    flex: 1;
    color: var(--color-text-body-secondary-66ygbb);
    word-break: break-all;
  }
}

.modal-text {
  margin-top: 10px;

  .mt-head {
    font-size: 12px;
    color: var(--color-text-body-secondary-66ygbb);
    margin-bottom: 4px;
    display: flex;
    align-items: baseline;
    gap: 8px;

    .mt-meta {
      color: var(--color-text-body-secondary-66ygbb);
      font-size: 12px;
    }
  }
}

.audit-pre {
  background: var(--color-background-cell-shaded-twxsbz);
  padding: 12px 14px;
  border-radius: 8px;
  font-family: var(--font-family-monospace-842lr2);
  font-size: 13px;
  line-height: 1.5;
  color: var(--color-text-body-default-b2us04);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 380px;
  overflow-y: auto;
  margin: 0;
}
</style>
