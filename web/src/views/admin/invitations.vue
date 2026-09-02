<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { CopyDocument, Refresh, Ticket, Link, Delete } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
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

    <BaseCard title="邀请注册码列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Ticket /></el-icon>
        <p style="margin-top: 8px">暂无邀请码，点击右上角「生成邀请码」</p>
      </div>

      <!-- 全局统一邀请码卡片网格流 (自适应 1~4 列) -->
      <div v-else class="inv-card-grid">
        <div v-for="row in list" :key="row.id" class="inv-card" :class="{ disabled: row.status !== 0 }">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <code
                class="cell-mono inv-code-text"
                title="点击复制邀请码"
                @click="copyCode(row.code)"
              >
                {{ row.code }}
              </code>
            </div>
            <span
              class="x-chip"
              :class="row.status === 0 ? 'blue' : (row.status === 1 ? 'green' : 'red')"
              style="font-size: 10.5px"
            >
              {{ statusMap[row.status]?.text ?? row.status }}
            </span>
          </div>

          <!-- 邀请码属性网格 -->
          <div class="card-grid">
            <div class="grid-item">
              <span class="item-label">创建人</span>
              <div class="item-value">{{ row.created_by || '系统生成' }}</div>
            </div>
            <div class="grid-item">
              <span class="item-label">有效期</span>
              <div class="item-value cell-mono font-11">
                {{ row.expires_at ? '至 ' + fmtTime(row.expires_at) : '永不过期' }}
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">创建时间</span>
              <div class="item-value cell-mono muted font-11">{{ fmtTime(row.created_at) }}</div>
            </div>
            <div class="grid-item">
              <span class="item-label">核销时间</span>
              <div class="item-value cell-mono font-11" :style="{ color: row.used_at ? '#10b981' : 'var(--x-text-3)' }">
                {{ row.used_at ? fmtTime(row.used_at) : '待核销' }}
              </div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <template v-if="row.status === 0">
              <el-button size="small" type="primary" plain @click="copyRegisterLink(row.code)">
                <el-icon><Link /></el-icon>&nbsp;复制链接
              </el-button>
              <el-button size="small" type="default" plain @click="copyCode(row.code)">
                <el-icon><CopyDocument /></el-icon>&nbsp;复制码
              </el-button>
              <el-button size="small" type="danger" plain @click="revoke(row)">
                <el-icon><Delete /></el-icon>&nbsp;作废
              </el-button>
            </template>
            <span v-else class="muted font-11" style="line-height: 30px">
              {{ row.status === 1 ? '该邀请码已被成功核销' : '该邀请码已被作废禁用' }}
            </span>
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

/* ================= 全局统一邀请码卡片网格流 ================= */
.inv-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.inv-card {
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

  &.disabled {
    opacity: 0.75;
    background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
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

    .inv-code-text {
      font-weight: 700;
      font-size: 13px;
      color: var(--x-primary);
      cursor: pointer;
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

@media (max-width: 640px) {
  .inv-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>