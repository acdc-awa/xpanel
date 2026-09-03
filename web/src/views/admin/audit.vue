<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh, Loading, DocumentCopy, Search } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getAuditLogs, type AuditLog } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

const category = ref('')
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await getAuditLogs(page.value, size.value, {
      category: category.value || undefined,
      keyword: keyword.value.trim() || undefined,
    })
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载审计日志失败'))
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  page.value = 1
  load()
}

function resetFilters() {
  category.value = ''
  keyword.value = ''
  page.value = 1
  load()
}

onMounted(load)

function fmtTime(t: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

interface ActionMeta {
  categoryName: string
  categoryColor: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  title: string
}

function getActionMeta(action: string, detail = ''): ActionMeta {
  const act = action || ''
  const d = detail || ''

  // 1. 节点管理
  if (act.startsWith('servers')) {
    let title = '节点管理'
    if (act === 'servers') {
      title = '创建节点'
    } else if (act.endsWith('.command')) {
      if (d.includes('push_config')) title = '推送节点配置'
      else if (d.includes('restart_xray')) title = '重启节点 Xray'
      else if (d.includes('upgrade_agent')) title = '升级节点 Agent'
      else if (d.includes('get_status')) title = '查询节点状态'
      else if (d.includes('get_logs')) title = '查看节点日志'
      else title = '执行节点指令'
    } else if (act.endsWith('.reset-secret')) {
      title = '重置节点密钥'
    } else if (act.endsWith('.generate-config')) {
      title = '重新生成节点配置'
    } else if (act.includes('.outbounds')) {
      title = d.includes('DELETE') ? '删除节点出站' : '配置节点出站'
    } else if (act.includes('.routing')) {
      title = d.includes('DELETE') ? '删除节点分流' : '配置节点分流'
    } else if (act.includes('.layers')) {
      title = d.includes('DELETE') ? '删除分层规则' : '配置分层规则'
    } else if (d.includes('DELETE')) {
      title = '删除节点'
    } else {
      title = '修改节点信息'
    }
    return { categoryName: '节点', categoryColor: 'primary', title }
  }

  // 2. 用户管理
  if (act.startsWith('users') || act.startsWith('invitations')) {
    let title = '用户管理'
    if (act === 'users') {
      title = '创建新用户'
    } else if (act.endsWith('.toggle')) {
      title = '启停用户账号'
    } else if (act.endsWith('.2fa.disable')) {
      title = '关闭双重验证(2FA)'
    } else if (act.endsWith('.reset-traffic')) {
      title = '重置用户流量'
    } else if (act.endsWith('.balance')) {
      title = '调整用户余额'
    } else if (act.startsWith('invitations')) {
      title = d.includes('DELETE') ? '作废邀请码' : '批量生成邀请码'
    } else if (d.includes('DELETE')) {
      title = '删除用户账号'
    } else {
      title = '修改用户信息'
    }
    return { categoryName: '用户', categoryColor: 'success', title }
  }

  // 3. 财务与套餐
  if (act.startsWith('plans') || act.startsWith('orders') || act.startsWith('gift-cards') || act.startsWith('billing')) {
    let title = '财务套餐'
    if (act.startsWith('plans')) {
      title = act === 'plans' ? '创建套餐' : (d.includes('DELETE') ? '删除套餐' : '修改套餐')
    } else if (act.startsWith('orders')) {
      if (d.includes('confirm') || act.includes('confirm')) title = '确认订单'
      else if (d.includes('cancel') || act.includes('cancel')) title = '取消订单'
      else title = '订单处理'
    } else if (act.startsWith('gift-cards')) {
      title = d.includes('DELETE') ? '删除礼品卡' : '批量生成礼品卡'
    }
    return { categoryName: '财务', categoryColor: 'warning', title }
  }

  // 4. 入站与证书
  if (act.startsWith('inbounds') || act.startsWith('certs') || act.startsWith('access-points') || act.startsWith('permission-groups')) {
    let title = '入站证书'
    if (act.startsWith('inbounds')) {
      if (act.endsWith('.toggle')) title = '启停入站端口'
      else if (act.includes('setup-internal') || act.includes('rotate-internal')) title = '轮转内部中继账户'
      else if (d.includes('DELETE')) title = '删除入站端口'
      else title = act === 'inbounds' ? '新建入站端口' : '修改入站端口'
    } else if (act.startsWith('certs')) {
      if (act.includes('self-signed')) title = '签发自签名证书'
      else title = d.includes('DELETE') ? '删除 TLS 证书' : '上传/配置 TLS 证书'
    } else if (act.startsWith('access-points')) {
      title = d.includes('DELETE') ? '删除接入点' : '配置自定义接入点'
    } else if (act.startsWith('permission-groups')) {
      title = d.includes('DELETE') ? '删除权限组' : '配置用户权限组'
    }
    return { categoryName: '协议证书', categoryColor: 'info', title }
  }

  // 5. 系统设置与运维
  if (act.startsWith('settings') || act.startsWith('notices') || act.startsWith('backup') || act.startsWith('topology')) {
    let title = '系统设置'
    if (act.startsWith('settings')) title = '修改全局系统设置'
    else if (act.startsWith('notices')) title = d.includes('DELETE') ? '删除系统公告' : '发布/编辑系统公告'
    else if (act.startsWith('backup')) title = '创建系统数据备份'
    else if (act.startsWith('topology')) title = '保存拓扑结构布局'
    return { categoryName: '系统', categoryColor: 'danger', title }
  }

  // 6. 认证登录
  if (act.startsWith('auth')) {
    return {
      categoryName: '认证',
      categoryColor: 'info',
      title: act === 'auth.login' ? '账号登录' : '身份认证操作',
    }
  }

  return { categoryName: '操作', categoryColor: 'info', title: act }
}

const detailModalOpen = ref(false)
const currentDetail = ref('')
function viewDetail(detail: string) {
  if (!detail) return
  currentDetail.value = detail
  detailModalOpen.value = true
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left" style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap;">
        <el-select v-model="category" placeholder="全部分类" clearable style="width: 130px" @change="onFilterChange">
          <el-option label="全部分类" value="" />
          <el-option label="节点管理" value="servers" />
          <el-option label="用户管理" value="users" />
          <el-option label="财务套餐" value="billing" />
          <el-option label="入站证书" value="inbounds" />
          <el-option label="系统设置" value="settings" />
          <el-option label="身份认证" value="auth" />
        </el-select>
        <el-input
          v-model="keyword"
          placeholder="搜索操作内容、IP、路由..."
          clearable
          style="width: 240px"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button type="primary" @click="onFilterChange"><el-icon><Search /></el-icon>&nbsp;查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard title="系统审计日志">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><DocumentCopy /></el-icon>
        <p style="margin-top: 8px">暂无审计日志</p>
      </div>

      <!-- 全局统一审计日志卡片网格流 (自适应 1~4 列) -->
      <div v-else class="audit-card-grid">
        <div v-for="row in list" :key="row.id" class="audit-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="x-chip" :class="row.operator_type === 'admin' ? 'orange' : 'blue'" style="font-size: 10px; padding: 1px 5px">
                {{ row.operator_type === 'admin' ? '管理员' : '用户' }} #{{ row.operator_id }}
              </span>
              <el-tag :type="getActionMeta(row.action, row.detail).categoryColor" size="small" effect="plain" style="font-size: 11px">
                {{ getActionMeta(row.action, row.detail).categoryName }}
              </el-tag>
              <span class="action-name">{{ getActionMeta(row.action, row.detail).title }}</span>
            </div>
            <code class="cell-mono muted" style="font-size: 11px">{{ row.ip || '—' }}</code>
          </div>

          <!-- 动作原始技术标识 -->
          <div class="action-code-bar">
            <span class="action-code cell-mono" :title="row.action">{{ row.action }}</span>
          </div>

          <!-- 属性网格与日志详情 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 2px">
                <span class="item-label">操作详情</span>
                <el-button v-if="row.detail && row.detail.length > 40" type="primary" link size="small" style="font-size: 11px; padding: 0" @click="viewDetail(row.detail)">
                  查看完整
                </el-button>
              </div>
              <div
                class="item-value audit-detail-clamp"
                :title="row.detail"
                @click="viewDetail(row.detail)"
              >
                {{ row.detail || '—' }}
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">记录时间</span>
              <div class="item-value cell-mono muted font-11">{{ fmtTime(row.created_at) }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>

    <!-- 详情弹窗 -->
    <el-dialog v-model="detailModalOpen" title="操作详情" width="520px" append-to-body>
      <div v-if="currentDetail" class="audit-modal-body">
        <pre class="audit-pre">{{ currentDetail }}</pre>
      </div>
      <template #footer>
        <el-button @click="detailModalOpen = false">关 闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: var(--x-font-mono); font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }

/* ================= 全局统一审计日志卡片网格流 ================= */
.audit-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.audit-card {
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

    .action-name {
      font-weight: 600;
      font-size: 13px;
      color: var(--x-text, #111827);
    }
  }

  .action-code-bar {
    padding: 6px 0 2px;
    .action-code {
      font-size: 11px;
      color: var(--x-text-3);
      background: var(--x-fill-2, #f8fafc);
      padding: 2px 6px;
      border-radius: 4px;
      border: 1px solid var(--x-border-light, #f1f5f9);
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0 2px;

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
}

.x-pager {
  display: flex;
  justify-content: flex-end;
  padding: 14px 0 0;
  margin-top: 16px;
  border-top: 1px solid var(--x-border-light, #f1f5f9);
}
.audit-detail-clamp {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
  line-height: 1.45;
  color: var(--x-text-2);
  cursor: pointer;
  background: var(--x-bg, #f9fafb);
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--x-border-light, #f3f4f6);
  font-size: 12px;
  &:hover {
    color: var(--x-primary);
  }
}
.audit-pre {
  background: var(--x-fill-2, #f1f5f9);
  padding: 12px 14px;
  border-radius: 8px;
  font-family: var(--x-font-mono, monospace);
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--x-text);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 380px;
  overflow-y: auto;
  margin: 0;
}

@media (max-width: 640px) {
  .audit-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>