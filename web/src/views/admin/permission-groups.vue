<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { BP, MQ } from '@/utils/media'
import { toast, messageBox } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsInput,
  CsModal,
  CsOption,
  CsPage,
  CsSelect,
  CsSpinner,
  CsToolbar,
  CsTooltip,
} from '@/components/cloudscape'
import {
  createPermissionGroup,
  deletePermissionGroup,
  getPermissionGroups,
  getAccessPoints,
  reorderPermissionGroups,
  setPermissionGroupAccessPoints,
  updatePermissionGroup,
  type PermissionGroup,
} from '@/api/admin'
import type { UserAccessPoint } from '@/api/types'
import { errMsg } from '@/api/http'
import SortHandle from '@/components/base/SortHandle.vue'
import { useCardDragSort } from '@/composables/useCardDragSort'
import { useTableRowDrag } from '@/composables/useTableRowDrag'

const router = useRouter()

const list = ref<PermissionGroup[]>([])
const loading = ref(false)

const isMobile = ref(false)
let mobileMq: MediaQueryList | null = null
const onMobileMq = () => {
  isMobile.value = window.matchMedia(MQ.tablet).matches || window.innerWidth <= BP.tabletMin
}

async function load() {
  loading.value = true
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) {
      list.value = data.data.items
      syncSnapshot()
    } else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载权限组失败'))
  } finally {
    loading.value = false
  }
}

// ---- 排序（拖拽）----
// 只影响管理端列表与下拉展示顺序；组内接入点优先级仍由编辑器单独维护。
const desktopReorderEnabled = computed(() => !isMobile.value && !loading.value)
const {
  canReorder: canTableReorder,
  draggingId: tableDraggingId,
  onPointerDown: onTablePointerDown,
} = useTableRowDrag({
  list,
  enabled: desktopReorderEnabled,
  persist: reorderPermissionGroups,
})

const mobileReorderEnabled = computed(() => isMobile.value && !loading.value)
const {
  canReorder: canCardReorder,
  draggingId: cardDraggingId,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  syncSnapshot,
} = useCardDragSort({
  list,
  idOf: (g) => g.id,
  enabled: mobileReorderEnabled,
  persist: reorderPermissionGroups,
})

onMounted(() => {
  mobileMq = window.matchMedia(MQ.tablet)
  onMobileMq()
  mobileMq.addEventListener('change', onMobileMq)
  // matchMedia 为主；再挂 resize 兜住不派发 change 的嵌入环境（IAB/WebView）
  window.addEventListener('resize', onMobileMq)
  load()
})
onUnmounted(() => {
  mobileMq?.removeEventListener('change', onMobileMq)
  window.removeEventListener('resize', onMobileMq)
})

// ---- 创建/编辑基础信息 ----
const formOpen = ref(false)
const editing = ref<PermissionGroup | null>(null)
const saving = ref(false)
const form = reactive({ name: '', remark: '' })

function openCreate() {
  editing.value = null
  form.name = ''
  form.remark = ''
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = row
  form.name = row.name
  form.remark = row.remark
  formOpen.value = true
}

async function save() {
  if (!form.name.trim()) {
    toast.warning('请填写名称')
    return
  }
  saving.value = true
  try {
    const { data } = editing.value
      ? await updatePermissionGroup(editing.value.id, { name: form.name.trim(), remark: form.remark })
      : await createPermissionGroup({ name: form.name.trim(), remark: form.remark })
    if (data.code === 0) {
      toast.success(editing.value ? '权限组已更新' : '权限组已创建')
      formOpen.value = false
      load()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// ===== 组内接入点优先级编辑器（2026-09-03：订阅节点输出顺序同此）=====
const apDialogOpen = ref(false)
const apTarget = ref<PermissionGroup | null>(null)
const orderingSaving = ref(false)
const orderedApIds = ref<number[]>([])
const selectedApId = ref<number | undefined>(undefined)
const allAPs = ref<UserAccessPoint[]>([])

const apNameMap = computed(() => {
  const m = new Map<number, string>()
  for (const ap of allAPs.value) m.set(ap.id, ap.name)
  return m
})
// 可选未加入列表的接入点（仅启用项可加入）
const availableAPs = computed(() => allAPs.value.filter((ap) => ap.enabled && !orderedApIds.value.includes(ap.id)))

async function openAPEditor(row: any) {
  apTarget.value = row
  orderedApIds.value = [...(row.access_point_ids || [])]
  apDialogOpen.value = true
  if (allAPs.value.length === 0) {
    try {
      const { data } = await getAccessPoints()
      if (data.code === 0) allAPs.value = data.data.items
    } catch (e) {
      toast.error(errMsg(e, '加载接入点失败'))
    }
  }
}

function addAP() {
  const id = Number(selectedApId.value)
  if (!id || orderedApIds.value.includes(id)) return
  orderedApIds.value.push(id)
  selectedApId.value = undefined
}

function moveAP(idx: number, dir: -1 | 1) {
  const j = idx + dir
  if (j < 0 || j >= orderedApIds.value.length) return
  const arr = orderedApIds.value
  ;[arr[idx], arr[j]] = [arr[j], arr[idx]]
}

function removeAP(idx: number) {
  orderedApIds.value.splice(idx, 1)
}

async function saveOrdering() {
  if (!apTarget.value) return
  orderingSaving.value = true
  try {
    const { data } = await setPermissionGroupAccessPoints(apTarget.value.id, orderedApIds.value)
    if (data.code === 0) {
      toast.success('接入点优先级已保存，订阅节点顺序即此排列')
      apDialogOpen.value = false
      load()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '保存优先级失败'))
  } finally {
    orderingSaving.value = false
  }
}

// 订阅模板已独立成页；这里只做带目标组的跳转，编辑入口集中在「订阅与财务 · 订阅模板」
function openTemplates(row: PermissionGroup) {
  router.push({ path: '/admin/sub-templates', query: { group: String(row.id) } })
}

// 模板状态三态：绑定模板库（活引用）/ 组内自定义副本 / 系统默认。
// template_mode/template_source 由后端按唯一口径解析（悬挂绑定会显示为自定义）。
function templateBadge(row: PermissionGroup) {
  if (row.template_mode === 'bound') return `绑定「${row.sub_template_name || '模板'}」`
  return row.template_source === 'custom' ? '自定义模板' : '系统默认'
}

function templateBadgeTone(row: PermissionGroup): 'info' | 'neutral' {
  return row.template_mode === 'bound' || row.template_source === 'custom' ? 'info' : 'neutral'
}

function visibleAPs(row: PermissionGroup): string[] {
  return (row.access_point_names || []).slice(0, 3)
}

function extraAPCount(row: PermissionGroup): number {
  return Math.max(0, (row.access_point_names?.length || 0) - 3)
}

function apFullTooltip(row: PermissionGroup): string {
  return (row.access_point_names || []).map((n, i) => `${i + 1}. ${n}`).join('；')
}

async function remove(row: any) {
  try {
    await messageBox.confirm(`确认删除权限组「${row.name}」？关联的套餐和节点权限引用将一并解除。`, '删除权限组', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deletePermissionGroup(row.id)
    if (data.code === 0) {
      toast.success('已删除')
      load()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <TipIcon content="权限组用于组织与分发接入点。点击订阅模板标签即可前往编辑、绑定模板库或转为自定义。" />
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新增权限组</CsButton>
      </template>
    </CsToolbar>

    <CsContainer header="权限分组列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <CsEmpty v-else-if="list.length === 0" icon-name="folder-open" description="尚无权限组，点击右上角「新增权限组」" />

      <!-- 桌面端紧凑表格 -->
      <div v-else-if="!isMobile" class="pg-table-wrap" @dragstart.prevent>
        <table
          class="pg-table"
          :class="{ 'pg-table--dragging': tableDraggingId !== null }"
          aria-label="权限分组列表"
        >
          <thead>
            <tr>
              <th v-if="canTableReorder" class="col-sort" aria-label="拖拽排序" />
              <th>权限组</th>
              <th class="col-template">订阅模板</th>
              <th>包含接入点</th>
              <th>备注</th>
              <th class="col-actions">操作</th>
            </tr>
          </thead>
          <tbody @pointerdown="onTablePointerDown">
            <tr
              v-for="row in list"
              :key="row.id"
              class="pg-row"
              :class="{ 'pg-row--dragging': tableDraggingId === row.id }"
              :data-sort-id="row.id"
            >
              <td v-if="canTableReorder" class="col-sort">
                <SortHandle :draggable="false" title="拖拽调整权限组顺序" />
              </td>

              <td>
                <div class="pg-name-cell">
                  <button type="button" class="pg-name-btn" title="点击编辑权限组" @click="openEdit(row)">
                    {{ row.name }}
                  </button>
                  <span class="pg-id pg-id--inline">#{{ row.id }}</span>
                </div>
              </td>

              <td class="col-template">
                <button
                  type="button"
                  class="pg-template-flag"
                  title="点击前往「订阅模板」页调整模板或绑定"
                  @click="openTemplates(row)"
                >
                  <span class="soft-tag soft-tag--clickable" :class="`soft-tag--${templateBadgeTone(row)}`">
                    {{ templateBadge(row) }}
                  </span>
                </button>
              </td>

              <td>
                <div class="pg-aps">
                  <template v-if="row.access_point_names && row.access_point_names.length">
                    <span v-for="(name, idx) in visibleAPs(row)" :key="`${idx}-${name}`" class="soft-tag soft-tag--info">
                      {{ idx + 1 }}. {{ name }}
                    </span>
                    <CsTooltip v-if="extraAPCount(row) > 0" :content="apFullTooltip(row)">
                      <span class="soft-tag soft-tag--neutral pg-ap-more">+{{ extraAPCount(row) }}</span>
                    </CsTooltip>
                    <CsButton variant="inline-link" :wrap-text="false" @click="openAPEditor(row)">调整顺序</CsButton>
                  </template>
                  <template v-else>
                    <span class="pg-empty">暂无接入点</span>
                    <CsButton variant="inline-link" :wrap-text="false" @click="openAPEditor(row)">添加</CsButton>
                  </template>
                </div>
              </td>

              <td>
                <span :class="{ 'cell-muted': !row.remark }">{{ row.remark || '—' }}</span>
              </td>

              <td class="col-actions">
                <div class="row-actions">
                  <CsButton :wrap-text="false" @click="openEdit(row)">编辑</CsButton>
                  <CsButton variant="danger-outline" :wrap-text="false" @click="remove(row)">删除</CsButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 移动端精简卡片 -->
      <CsCardGrid
        v-else
        :min-width="320"
        @dragstart="onDragStart"
        @dragover="onDragOver"
        @drop="onDrop"
        @dragend="onDragEnd"
      >
        <CsCard
          v-for="row in list"
          :key="row.id"
          :class="{ 'pg-card--dragging': cardDraggingId === row.id }"
          :data-sort-id="row.id"
        >
          <template #header>
            <div class="pg-head">
              <SortHandle v-if="canCardReorder" title="拖拽调整权限组顺序" />
              <button type="button" class="pg-name" title="点击编辑权限组" @click="openEdit(row)">
                {{ row.name }}
              </button>
            </div>
            <div class="pg-id">#{{ row.id }}</div>
          </template>
          <template #actions>
            <button
              type="button"
              class="pg-template-flag"
              title="点击前往「订阅模板」页调整模板或绑定"
              @click="openTemplates(row)"
            >
              <span class="soft-tag soft-tag--clickable" :class="`soft-tag--${templateBadgeTone(row)}`">
                {{ templateBadge(row) }}
              </span>
            </button>
          </template>

          <CsCardFacts>
            <CsCardFact v-if="row.remark" label="备注说明" :value="row.remark" full-width />
            <CsCardFact label="包含接入点" full-width>
              <div class="pg-aps">
                <template v-if="row.access_point_names && row.access_point_names.length">
                  <span v-for="(name, idx) in visibleAPs(row)" :key="`${idx}-${name}`" class="soft-tag soft-tag--info">
                    {{ idx + 1 }}. {{ name }}
                  </span>
                  <CsTooltip v-if="extraAPCount(row) > 0" :content="apFullTooltip(row)">
                    <span class="soft-tag soft-tag--neutral pg-ap-more">+{{ extraAPCount(row) }}</span>
                  </CsTooltip>
                  <CsButton variant="inline-link" @click="openAPEditor(row)">调整顺序</CsButton>
                </template>
                <template v-else>
                  <span class="pg-empty">暂无接入点</span>
                  <CsButton variant="inline-link" @click="openAPEditor(row)">添加</CsButton>
                </template>
              </div>
            </CsCardFact>
          </CsCardFacts>

          <template #footer>
            <CsButton :wrap-text="false" @click="openEdit(row)">编辑</CsButton>
            <CsButton variant="danger-outline" :wrap-text="false" @click="remove(row)">删除</CsButton>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <!-- ===== 权限组基础信息编辑弹窗 ===== -->
    <CsModal v-model:visible="formOpen" :header="editing ? '编辑权限组' : '新增权限组'" size="medium" :close-on-click-overlay="false">
      <CsForm label-position="top">
        <CsFormItem label="名称"><CsInput v-model="form.name" placeholder="如 VIP 1 / 基础套餐组" /></CsFormItem>
        <CsFormItem label="备注说明"><CsInput v-model="form.remark" placeholder="选填，如 普通节点权限组" /></CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="formOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">保存</CsButton>
      </template>
    </CsModal>

    <!-- ===== 组内接入点优先级编辑弹窗 ===== -->
    <CsModal
      v-model:visible="apDialogOpen"
      :header="`接入点优先级：${apTarget?.name || ''}`"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsForm label-position="top">
        <CsFormItem>
          <template #label>
            添加接入点（下方列表调整顺序）
            <TipIcon content="列表顺序即该组订阅中节点的输出顺序（$PROXIES$ 注入与客户端显示同序）；同组调整不影响该接入点在其他组中的优先级。" />
          </template>
          <div style="display: flex; gap: 8px; width: 100%">
            <CsSelect
              v-model="selectedApId"
              placeholder="选择启用中的接入点"
              style="flex: 1"
              :disabled="availableAPs.length === 0"
              filterable
            >
              <CsOption v-for="ap in availableAPs" :key="ap.id" :label="ap.name" :value="ap.id" />
            </CsSelect>
            <CsButton variant="primary" icon-name="add-plus" :disabled="!selectedApId" @click="addAP">
              加入
            </CsButton>
          </div>
        </CsFormItem>

        <div v-if="orderedApIds.length === 0" class="cs-text-secondary" style="font-size: 13px; padding: 16px 0; text-align: center">
          当前权限组未绑定接入点（订阅将无节点）
        </div>
        <div v-else class="order-list">
          <div v-for="(apId, idx) in orderedApIds" :key="apId" class="order-row">
            <span class="order-idx">{{ idx + 1 }}</span>
            <span class="order-name">{{ apNameMap.get(apId) || ('#' + apId) }}</span>
            <div class="order-actions">
              <CsButton variant="icon" icon-name="arrow-up" aria-label="上移" :disabled="idx === 0" @click="moveAP(idx, -1)" />
              <CsButton variant="icon" icon-name="arrow-down" aria-label="下移" :disabled="idx === orderedApIds.length - 1" @click="moveAP(idx, 1)" />
              <CsButton variant="icon" icon-name="remove" aria-label="移除" @click="removeAP(idx)" />
            </div>
          </div>
        </div>
      </CsForm>
      <template #footer>
        <CsButton @click="apDialogOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="orderingSaving" @click="saveOrdering">保存顺序</CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
.pg-table-wrap {
  width: 100%;
  overflow-x: auto;
}

.pg-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-size: 13px;

  th {
    padding: 8px 10px;
    text-align: left;
    font-size: 12px;
    font-weight: 700;
    color: var(--color-text-label-g80qnu, #414d5c);
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
    white-space: nowrap;
  }

  td {
    padding: 10px;
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
    vertical-align: middle;
  }

  .col-sort {
    width: 1%;
    padding-inline: 2px 4px;
    text-align: center;
    white-space: nowrap;
  }

  .col-template,
  .col-actions {
    width: 1%;
    white-space: nowrap;
  }
}

.pg-table--dragging {
  user-select: none;
  cursor: grabbing;

  .pg-row:not(.pg-row--dragging):hover {
    background: transparent;
  }
}

.pg-row {
  transition: background-color 0.15s ease;

  &:hover {
    background: var(--color-background-dropdown-item-hover-pvfsrg, rgba(0, 7, 22, 0.04));
  }

  &--dragging {
    position: relative;
    z-index: 10;
    background: var(--color-background-container-content-mjfil9, #ffffff);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.14);
    will-change: transform;
    transition: none;
  }
}

.pg-name-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.pg-name-btn {
  padding: 0;
  border: 0;
  background: transparent;
  font-family: inherit;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  &:hover {
    text-decoration: underline;
  }
}

.row-actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.soft-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  font-size: 12px;
  font-weight: 500;
  line-height: 16px;
  white-space: nowrap;
  flex-shrink: 0;
}

.soft-tag--info {
  color: var(--x-tag-info-color, #006ce0);
  background: var(--x-tag-info-bg, #f0fbff);
  border-color: var(--x-tag-info-border, rgba(0, 108, 224, 0.25));
}

.soft-tag--neutral {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  background: var(--x-card-soft, #f8fafc);
  border-color: var(--color-border-divider-default-diwd4w, #e9ebed);
}

.soft-tag--clickable {
  cursor: pointer;
  transition: filter 0.15s ease;

  &:hover {
    filter: brightness(0.96);
  }
}

.pg-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.pg-card--dragging {
  opacity: 0.45;
}

.pg-name {
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

.pg-id {
  margin-block-start: 2px;
  font-family: var(--x-font-mono);
  font-variant-numeric: tabular-nums;
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.pg-id--inline {
  margin-block-start: 0;
}

.pg-template-flag {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  cursor: pointer;
  line-height: 0;
}

.pg-aps {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.pg-table .pg-aps {
  flex-wrap: nowrap;
  white-space: nowrap;
}

.pg-ap-more {
  cursor: help;
}

.pg-empty {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.order-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  padding: 8px;
}
.order-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  padding: 6px 10px;
}
.order-idx {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--color-background-item-selected-dotkup);
  color: var(--color-text-link-default-qabnuk);
  font-size: 12px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.order-name {
  flex: 1;
  font-size: 13px;
  color: var(--color-text-body-default-b2us04);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.order-actions {
  display: flex;
  gap: 2px;
}
</style>

