<script setup lang="ts">
import { ref, computed } from 'vue'
import { Plus, Search, Edit, Delete, Share } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { mockServers, type Server } from '@/mock/data'

const keyword = ref('')
const statusFilter = ref('')
const page = ref(1)
const pageSize = 5

const filtered = computed(() =>
  mockServers.filter((s: Server) => {
    const kw = keyword.value.trim().toLowerCase()
    const hitKw = !kw || s.name.toLowerCase().includes(kw) || s.ip.includes(kw) || s.location.includes(kw)
    const hitStatus = !statusFilter.value || s.status === statusFilter.value
    return hitKw && hitStatus
  }),
)

const statusMap: Record<string, { type: 'success' | 'warning' | 'info'; text: string }> = {
  online: { type: 'success', text: '在线' },
  connecting: { type: 'warning', text: '重连中' },
  offline: { type: 'info', text: '离线' },
}
</script>

<template>
  <div class="x-page">
    <el-alert type="info" :closable="false" show-icon title="演示数据：P1 节点通道上线后接入真实服务器" style="margin-bottom: 16px" />
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input v-model="keyword" placeholder="搜索服务器名称 / IP / 地区" :prefix-icon="Search" clearable style="width: 260px" />
        <el-select v-model="statusFilter" placeholder="全部状态" clearable style="width: 130px">
          <el-option label="在线" value="online" />
          <el-option label="重连中" value="connecting" />
          <el-option label="离线" value="offline" />
        </el-select>
      </div>
      <el-button type="primary"><el-icon><Plus /></el-icon>&nbsp;新增服务器</el-button>
    </div>

    <BaseCard>
      <el-table :data="filtered">
        <el-table-column prop="name" label="名称" min-width="120">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
        </el-table-column>
        <el-table-column prop="host" label="主机地址" min-width="170">
          <template #default="{ row }"><code class="cell-mono">{{ row.host }}</code></template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" min-width="130">
          <template #default="{ row }"><code class="cell-mono">{{ row.ip }}</code></template>
        </el-table-column>
        <el-table-column prop="location" label="地区" width="100" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusMap[row.status].type" size="small">
              <span class="x-status-dot" :class="row.status" />{{ statusMap[row.status].text }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="lastSeenAt" label="最后心跳" width="110" />
        <el-table-column prop="remark" label="备注" min-width="100">
          <template #default="{ row }"><span class="muted">{{ row.remark ?? '—' }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default>
            <el-button size="small" text><el-icon><Share /></el-icon></el-button>
            <el-button size="small" text><el-icon><Edit /></el-icon></el-button>
            <el-button size="small" text type="danger"><el-icon><Delete /></el-icon></el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="table-foot">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="filtered.length"
          layout="prev, pager, next"
          small
        />
        <span class="muted">共 {{ filtered.length }} 台</span>
      </div>
    </BaseCard>

    <BaseCard title="服务器 = 一台部署了 Xray-Agent 的机器">
      <p class="muted" style="font-size: 13px">
        每个服务器对应一台物理机 / VPS（systemd 托管 Agent）。节点（接入点）挂在服务器之下，一台服务器可配置多个入站（vmess / vless / trojan / ss 等）。新增服务器时生成 node_id + secret，供一键安装脚本使用。
      </p>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.table-foot { display: flex; align-items: center; justify-content: space-between; padding: 12px 20px; }
@media (max-width: 900px) {
  .cell-mono { font-size: 11.5px; }
}
</style>
