<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsPagination,
  CsSelect,
  CsSpinner,
} from '@/components/cloudscape'
import { createInvitations, getInvitations, revokeInvitation, type Invitation } from '@/api/admin'
import { errMsg } from '@/api/http'

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

// 卡片状态角标：过期优先于未使用显示，避免管理员误认为可用
function chipOf(row: Invitation): { cls: string; text: string } {
  if (row.status === 1) return { cls: 'green', text: '已使用' }
  if (row.status === 2) return { cls: 'red', text: '已禁用' }
  if (isExpired(row)) return { cls: 'gray', text: '已过期' }
  return { cls: 'blue', text: '未使用' }
}

function badgeColorOf(row: Invitation): 'green' | 'red' | 'grey' | 'blue' {
  const cls = chipOf(row).cls
  return cls === 'gray' ? 'grey' : (cls as 'green' | 'red' | 'blue')
}

// ---- 本地分页（列表后端 Limit(200) 全量返回）----
const PAGE_SIZE = 12
const page = ref(1)
const paged = computed(() =>
  filtered.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE),
)

watch(statusFilter, () => {
  page.value = 1
})

// 条目减少后页码回夹：停在被清空的页时网格空白且分页器消失（12≤PAGE_SIZE 不渲染）
watch(
  () => filtered.value.length,
  (len) => {
    const max = Math.max(1, Math.ceil(len / PAGE_SIZE))
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
  if (!t) return '—'
  return t.replace('T', ' ').slice(0, 16)
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px">
          <CsOption label="未使用" value="unused" />
          <CsOption label="已使用" value="used" />
          <CsOption label="已过期" value="expired" />
          <CsOption label="已禁用" value="disabled" />
        </CsSelect>
        <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      </div>
      <CsButton variant="primary" icon-name="add-plus" @click="openGen">生成邀请码</CsButton>
    </div>

    <CsContainer header="邀请码列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="filtered.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <CsIcon name="ticket" size="large" />
        <p style="margin-top: 8px">{{ list.length === 0 ? '暂无邀请码，点击右上角「生成邀请码」' : '未找到匹配当前筛选的邀请码' }}</p>
      </div>

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，正文 14/20，次要信息 12/16 -->
      <CsCardGrid v-else>
        <CsCard v-for="row in paged" :key="row.id" :class="{ 'inv-card-muted': row.status !== 0 || isExpired(row) }">
          <template #header>
            <button type="button" class="inv-code" title="点击复制邀请码" @click="copyCode(row.code)">
              {{ row.code }}
            </button>
          </template>
          <template #actions>
            <CsBadge :color="badgeColorOf(row)">{{ chipOf(row).text }}</CsBadge>
          </template>

          <CsCardFacts>
            <CsCardFact label="创建人" :value="row.created_by || '系统生成'" />
            <CsCardFact label="有效期" :value="row.expires_at ? '至 ' + fmtTime(row.expires_at) : '永不过期'" mono />
            <CsCardFact label="创建时间" :value="fmtTime(row.created_at)" mono />
            <CsCardFact label="使用时间" :value="row.used_at ? fmtTime(row.used_at) : '未使用'" mono />
          </CsCardFacts>

          <template #footer>
            <template v-if="row.status === 0 && !isExpired(row)">
              <CsButton icon-name="external" @click="copyRegisterLink(row.code)">复制链接</CsButton>
              <CsButton icon-name="copy" @click="copyCode(row.code)">复制码</CsButton>
              <CsButton icon-name="remove" @click="revoke(row)">作废</CsButton>
            </template>
            <span v-else class="inv-card-note">
              {{ row.status === 1 ? '该邀请码已被成功使用' : isExpired(row) ? '该邀请码已过有效期，无法注册' : '该邀请码已被作废禁用' }}
            </span>
          </template>
        </CsCard>
      </CsCardGrid>

      <!-- 本地分页 -->
      <div v-if="filtered.length > PAGE_SIZE" style="display: flex; justify-content: center; margin-top: 16px">
        <CsPagination
          v-model:current-page="page"
          :total="filtered.length"
          :page-size="PAGE_SIZE"
          layout="prev, pager, next"
        />
      </div>
    </CsContainer>

    <CsModal v-model:visible="genOpen" header="生成邀请码" size="small">
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
        <p class="muted" style="margin: 0 0 8px">已生成 {{ generated.length }} 个（一次性使用）：</p>
        <div class="inv-codes" style="max-height: 160px; overflow-y: auto">
          <div v-for="c in generated" :key="c" class="inv-code-row">
            <code class="inv-code">{{ c }}</code>
            <CsButton variant="link" @click="copyRegisterLink(c)">复制链接</CsButton>
          </div>
        </div>
        <div style="margin-top: 12px">
          <div class="muted" style="font-size: 12px; margin-bottom: 4px">只读备用文本（便于全选复制）：</div>
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
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }

.inv-codes { display: grid; gap: 8px; max-height: 260px; overflow: auto; }
.inv-code-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 6px 12px;
}
.inv-code {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 16px;
  line-height: 20px;
  font-weight: 700;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  cursor: pointer;
  text-align: start;
  overflow-wrap: anywhere;

  &:hover {
    text-decoration: underline;
  }
}

.inv-card-muted .inv-code {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.inv-card-note {
  margin-inline-start: auto;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  text-align: end;
}
</style>