<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Edit, Delete, Refresh } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { createPlan, deletePlan, getPlans, updatePlan, type Plan } from '@/api/admin'
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

const formOpen = ref(false)
const editing = ref(false)
const form = reactive({ id: 0, name: '', price_cents: 2500, traffic_gb: 200, duration_days: 30, speed_limit_kbps: 0 })
const saving = ref(false)

function openCreate() {
  editing.value = false
  Object.assign(form, { id: 0, name: '', price_cents: 2500, traffic_gb: 200, duration_days: 30, speed_limit_kbps: 0 })
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = true
  Object.assign(form, {
    id: row.id, name: row.name, price_cents: row.price_cents, traffic_gb: row.traffic_gb,
    duration_days: row.duration_days, speed_limit_kbps: row.speed_limit_kbps,
  })
  formOpen.value = true
}

async function save() {
  if (!form.name || form.price_cents < 0 || form.traffic_gb < 1 || form.duration_days < 1) {
    ElMessage.warning('请填写完整套餐信息')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name, price_cents: form.price_cents, traffic_gb: form.traffic_gb,
      duration_days: form.duration_days, speed_limit_kbps: form.speed_limit_kbps,
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
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="id" label="ID" width="60">
          <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="130">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
        </el-table-column>
        <el-table-column label="价格" width="100">
          <template #default="{ row }">¥ {{ (row.price_cents / 100).toFixed(2) }}</template>
        </el-table-column>
        <el-table-column label="流量" width="120">
          <template #default="{ row }">
            {{ row.traffic_gb >= 1024 ? `${(row.traffic_gb / 1024).toFixed(1)} TB` : `${row.traffic_gb} GB` }}
          </template>
        </el-table-column>
        <el-table-column prop="duration_days" label="时长(天)" width="100" />
        <el-table-column label="限速" width="110">
          <template #default="{ row }">{{ row.speed_limit_kbps ? `${(row.speed_limit_kbps / 1000).toFixed(1)} Mbps` : '不限' }}</template>
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
    </BaseCard>

    <el-dialog v-model="formOpen" :title="editing ? '编辑套餐' : '新增套餐'" width="480px">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 月付 200G" /></el-form-item>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="价格（元）">
            <el-input-number v-model="form.price_cents" :min="0" :step="100" :precision="0" style="width: 100%" />
          </el-form-item>
          <el-form-item label="时长（天）">
            <el-input-number v-model="form.duration_days" :min="1" style="width: 100%" />
          </el-form-item>
          <el-form-item label="流量（GB）">
            <el-input-number v-model="form.traffic_gb" :min="1" style="width: 100%" />
          </el-form-item>
          <el-form-item label="限速（kbps，0=不限）">
            <el-input-number v-model="form.speed_limit_kbps" :min="0" :step="1000" style="width: 100%" />
          </el-form-item>
        </div>
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