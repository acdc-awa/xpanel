<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { ElMessage } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsModal,
  CsOption,
  CsSelect,
  CsSpinner,
} from '@/components/cloudscape'
import {
  createPermissionGroup,
  deletePermissionGroup,
  getPermissionGroups,
  getAccessPoints,
  setPermissionGroupAccessPoints,
  updatePermissionGroup,
  type PermissionGroup,
} from '@/api/admin'
import type { UserAccessPoint } from '@/api/types'
import { errMsg } from '@/api/http'

const router = useRouter()

const list = ref<PermissionGroup[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载权限组失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

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
    ElMessage.warning('请填写名称')
    return
  }
  saving.value = true
  try {
    const { data } = editing.value
      ? await updatePermissionGroup(editing.value.id, { name: form.name.trim(), remark: form.remark })
      : await createPermissionGroup({ name: form.name.trim(), remark: form.remark })
    if (data.code === 0) {
      ElMessage.success(editing.value ? '权限组已更新' : '权限组已创建')
      formOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
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
      ElMessage.error(errMsg(e, '加载接入点失败'))
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
      ElMessage.success('接入点优先级已保存，订阅节点顺序即此排列')
      apDialogOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存优先级失败'))
  } finally {
    orderingSaving.value = false
  }
}

// 订阅模板已独立成页；这里只做带目标组的跳转，编辑入口集中在「订阅与财务 · 订阅模板」
function openTemplates(row: PermissionGroup) {
  router.push({ path: '/admin/sub-templates', query: { group: String(row.id) } })
}

function hasTemplate(row: PermissionGroup) {
  return !!(row.clash_template && row.clash_template.trim())
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除权限组「${row.name}」？关联的套餐和节点权限引用将一并解除。`, '删除权限组', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deletePermissionGroup(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新增权限组</CsButton>
        <TipIcon content="权限组用于组织与分发接入点。订阅模板已独立成页，点卡片上的「订阅模板」或模板徽标即可前往编辑。" />
      </div>
    </div>

    <CsContainer header="权限分组列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <CsIcon name="folder-open" size="large" />
        <p style="margin-top: 8px">尚无权限组，点击右上角「新增权限组」</p>
      </div>

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，正文 14/20，模板状态作为卡片角标 -->
      <CsCardGrid v-else>
        <CsCard v-for="row in list" :key="row.id">
          <template #header>
            <button type="button" class="pg-name" title="点击编辑权限组" @click="openEdit(row)">
              {{ row.name }}
            </button>
            <div class="pg-id">#{{ row.id }}</div>
          </template>
          <template #actions>
            <button
              type="button"
              class="pg-template-flag"
              title="订阅模板已独立成页，点击前往编辑"
              @click="openTemplates(row)"
            >
              <CsBadge :color="hasTemplate(row) ? 'blue' : 'grey'">
                {{ hasTemplate(row) ? '自定义模板' : '系统默认' }}
              </CsBadge>
            </button>
          </template>

          <CsCardFacts>
            <CsCardFact label="备注说明" :value="row.remark || '—'" full-width />
            <CsCardFact label="包含接入点" full-width>
              <div class="pg-aps">
                <template v-if="row.access_point_names && row.access_point_names.length">
                  <CsBadge v-for="(name, idx) in row.access_point_names" :key="name" color="blue">
                    {{ idx + 1 }}. {{ name }}
                  </CsBadge>
                  <CsButton variant="link" @click="openAPEditor(row)">调整顺序</CsButton>
                </template>
                <span v-else class="pg-empty">暂无接入点（在拓扑画布中绑定该组）</span>
              </div>
            </CsCardFact>
          </CsCardFacts>

          <template #footer>
            <CsButton icon-name="file" @click="openTemplates(row)">订阅模板</CsButton>
            <CsButton icon-name="edit" @click="openEdit(row)">编辑</CsButton>
            <CsButton icon-name="remove" @click="remove(row)">删除</CsButton>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <!-- ===== 权限组基础信息编辑弹窗 ===== -->
    <CsModal v-model:visible="formOpen" :header="editing ? '编辑权限组' : '新增权限组'" size="medium">
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
      :header="`接入点优先级 · ${apTarget?.name || ''}`"
      size="medium"
    >
      <CsAlert type="info" style="margin-bottom: 12px">
        列表顺序即该权限组订阅中节点的输出顺序（$PROXIES$ 注入与客户端显示同序）。同组调整不影响接入点在其他权限组中的优先级。
      </CsAlert>
      <CsForm label-position="top">
        <CsFormItem label="添加接入点（下方列表调整顺序）">
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

        <div v-if="orderedApIds.length === 0" class="muted" style="font-size: 12.5px; padding: 16px 0; text-align: center">
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
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }

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
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
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

.pg-empty {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.order-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 8px;
}
.order-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-bg);
  border: 1px solid var(--x-border-light);
  border-radius: 6px;
  padding: 6px 10px;
}
.order-idx {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--x-primary-soft);
  color: var(--x-primary);
  font-size: 11.5px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.order-name {
  flex: 1;
  font-size: 12.5px;
  color: var(--x-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.order-actions {
  display: flex;
  gap: 2px;
}
</style>

