<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  CsButton,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsTable,
  CsToolbar,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import type { TableColumn } from '@/components/cloudscape'
import { createInvitations, getInvitations, revokeInvitation, type Invitation } from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateTimeMinute } from '@/utils/timezone'

const list = ref<Invitation[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getInvitations()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载邀请码失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 状态筛选（含隐性「已过期」：status=0 且已过有效期，过期不落库注册时才拒）----
type InvStatusFilter = '' | 'unused' | 'used' | 'disabled' | 'expired'
const statusFilter = ref<InvStatusFilter>('')

function isExpired(row: Invitation): boolean {
  return row.status === 0 && !!row.expires_at && new Date(row.expires_at).getTime() < Date.now()
}

const filtered = computed(() => {
  switch (statusFilter.value) {
    case 'unused':
      return list.value.filter((r) => r.status === 0 && !isExpired(r))
    case 'used':
      return list.value.filter((r) => r.status === 1)
    case 'disabled':
      return list.value.filter((r) => r.status === 2)
    case 'expired':
      return list.value.filter(isExpired)
    default:
      return list.value
  }
})

const emptyText = computed(() =>
  list.value.length === 0
    ? '暂无邀请码，点击右上角「生成邀请码」'
    : '未找到匹配当前筛选的邀请码',
)

const invColumns: TableColumn[] = [
  { id: 'code', header: '邀请码（点击复制）' },
  { id: 'status', header: '状态' },
  { id: 'created_by', header: '创建人' },
  { id: 'expires_at', header: '有效期' },
  { id: 'created_at', header: '创建时间' },
  { id: 'used_at', header: '使用时间' },
  { id: 'actions', header: '操作' },
]

function statusOf(row: Invitation): { className: string; text: string } {
  if (row.status === 1) return { className: 'inv-status-used', text: '已使用' }
  if (row.status === 2) return { className: 'inv-status-danger', text: '已禁用' }
  if (isExpired(row)) return { className: 'inv-status-danger', text: '已过期' }
  return { className: 'inv-status-unused', text: '未使用' }
}

function isMutedRow(row: Invitation): boolean {
  return row.status !== 0 || isExpired(row)
}

// ---- 本地分页（列表后端 Limit(200) 全量返回）----
const page = ref(1)
const pageSize = ref(20)
const paged = computed(() =>
  filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
)

watch(statusFilter, () => {
  page.value = 1
})

// 条目减少后页码回夹：停在被清空的页时列表空白
watch(
  [() => filtered.value.length, pageSize],
  ([len, size]) => {
    const max = Math.max(1, Math.ceil(len / size))
    if (page.value > max) page.value = max
  },
)

const genOpen = ref(false)
const invForm = reactive({ count: 5, expires: '' })
const creating = ref(false)
const generated = ref<string[]>([])

function openGen() {
  generated.value = []
  invForm.expires = ''
  invForm.count = 5
  genOpen.value = true
}

async function create() {
  creating.value = true
  try {
    const { data } = await createInvitations(invForm.count, invForm.expires)
    if (data.code === 0) {
      generated.value = data.data.codes
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    creating.value = false
  }
}

function getRegisterUrl(code: string) {
  return `${location.origin}/register?code=${encodeURIComponent(code)}`
}

async function copyCode(code: string) {
  try {
    await navigator.clipboard.writeText(code)
    ElMessage.success(`邀请码 ${code} 已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

async function copyRegisterLink(code: string) {
  const url = getRegisterUrl(code)
  try {
    await navigator.clipboard.writeText(url)
    ElMessage.success('注册链接已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

async function copyAll() {
  try {
    await navigator.clipboard.writeText(generated.value.join('\n'))
    ElMessage.success('已复制全部邀请码')
  } catch {
    ElMessage.warning('复制失败，请手动选择')
  }
}

async function copyAllLinks() {
  const links = generated.value.map(c => getRegisterUrl(c))
  try {
    await navigator.clipboard.writeText(links.join('\n'))
    ElMessage.success('已复制全部注册链接')
  } catch {
    ElMessage.warning('复制失败，请手动选择')
  }
}

async function revoke(row: any) {
  try {
    await ElMessageBox.confirm(`确认作废邀请码 ${row.code}？作废后该邀请码将无法用于注册。`, '作废邀请码', {
      type: 'warning',
      confirmButtonText: '作废',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    const { data } = await revokeInvitation(row.id)
    if (data.code === 0) {
      ElMessage.success('已作废')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '作废失败'))
  }
}

function fmtTime(t: string | null) {
  return t ? formatDateTimeMinute(t) : '—'
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px">
        <CsOption label="未使用" value="unused" />
        <CsOption label="已使用" value="used" />
        <CsOption label="已过期" value="expired" />
        <CsOption label="已禁用" value="disabled" />
      </CsSelect>
      <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="openGen">生成邀请码</CsButton>
      </template>
    </CsToolbar>

    <CsContainer header="邀请码列表">
      <CsTable
        compact
        :wrap-lines="false"
        :items="paged"
        :columns="invColumns"
        :loading="loading"
        loading-text="加载中…"
        track-by="id"
        :empty-text="emptyText"
      >
        <template #cell-code="{ row }">
          <button
            type="button"
            class="inv-code-btn cell-mono"
            :class="{ 'is-muted': isMutedRow(row) }"
            title="点击复制邀请码"
            @click="copyCode(row.code)"
          >
            {{ row.code }}
          </button>
        </template>
        <template #cell-status="{ row }">
          <span :class="statusOf(row).className">{{ statusOf(row).text }}</span>
        </template>
        <template #cell-created_by="{ row }">
          <span :class="{ 'cell-muted': isMutedRow(row) }">{{ row.created_by || '系统生成' }}</span>
        </template>
        <template #cell-expires_at="{ row }">
          <span v-if="row.expires_at" class="cell-mono" :class="{ 'cell-muted': isMutedRow(row) }">
            {{ fmtTime(row.expires_at) }}
          </span>
          <span v-else class="cell-muted">永不过期</span>
        </template>
        <template #cell-created_at="{ row }">
          <span class="cell-mono" :class="{ 'cell-muted': isMutedRow(row) }">{{ fmtTime(row.created_at) }}</span>
        </template>
        <template #cell-used_at="{ row }">
          <span v-if="row.used_at" class="cell-mono" :class="{ 'cell-muted': isMutedRow(row) }">
            {{ fmtTime(row.used_at) }}
          </span>
          <span v-else class="cell-muted">未使用</span>
        </template>
        <template #cell-actions="{ row }">
          <div v-if="row.status === 0 && !isExpired(row)" class="inv-actions">
            <CsButton variant="normal" :wrap-text="false" @click="copyRegisterLink(row.code)">
              复制链接
            </CsButton>
            <CsButton variant="danger-outline" :wrap-text="false" @click="revoke(row)">
              作废
            </CsButton>
          </div>
          <span v-else class="cell-muted">—</span>
        </template>
      </CsTable>

      <!-- 分页放容器 footer：官方容器 footer 自带分隔线与内边距 -->
      <template v-if="filtered.length > 0" #footer>
        <CsPaginationBar>
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="filtered.length"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            simple
            @size-change="page = 1"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <CsModal v-model:visible="genOpen" header="生成邀请码" size="small" :close-on-click-overlay="false">
      <template v-if="!generated.length">
        <CsForm label-position="top">
          <CsFormItem label="生成数量">
            <CsInputNumber v-model="invForm.count" :min="1" :max="100" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="过期时间">
            <CsDatePicker
              v-model="invForm.expires"
              type="datetime"
              placeholder="留空表示永不过期"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              style="width: 100%"
            />
          </CsFormItem>
        </CsForm>
      </template>
      <template v-else>
        <p class="cs-text-secondary" style="margin: 0 0 8px">已生成 {{ generated.length }} 个（一次性使用）：</p>
        <div class="inv-codes" style="max-height: 160px; overflow-y: auto">
          <div v-for="c in generated" :key="c" class="inv-code-row">
            <code class="inv-code">{{ c }}</code>
            <CsButton variant="link" @click="copyRegisterLink(c)">复制链接</CsButton>
          </div>
        </div>
        <div style="margin-top: 12px">
          <div class="cs-text-secondary" style="font-size: 12px; margin-bottom: 4px">只读备用文本（便于全选复制）：</div>
          <CsInput
            type="textarea"
            :rows="4"
            read-only
            :model-value="generated.join('\n')"
            placeholder="邀请码列表"
          />
        </div>
      </template>
      <template #footer>
        <template v-if="!generated.length">
          <CsButton @click="genOpen = false">取消</CsButton>
          <CsButton variant="primary" :loading="creating" @click="create">生成</CsButton>
        </template>
        <template v-else>
          <CsButton @click="genOpen = false; generated = []">完成</CsButton>
          <CsButton icon-name="copy" @click="copyAll">复制全部邀请码</CsButton>
          <CsButton variant="primary" icon-name="copy" @click="copyAllLinks">复制全部链接</CsButton>
        </template>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">

.inv-codes { display: grid; gap: 8px; max-height: 260px; overflow: auto; }
.inv-code-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--color-background-item-selected-dotkup);
  border-radius: 8px;
  padding: 6px 12px;
}
.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.inv-status-unused {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.inv-status-used {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.inv-status-danger {
  color: var(--color-text-status-error-ri38gt, #d91515);
}

.inv-code-btn {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font: inherit;
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  cursor: pointer;
  text-align: start;

  &:hover {
    text-decoration: underline;
  }

  &.is-muted {
    color: var(--color-text-status-inactive-fqjc97, #656871);
  }
}

.inv-actions {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.inv-code {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}
</style>