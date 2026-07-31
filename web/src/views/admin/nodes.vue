<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Search, Edit, Delete, Lightning } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { mockInbounds, mockServers } from '@/mock/data'

const drawerOpen = ref(false)
const protocolTag: Record<string, string> = { vless: 'vless', vmess: 'vmess', trojan: 'trojan', ss: 'ss' }
const serverOptions = mockServers.map((s) => ({ label: s.name, value: s.id }))

const form = ref({
  serverId: 1,
  protocol: 'vless',
  port: 443,
  account: '',
  network: 'tcp',
  tls: 'reality',
  ratio: 1.0,
  tag: '',
  enabled: true,
})
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input placeholder="搜索入站标签 / 端口" :prefix-icon="Search" clearable style="width: 240px" />
        <el-select placeholder="全部服务器" clearable style="width: 150px">
          <el-option v-for="o in serverOptions" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
        <el-select placeholder="全部协议" clearable style="width: 130px">
          <el-option v-for="p in Object.keys(protocolTag)" :key="p" :label="p" :value="p" />
        </el-select>
      </div>
      <el-button type="primary" @click="drawerOpen = true"><el-icon><Plus /></el-icon>&nbsp;新建入站</el-button>
    </div>

    <BaseCard>
      <el-table :data="mockInbounds">
        <el-table-column prop="tag" label="标签" min-width="130">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
        </el-table-column>
        <el-table-column prop="serverName" label="服务器" width="130" />
        <el-table-column label="协议" width="90">
          <template #default="{ row }"><el-tag size="small">{{ row.protocol }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="port" label="端口" width="80">
          <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
        </el-table-column>
        <el-table-column prop="account" label="账号" min-width="110">
          <template #default="{ row }"><code class="cell-mono">{{ row.account }}</code></template>
        </el-table-column>
        <el-table-column label="传输 / TLS" min-width="110">
          <template #default="{ row }">{{ row.network }} + {{ row.tls }}</template>
        </el-table-column>
        <el-table-column prop="ratio" label="倍率" width="70" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default>
            <el-button size="small" text><el-icon><Edit /></el-icon></el-button>
            <el-button size="small" text type="danger"><el-icon><Delete /></el-icon></el-button>
          </template>
        </el-table-column>
      </el-table>
    </BaseCard>

    <BaseCard title="协议支持（初版范围）">
      <el-tag v-for="p in Object.keys(protocolTag)" :key="p" size="small" style="margin-right: 6px">{{ p }}</el-tag>
      <span class="muted" style="margin-left: 6px">传输层：tcp / ws / grpc；TLS：reality / tls / none（后续版本扩展）</span>
    </BaseCard>

    <!-- 新建入站抽屉 -->
    <el-drawer v-model="drawerOpen" title="新建入站" size="440px">
      <el-form label-position="top">
        <el-form-item label="所属服务器">
          <el-select v-model="form.serverId" style="width: 100%">
            <el-option v-for="o in serverOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="协议">
              <el-select v-model="form.protocol" style="width: 100%">
                <el-option v-for="p in Object.keys(protocolTag)" :key="p" :label="p" :value="p" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="端口"><el-input v-model.number="form.port" /></el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="UUID / 密码">
          <el-input v-model="form.account" placeholder="留空则自动生成" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="传输层">
              <el-select v-model="form.network" style="width: 100%">
                <el-option label="tcp" value="tcp" />
                <el-option label="ws" value="ws" />
                <el-option label="grpc" value="grpc" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="TLS">
              <el-select v-model="form.tls" style="width: 100%">
                <el-option label="reality" value="reality" />
                <el-option label="tls" value="tls" />
                <el-option label="none" value="none" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="流量倍率"><el-input-number v-model="form.ratio" :min="0.1" :step="0.1" style="width: 100%" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="标签"><el-input v-model="form.tag" placeholder="如 Tokyo-VLESS" /></el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <div class="x-pill-note">
          <el-icon><Lightning /></el-icon>
          <span>保存后将向节点 Agent 下发新配置并重启 Xray（约 1~2 秒断线）。</span>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="drawerOpen = false">取消</el-button>
        <el-button type="primary" @click="drawerOpen = false">保存并下发</el-button>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
</style>
