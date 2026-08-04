<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, CopyDocument, Refresh, Ticket } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { createInvitations, getInvitations, type Invitation } from '@/api/admin'
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

const genOpen = ref(false)
const invForm = reactive({ count: 5, expires: '' })
const creating = ref(false)
const generated = ref<string[]>([])

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

async function copyAll() {
  try {
    await navigator.clipboard.writeText(generated.value.join('\n'))
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选择')
  }
}

const statusMap: Record<number, { type: 'success' | 'info' | 'danger'; text: string }> = {
  0: { type: 'success', text: '未使用' },
  1: { type: 'info', text: '已使用' },
  2: { type: 'danger', text: '已禁用' },
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
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <el-button type="primary" @click="genOpen = true"><el-icon><Ticket /></el-icon>&nbsp;生成邀请码</el-button>
    </div>

    <BaseCard>
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="code" label="邀请码" min-width="180">
          <template #default="{ row }"><code class="cell-mono">{{ row.code }}</code></template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusMap[row.status]?.type ?? 'info'" size="small">{{ statusMap[row.status]?.text ?? row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_by" label="创建人" width="80" />
        <el-table-column label="过期时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.expires_at) }}</template>
        </el-table-column>
        <el-table-column label="使用时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.used_at) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
    </BaseCard>

    <el-dialog v-model="genOpen" title="生成邀请码" width="440px">
      <template v-if="!generated.length">
        <el-form label-position="top">
          <el-form-item label="生成数量">
            <el-input-number v-model="invForm.count" :min="1" :max="100" style="width: 100%" />
          </el-form-item>
          <el-form-item label="过期时间（选填，如 2026-12-31T23:59:59+08:00）">
            <el-input v-model="invForm.expires" placeholder="留空 = 永不过期" />
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <p class="muted" style="margin: 0 0 8px">已生成 {{ generated.length }} 个（一次性使用）：</p>
        <div class="inv-codes">
          <code v-for="c in generated" :key="c" class="inv-code">{{ c }}</code>
        </div>
      </template>
      <template #footer>
        <template v-if="!generated.length">
          <el-button @click="genOpen = false">取消</el-button>
          <el-button type="primary" :loading="creating" @click="create">生成</el-button>
        </template>
        <template v-else>
          <el-button @click="genOpen = false; generated = []">完成</el-button>
          <el-button type="primary" @click="copyAll"><el-icon><CopyDocument /></el-icon>&nbsp;复制全部</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.inv-codes { display: grid; gap: 8px; max-height: 260px; overflow: auto; }
.inv-code {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 13px;
  background: var(--x-primary-soft);
  color: var(--x-primary);
  border-radius: 8px;
  padding: 10px 12px;
  word-break: break-all;
}
</style>