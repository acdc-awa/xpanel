<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { CopyDocument, Refresh, Ticket } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { createInvitations, getInvitations, revokeInvitation, type Invitation } from '@/api/admin'
import { errMsg } from '@/api/http'
import { withBase } from '@/config/site'

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

function getRegisterUrl(code: string) {
  return `${location.origin}${withBase('/register')}?code=${encodeURIComponent(code)}`
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
    await ElMessageBox.confirm(`确认作废邀请码 ${row.code}？作废后无法注册。`, '作废邀请码', {
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
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="code" label="邀请码" min-width="180">
            <template #default="{ row }"><code class="cell-mono">{{ row.code }}</code></template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <span v-if="row.status === 0" class="x-chip blue">未使用</span>
              <span v-else-if="row.status === 1" class="x-chip green">已使用</span>
              <span v-else class="x-chip red">已禁用</span>
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
          <el-table-column label="操作" width="160" fixed="right">
            <template #default="{ row }">
              <template v-if="row.status === 0">
                <el-button link type="primary" @click="copyRegisterLink(row.code)">复制链接</el-button>
                <el-button link type="default" @click="copyCode(row.code)">复制码</el-button>
                <el-button link type="danger" @click="revoke(row)">作废</el-button>
              </template>
              <span v-else class="muted">—</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          暂无邀请码，点击右上角「生成邀请码」
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <code class="cell-mono" style="font-weight: 700; color: var(--x-primary); font-size: 13px">{{ row.code }}</code>
              </div>
              <el-tag :type="statusMap[row.status]?.type ?? 'info'" size="small">
                {{ statusMap[row.status]?.text ?? row.status }}
              </el-tag>
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">创建人</span>
                <div class="item-value">{{ row.created_by || '系统' }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">过期时间</span>
                <div class="item-value cell-mono" style="font-size: 11.5px">{{ fmtTime(row.expires_at) }}</div>
              </div>
              <div v-if="row.used_at" class="grid-item full-width">
                <span class="item-label">使用时间</span>
                <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ fmtTime(row.used_at) }}</div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">创建时间</span>
                <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ fmtTime(row.created_at) }}</div>
              </div>
            </div>

            <div v-if="row.status === 0" class="card-foot-actions" style="display: flex; gap: 8px; flex-wrap: wrap">
              <el-button size="small" type="primary" plain @click="copyRegisterLink(row.code)">
                复制注册链接
              </el-button>
              <el-button size="small" type="default" plain @click="copyCode(row.code)">
                复制邀请码
              </el-button>
              <el-button size="small" type="danger" plain @click="revoke(row)">
                作废
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>

    <el-dialog v-model="genOpen" title="生成邀请码" width="480px">
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
          <div v-for="c in generated" :key="c" class="inv-code-row">
            <code class="inv-code">{{ c }}</code>
            <el-button link type="primary" size="small" @click="copyRegisterLink(c)">复制链接</el-button>
          </div>
        </div>
      </template>
      <template #footer>
        <template v-if="!generated.length">
          <el-button @click="genOpen = false">取消</el-button>
          <el-button type="primary" :loading="creating" @click="create">生成</el-button>
        </template>
        <template v-else>
          <el-button @click="genOpen = false; generated = []">完成</el-button>
          <el-button @click="copyAll"><el-icon><CopyDocument /></el-icon>&nbsp;复制全部邀请码</el-button>
          <el-button type="primary" @click="copyAllLinks"><el-icon><CopyDocument /></el-icon>&nbsp;复制全部链接</el-button>
        </template>
      </template>
    </el-dialog>
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
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 13px;
  color: var(--x-primary);
  word-break: break-all;
}
</style>