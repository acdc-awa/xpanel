<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Edit, Delete, Refresh } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { createPlan, deletePlan, getPermissionGroups, getPlans, updatePlan, type PermissionGroup, type Plan } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Plan[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getPlans()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载套餐失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Phase T：权限组下拉（套餐自动授权）
const groups = ref<PermissionGroup[]>([])
async function loadGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) groups.value = data.data.items
  } catch { /* 权限组加载失败不阻塞 */ }
}
onMounted(loadGroups)

const formOpen = ref(false)
const editing = ref(false)
const form = reactive({ id: 0, name: '', price_yuan: 25, traffic_gb: 200, duration_days: 30, device_limit: 0, permission_group_id: 0 })
const saving = ref(false)

function openCreate() {
  editing.value = false
  Object.assign(form, { id: 0, name: '', price_yuan: 25, traffic_gb: 200, duration_days: 30, device_limit: 0, permission_group_id: 0 })
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    name: row.name,
    price_yuan: Number((row.price_cents / 100).toFixed(2)),
    traffic_gb: row.traffic_gb,
    duration_days: row.duration_days,
    device_limit: row.device_limit || 0,
    permission_group_id: row.permission_group_id || 0,
  })
  formOpen.value = true
}

async function save() {
  if (!form.name || form.price_yuan < 0 || form.traffic_gb < 1 || form.duration_days < 1) {
    ElMessage.warning('请填写完整套餐信息')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name,
      price_cents: Math.round(form.price_yuan * 100),
      traffic_gb: form.traffic_gb,
      duration_days: form.duration_days,
      device_limit: form.device_limit,
      permission_group_id: form.permission_group_id || undefined,
    }
    const { data } = editing.value ? await updatePlan(form.id, payload) : await createPlan(payload)
    if (data.code === 0) {
      ElMessage.success(editing.value ? '已保存' : '已创建')
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

async function togglePlan(row: any) {
  try {
    const { data } = await updatePlan(row.id, { enabled: !row.enabled })
    if (data.code === 0) load()
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除套餐「${row.name}」？`, '删除套餐', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deletePlan(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
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
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增套餐</el-button>
    </div>

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="id" label="ID" width="60">
            <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
          </el-table-column>
          <el-table-column prop="name" label="名称" min-width="130">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
          </el-table-column>
          <el-table-column label="价格" width="100">
            <template #default="{ row }"><span class="cell-mono" style="font-weight: 600">¥ {{ (row.price_cents / 100).toFixed(2) }}</span></template>
          </el-table-column>
          <el-table-column label="流量" width="120">
            <template #default="{ row }">
              <span class="cell-mono">{{ row.traffic_gb >= 1024 ? `${(row.traffic_gb / 1024).toFixed(1)} TB` : `${row.traffic_gb} GB` }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="duration_days" label="时长(天)" width="90">
            <template #default="{ row }"><span class="cell-mono">{{ row.duration_days }} 天</span></template>
          </el-table-column>
          <el-table-column label="设备限制" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="row.device_limit ? 'primary' : 'info'">
                {{ row.device_limit ? `${row.device_limit} 台` : '不限' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-switch :model-value="row.enabled" @change="togglePlan(row)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text @click="openEdit(row)"><el-icon><Edit /></el-icon></el-button>
              <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          暂无套餐，点击右上角「新增套餐」
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
                <span style="font-weight: 700">{{ row.name }}</span>
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                  {{ row.enabled ? '已上架' : '已下架' }}
                </el-tag>
              </div>
              <el-switch :model-value="row.enabled" size="small" @change="togglePlan(row)" />
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">套餐价格</span>
                <div class="item-value cell-mono" style="color: #059669; font-weight: 700; font-size: 14px">
                  ¥ {{ (row.price_cents / 100).toFixed(2) }}
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">包含流量</span>
                <div class="item-value cell-mono" style="font-weight: 600">
                  {{ row.traffic_gb >= 1024 ? `${(row.traffic_gb / 1024).toFixed(1)} TB` : `${row.traffic_gb} GB` }}
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">有效周期</span>
                <div class="item-value cell-mono">{{ row.duration_days }} 天</div>
              </div>
              <div class="grid-item">
                <span class="item-label">设备限制</span>
                <div class="item-value">
                  <el-tag size="small" :type="row.device_limit ? 'primary' : 'info'">
                    {{ row.device_limit ? `${row.device_limit} 台` : '不限设备' }}
                  </el-tag>
                </div>
              </div>
            </div>

            <div class="card-foot-actions">
              <el-button size="small" type="primary" plain @click="openEdit(row)">
                <el-icon><Edit /></el-icon>&nbsp;编辑套餐
              </el-button>
              <el-button size="small" type="danger" plain @click="remove(row)">
                <el-icon><Delete /></el-icon>&nbsp;删除
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>

    <el-dialog v-model="formOpen" :title="editing ? '编辑套餐' : '新增套餐'" width="500px">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 月付 200G" /></el-form-item>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="价格（元）">
            <el-input-number v-model="form.price_yuan" :min="0" :step="1" :precision="2" style="width: 100%" />
          </el-form-item>
          <el-form-item label="时长（天）">
            <el-input-number v-model="form.duration_days" :min="1" style="width: 100%" />
          </el-form-item>
          <el-form-item label="流量（GB）">
            <el-input-number v-model="form.traffic_gb" :min="1" style="width: 100%" />
          </el-form-item>
          <el-form-item label="设备限制（台，0=不限）">
            <el-input-number v-model="form.device_limit" :min="0" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="权限组（选填，购买后自动授权组内入站）">
          <el-select v-model="form.permission_group_id" style="width: 100%" clearable placeholder="不绑定">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
</style>