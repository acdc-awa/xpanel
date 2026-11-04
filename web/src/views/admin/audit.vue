<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardGrid,
  CsContainer,
  CsDatePicker,
  CsEmpty,
  CsInput,
  CsModal,
  CsOption,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsSpinner,
  CsTabs,
  CsToolbar,
} from '@/components/cloudscape'
import type { TabItem } from '@/components/cloudscape'
import { exportAuditLogs, getAuditLog, getAuditLogs, type AuditLog } from '@/api/admin'
import { getActionMeta, renderAudit, type AuditView } from '@/utils/auditRender'
import { errMsg } from '@/api/http'
import { ElMessage } from '@/components/cloudscape'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

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
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载审计日志失败'))
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
    ElMessage.success('已导出当前筛选结果')
  } catch (e) {
    ElMessage.error(errMsg(e, '导出失败'))
  } finally {
    exporting.value = false
  }
}

onMounted(load)

// 翻译层：每行一次解析（摘要 + 字段表 + 大文本预览 + 分类元信息）
interface RowView {
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
    // 列表对超长 detail 做过截断（不是合法 JSON）：卡片只给提示，完整内容走「查看完整原文」
    const view = row.detail_truncated
      ? { ...renderAudit(row.action, '', row.target_name ?? ''), summary: '内容较长，已截断；点「查看完整原文」加载全文' }
      : renderAudit(row.action, row.detail, row.target_name ?? '')
    const meta = getActionMeta(row.action, view.method, row.detail)
    const categoryMeta = row.category ? CATEGORY_META[row.category] : undefined
    return {
      row,
      view,
      categoryName: row.category_label || meta.categoryName,
      categoryColor: categoryMeta?.color ?? meta.categoryColor,
      title: meta.title,
    }
  }),
)

const CATEGORY_BADGE: Record<RowView['categoryColor'], 'blue' | 'green' | 'red' | 'grey'> = {
  primary: 'blue',
  info: 'grey',
  success: 'green',
  warning: 'grey',
  danger: 'red',
}

// 卡片内字段表最多展示条数，超出折叠进弹窗
const CARD_FIELD_LIMIT = 6

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
    ElMessage.error(errMsg(e, '加载完整日志失败'))
  } finally {
    detailLoading.value = false
  }
}

function fmtTime(t: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect v-model="category" placeholder="全部分类" clearable style="width: 140px" @change="onFilterChange">
          <CsOption label="全部分类" value="" />
          <CsOption label="服务器管理" value="servers" />
          <CsOption label="用户管理" value="users" />
          <CsOption label="财务套餐" value="billing" />
          <CsOption label="入站证书" value="inbounds" />
          <CsOption label="系统设置" value="settings" />
          <CsOption label="身份认证" value="auth" />
          <CsOption label="其他" value="other" />
        </CsSelect>
        <CsSelect v-model="operatorType" placeholder="操作人类型" clearable style="width: 140px" @change="onFilterChange">
          <CsOption label="全部类型" value="" />
          <CsOption label="管理员" value="admin" />
          <CsOption label="普通用户" value="user" />
          <CsOption label="系统任务" value="system" />
        </CsSelect>
        <CsSelect v-model="targetType" placeholder="目标类型" clearable style="width: 140px" @change="onFilterChange">
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
        <CsSelect v-model="result" placeholder="结果" clearable style="width: 120px" @change="onFilterChange">
          <CsOption label="全部结果" value="" />
          <CsOption label="成功" value="success" />
          <CsOption label="失败" value="fail" />
        </CsSelect>
        <div class="range-filter">
          <CsDatePicker
            v-model="startTime"
            type="datetimerange"
            placeholder="开始时间"
            style="width: 165px"
            @change="onFilterChange"
          />
          <span class="range-sep">至</span>
          <CsDatePicker
            v-model="endTime"
            type="datetimerange"
            placeholder="结束时间"
            style="width: 165px"
            @change="onFilterChange"
          />
        </div>
        <CsInput
          v-model="keyword"
          placeholder="搜索操作内容 / 路径 / IP / 操作人 / 目标"
          clearable
          prefix-icon="search"
          style="width: 260px"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        />
        <CsButton variant="primary" icon-name="search" @click="onFilterChange">查询</CsButton>
        <CsButton @click="resetFilters">重置</CsButton>
        <CsButton icon-name="refresh" @click="refresh">刷新</CsButton>
        <CsButton icon-name="download" :loading="exporting" @click="doExport">导出</CsButton>
    </CsToolbar>

    <CsContainer header="系统审计日志">
      <div v-if="loading" class="state-block">
        <CsSpinner size="large" />
      </div>

      <CsEmpty v-else-if="rows.length === 0" icon-name="file" description="暂无审计日志" />

      <!-- 审计日志卡片网格（官方 ItemCard）：摘要 + 字段表 + 大文本预览 -->
      <CsCardGrid v-else>
        <CsCard v-for="rv in rows" :key="rv.row.id">
          <template #header>
            <div class="audit-head">
              <span class="action-name">{{ rv.title }}</span>
              <div class="audit-meta">
                <CsBadge :color="CATEGORY_BADGE[rv.categoryColor]">{{ rv.categoryName }}</CsBadge>
                <CsBadge :color="rv.row.operator_type === 'admin' ? 'severity-medium' : rv.row.operator_type === 'system' ? 'grey' : 'blue'">
                  {{ rv.row.operator_type === 'admin' ? '管理员' : rv.row.operator_type === 'system' ? '系统' : '用户' }}
                </CsBadge>
                <button
                  v-if="rv.row.operator_type !== 'system'"
                  type="button"
                  class="audit-operator audit-operator--link"
                  :title="rv.row.operator_name ? `按操作人 ${rv.row.operator_name} 筛选` : undefined"
                  @click="filterByOperator(rv.row.operator_name)"
                >
                  {{ rv.row.operator_name || `#${rv.row.operator_id}` }}
                  <span v-if="rv.row.operator_name && rv.row.operator_id">#{{ rv.row.operator_id }}</span>
                </button>
                <CsBadge v-if="(rv.row.status ?? 0) >= 400" color="red">失败 {{ rv.row.status }}</CsBadge>
              </div>
              <code class="action-code">{{ rv.row.action }}</code>
            </div>
          </template>

          <template #actions>
            <code class="audit-ip" :title="rv.row.ip || undefined">{{ rv.row.ip || '—' }}</code>
          </template>

          <div v-if="rv.row.target_type" class="audit-target">
            目标：{{ targetLabel(rv.row.target_type) }}
            <span v-if="rv.row.target_name" class="audit-target-name">{{ rv.row.target_name }}</span>
            <span v-if="rv.row.target_id" class="audit-target-id">#{{ rv.row.target_id }}</span>
          </div>

          <div class="audit-summary" :title="rv.view.summary">{{ rv.view.summary || '—' }}</div>

          <div v-if="rv.view.fields.length" class="audit-fields">
            <div v-for="(f, i) in rv.view.fields.slice(0, CARD_FIELD_LIMIT)" :key="i" class="audit-field-row">
              <span class="af-label">{{ f.label }}</span>
              <span class="af-value" :class="{ 'af-value--mono': f.mono }" :title="f.value">{{ f.value }}</span>
            </div>
          </div>

          <div v-if="rv.view.texts.length" class="audit-text-chips">
            <span v-for="(t, i) in rv.view.texts" :key="i" class="audit-text-chip" :title="t.preview" @click="openDetail(rv.row)">
              <span>{{ t.title }}</span><span class="chip-meta">{{ t.meta }}</span>
            </span>
          </div>

          <template #footer>
            <span class="audit-time">{{ fmtTime(rv.row.created_at) }}</span>
            <CsButton variant="inline-link" @click="openDetail(rv.row)">
              {{ rv.row.detail_truncated ? '查看完整原文' : rv.view.fields.length > CARD_FIELD_LIMIT ? `全部 ${rv.view.fields.length} 项 / 原文` : '展开 / 原文' }}
            </CsButton>
          </template>
        </CsCard>
      </CsCardGrid>

      <template #footer>
        <CsPaginationBar>
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            @current-change="load"
            @size-change="() => { page = 1; load() }"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <!-- 详情弹窗：翻译视图 / 原文 -->
    <CsModal v-model:visible="detailModalOpen" header="操作详情" size="large">
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
  align-items: center;
  gap: 6px;
}

.range-sep {
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb);
}

.state-block {
  padding: 48px 0;
  text-align: center;
}

/* ================= 审计日志卡片（官方 ItemCard 口径） ================= */
.audit-head {
  min-inline-size: 0;
}

.action-name {
  display: block;
  font-size: var(--font-size-heading-s-hm1hv8, 16px);
  line-height: var(--line-height-heading-s-3z7qjv, 20px);
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}

.audit-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.audit-operator {
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

/* 操作人可点：按该邮箱筛选（搜索框搜不到名字的旧问题从此有入口） */
.audit-operator--link {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  color: var(--color-text-link-default, #0972d3);
  cursor: pointer;
  text-decoration: none;
}

.audit-operator--link:hover {
  text-decoration: underline;
}

.audit-target {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-end: 4px;
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.audit-target-name {
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}

.audit-target-id {
  font-family: var(--font-family-monospace, ui-monospace, SFMono-Regular, Menlo, monospace);
}

.action-code {
  display: inline-block;
  margin-block-start: 4px;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

.audit-ip {
  display: block;
  max-inline-size: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: nowrap;
}

/* 正文封顶：字段再多也不让单张卡片无限长，超出部分靠「展开 / 原文」看完整内容 */
:deep(.cs-card__body) {
  max-block-size: 240px;
  overflow: hidden;
}

/* 翻译视图：摘要一句话 */
.audit-summary {
  padding: 8px 10px;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: var(--color-background-cell-shaded-twxsbz, #f2f3f3);
  font-size: var(--font-size-body-s, 12px);
  line-height: 18px;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 翻译视图：字段表 */
.audit-fields {
  margin-block-start: 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit-field-row {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;

  .af-label {
    flex: 0 0 auto;
    min-width: 64px;
    color: var(--color-text-body-secondary-66ygbb, #656871);
  }

  .af-value {
    flex: 1 1 auto;
    min-width: 0;
    color: var(--color-text-body-default-b2us04, #0f141a);
    overflow-wrap: anywhere;
    /* 单字段最多两行，避免一段长 JSON / 长地址把整张卡片撑高 */
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .af-value--mono {
    font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  }
}

/* 翻译视图：大文本预览 chip（模板/正文/PEM） */
.audit-text-chip .chip-meta {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.audit-text-chips {
  margin-block-start: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.audit-text-chip {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  border: 1px dashed var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  padding: 2px 8px;
  cursor: pointer;

  &:hover {
    border-color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.audit-time {
  margin-inline-end: auto;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
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
