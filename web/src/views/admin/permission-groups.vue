<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Delete, Connection } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  createPermissionGroup,
  deletePermissionGroup,
  getGroupInbounds,
  getInbounds,
  getPermissionGroups,
  setGroupInbounds,
  updatePermissionGroup,
  type InboundItem,
  type PermissionGroup,
} from '@/api/admin'
import { errMsg } from '@/api/http'

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

// ---- 创建/编辑 ----
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

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除权限组「${row.name}」？关联的入站集合一并清除。`, '删除权限组', { type: 'error' })
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

// ---- 入站集合绑定 ----
const bindOpen = ref(false)
const bindGroup = ref<PermissionGroup | null>(null)
const allInbounds = ref<InboundItem[]>([])
const checked = ref<number[]>([])
const bindSaving = ref(false)

async function openBind(row: any) {
  bindGroup.value = row
  bindOpen.value = true
  checked.value = []
  try {
    const [i, g] = await Promise.all([getInbounds(), getGroupInbounds(row.id)])
    if (i.data.code === 0) allInbounds.value = i.data.data.items.filter((x) => x.enabled)
    if (g.data.code === 0) checked.value = g.data.data.inbound_ids
  } catch (e) {
    ElMessage.error(errMsg(e, '加载入站失败'))
  }
}

function inboundLabel(x: InboundItem) {
  return `${x.server_name} / ${x.tag}`
}

async function saveBind() {
  if (!bindGroup.value) return
  bindSaving.value = true
  try {
    const { data } = await setGroupInbounds(bindGroup.value.id, checked.value)
    if (data.code === 0) {
      ElMessage.success(`已保存 ${data.data.count} 个入站（套餐购买后自动授权）`)
      bindOpen.value = false
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    bindSaving.value = false
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增权限组</el-button>
        <span class="muted" style="font-size: 12px">套餐绑定权限组后，购买该套餐的用户自动获得组内全部 type=user 入站（订阅动态授权）</span>
      </div>
    </div>

    <BaseCard>
      <el-table v-loading="loading" :data="list" size="small">
        <el-table-column prop="name" label="名称" min-width="140">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="180" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="openBind(row)">
              <el-icon><Connection /></el-icon>&nbsp;入站集合
            </el-button>
            <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
          </template>
        </el-table-column>
        <template #empty><div class="table-empty">尚无权限组，点击右上角「新增权限组」</div></template>
      </el-table>
    </BaseCard>

    <el-dialog v-model="formOpen" :title="editing ? '编辑权限组' : '新增权限组'" width="440px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 基础套餐组" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" placeholder="选填" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="bindOpen" :title="`入站集合 — ${bindGroup?.name ?? ''}`" width="560px" :append-to-body="true">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 10px"
        title="勾选用户可访问的入站（type=user 生效，relay/idle 自动排除）" />
      <el-checkbox-group v-model="checked" class="inbound-checks">
        <el-checkbox v-for="x in allInbounds" :key="x.id" :value="x.id">{{ inboundLabel(x) }}</el-checkbox>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="bindOpen = false">取消</el-button>
        <el-button type="primary" :loading="bindSaving" @click="saveBind">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
.inbound-checks {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 320px;
  overflow-y: auto;
}
.table-empty { padding: 30px 0; text-align: center; color: var(--x-text-3); font-size: 13px; }
</style>
