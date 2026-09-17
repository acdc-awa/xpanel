<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Document, Edit, Delete, Loading, FolderChecked, ArrowUp, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import TipIcon from '@/components/base/TipIcon.vue'
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
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增权限组</el-button>
        <TipIcon content="权限组用于组织与分发接入点。订阅模板已独立成页，点卡片上的「订阅模板」或模板徽标即可前往编辑。" />
      </div>
    </div>

    <BaseCard title="权限分组列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><FolderChecked /></el-icon>
        <p style="margin-top: 8px">尚无权限组，点击右上角「新增权限组」</p>
      </div>

      <!-- 全局统一权限组卡片网格流 (自适应 1~4 列) -->
      <div v-else class="group-card-grid">
        <div v-for="row in list" :key="row.id" class="group-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
              <span class="group-name" title="点击编辑权限组" @click="openEdit(row)">{{ row.name }}</span>
            </div>
            <span
              class="x-chip"
              :class="row.clash_template && row.clash_template.trim() ? 'purple' : 'gray'"
              style="font-size: 10.5px; cursor: pointer"
              title="订阅模板已独立成页，点击前往编辑"
              @click="openTemplates(row)"
            >
              {{ row.clash_template && row.clash_template.trim() ? '自定义模板' : '系统默认' }}
            </span>
          </div>

          <!-- 属性网格 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <span class="item-label">备注说明</span>
              <div class="item-value">{{ row.remark || '—' }}</div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">包含接入点</span>
              <div class="item-value">
                <template v-if="row.access_point_names && row.access_point_names.length">
                  <span
                    v-for="(name, idx) in row.access_point_names"
                    :key="name"
                    class="x-chip blue"
                    style="margin-right: 4px; margin-bottom: 2px; font-size: 10.5px"
                  >
                    {{ idx + 1 }}. {{ name }}
                  </span>
                  <el-button size="small" text type="primary" style="font-size: 11.5px" @click="openAPEditor(row)">
                    调整顺序
                  </el-button>
                </template>
                <span v-else class="muted font-11">暂无接入点（在拓扑画布中绑定该组）</span>
              </div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="warning" plain @click="openTemplates(row)">
              <el-icon><Document /></el-icon>&nbsp;订阅模板
            </el-button>
            <el-button size="small" type="primary" plain @click="openEdit(row)">
              <el-icon><Edit /></el-icon>&nbsp;编辑
            </el-button>
            <el-button size="small" type="danger" plain @click="remove(row)">
              <el-icon><Delete /></el-icon>&nbsp;删除
            </el-button>
          </div>
        </div>
      </div>
    </BaseCard>

    <!-- ===== 权限组基础信息编辑弹窗 ===== -->
    <el-dialog v-model="formOpen" :title="editing ? '编辑权限组' : '新增权限组'" width="440px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 VIP 1 / 基础套餐组" /></el-form-item>
        <el-form-item label="备注说明"><el-input v-model="form.remark" placeholder="选填，如 普通节点权限组" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- ===== 组内接入点优先级编辑弹窗 ===== -->
    <el-dialog
      v-model="apDialogOpen"
      :title="`接入点优先级 · ${apTarget?.name || ''}`"
      width="560px"
      :append-to-body="true"
    >
      <el-alert type="info" :closable="false" style="margin-bottom: 12px"
        title="列表顺序即该权限组订阅中节点的输出顺序（$PROXIES$ 注入与客户端显示同序）。同组调整不影响接入点在其他权限组中的优先级。"
      />
      <el-form label-position="top">
        <el-form-item label="添加接入点（下方列表调整顺序）">
          <div style="display: flex; gap: 8px; width: 100%">
            <el-select
              v-model="selectedApId"
              placeholder="选择启用中的接入点"
              style="flex: 1"
              :disabled="availableAPs.length === 0"
              filterable
            >
              <el-option v-for="ap in availableAPs" :key="ap.id" :label="ap.name" :value="ap.id" />
            </el-select>
            <el-button type="primary" :disabled="!selectedApId" @click="addAP">
              <el-icon><Plus /></el-icon>&nbsp;加入
            </el-button>
          </div>
        </el-form-item>

        <div v-if="orderedApIds.length === 0" class="muted" style="font-size: 12.5px; padding: 16px 0; text-align: center">
          当前权限组未绑定接入点（订阅将无节点）
        </div>
        <div v-else class="order-list">
          <div v-for="(apId, idx) in orderedApIds" :key="apId" class="order-row">
            <span class="order-idx">{{ idx + 1 }}</span>
            <span class="order-name">{{ apNameMap.get(apId) || ('#' + apId) }}</span>
            <div class="order-actions">
              <el-button size="small" text :icon="ArrowUp" :disabled="idx === 0" @click="moveAP(idx, -1)" />
              <el-button size="small" text :icon="ArrowDown" :disabled="idx === orderedApIds.length - 1" @click="moveAP(idx, 1)" />
              <el-button size="small" text type="danger" :icon="Delete" @click="removeAP(idx)" />
            </div>
          </div>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="apDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="orderingSaving" @click="saveOrdering">保存顺序</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
.cell-mono {
  font-family: var(--x-font-mono, monospace);
  font-size: 12px;
}
.table-empty { padding: 30px 0; text-align: center; color: var(--x-text-3); font-size: 13px; }

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


/* ================= 全局统一权限组卡片网格流 ================= */
.group-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.group-card {
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

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .group-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }

  .card-foot-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 10px;
    border-top: 1px solid var(--x-border-light, #f1f5f9);
    margin-top: 6px;

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
    }
  }
}

@media (max-width: 640px) {
  .group-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>

