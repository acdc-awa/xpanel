<script setup lang="ts">
// 出站管理：从路由页拆出的独立页面（2026-10-22）。
// 只负责出站对象本身：列表、新建、编辑、启停、删除。
// 默认兜底出口与路由 DomainStrategy 属于全局路由策略，仍留在路由页。
// 四层直通的派生出口由通道/入站生命周期自动维护，本页只读展示。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import {
  CsBadge,
  CsButton,
  CsContainer,
  CsInput,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsTable,
  CsToggle,
  CsToolbar,
  CsTooltip,
  toast,
  type BadgeColor,
  type TableColumn,
} from '@/components/cloudscape'
import OutboundConfigEditor from './servers/OutboundConfigEditor.vue'
import { useServerInbounds, useServerScope } from '@/composables/useServerScope'
import {
  getServerOutbounds,
  updateServerOutbound,
  type ServerOutbound,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { isReservedOutboundTag } from '@/utils/outbound'

const route = useRoute()
const router = useRouter()

const { servers, serverFilter, serverLoading, loadServers, syncServerQuery } = useServerScope()
const { ownerTagOf, loadServerInbounds } = useServerInbounds(serverFilter)

const serverOptions = computed(() =>
  servers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id })),
)

const protocolFilterOptions = [
  { label: '全部协议', value: '' },
  { label: '待连线', value: 'draft' },
  { label: '直连 (freedom)', value: 'freedom' },
  { label: 'VLESS 链', value: 'vless' },
  { label: 'Hysteria 2', value: 'hysteria' },
  { label: 'SOCKS5', value: 'socks' },
  { label: 'HTTP(S)', value: 'http' },
  { label: '黑洞 (blackhole)', value: 'blackhole' },
  { label: '原生 JSON', value: 'custom' },
]

const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '已启用', value: 'enabled' },
  { label: '已停用', value: 'disabled' },
]

const outbounds = ref<ServerOutbound[]>([])
const loading = ref(false)
const editorOpen = ref(false)
const editing = ref<ServerOutbound | null>(null)

const keyword = ref('')
const protocolFilter = ref('')
const statusFilter = ref('')
const page = ref(1)
const pageSize = ref(10)

type EffectiveOutbound = ServerOutbound & { ownerInboundTag?: string }

const displayOutbounds = computed<EffectiveOutbound[]>(() =>
  outbounds.value.map((o) => ({
    ...o,
    ownerInboundTag: o.owner_inbound_id ? ownerTagOf.value[o.owner_inbound_id] || '' : '',
  })),
)

// 待连线：没有任何引用目标、也没有载荷（基础 settings / 自定义 JSON）。
// freedom 也要算：以「引用入站」形态保存但还没选目标时，行协议仍是 freedom 且载荷为空。
function isDraftOutbound(row: EffectiveOutbound): boolean {
  if (row.owner_inbound_id || row.inbound_ref || row.outbound_json) return false
  if (row.settings_json || row.stream_settings_json) return false
  return row.protocol !== 'blackhole'
}

const hasFilters = computed(() =>
  Boolean(keyword.value.trim() || protocolFilter.value || statusFilter.value),
)

function resetFilters() {
  keyword.value = ''
  protocolFilter.value = ''
  statusFilter.value = ''
  page.value = 1
}

const filteredOutbounds = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return displayOutbounds.value.filter((row) => {
    if (protocolFilter.value === 'draft') {
      if (!isDraftOutbound(row)) return false
    } else if (protocolFilter.value) {
      if (isDraftOutbound(row) || row.protocol !== protocolFilter.value) return false
    }
    if (statusFilter.value === 'enabled' && !row.enabled) return false
    if (statusFilter.value === 'disabled' && row.enabled) return false
    if (kw) {
      const hitTag = (row.tag || '').toLowerCase().includes(kw)
      const hitRemark = (row.remark || '').toLowerCase().includes(kw)
      if (!hitTag && !hitRemark) return false
    }
    return true
  })
})

watch([keyword, protocolFilter, statusFilter, serverFilter], () => {
  page.value = 1
})

watch([() => filteredOutbounds.value.length, pageSize], ([len, size]) => {
  const maxPage = Math.max(1, Math.ceil(len / Math.max(1, size)))
  if (page.value > maxPage) page.value = maxPage
})

const pagedOutbounds = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredOutbounds.value.slice(start, start + pageSize.value)
})

const emptyText = computed(() => {
  if (!serverFilter.value) return '请先选择目标服务器'
  if (hasFilters.value) return '未找到匹配当前筛选的出站通道'
  return '该服务器暂无出站通道，点击右上角「新增出站」'
})

const outboundColumns: TableColumn[] = [
  { id: 'tag', header: '名称' },
  { id: 'protocol', header: '协议' },
  { id: 'priority', header: '优先级' },
  { id: 'remark', header: '备注' },
  { id: 'status', header: '状态' },
  { id: 'actions', header: '操作', width: '1%' },
]

async function loadOutbounds() {
  if (!serverFilter.value) {
    outbounds.value = []
    return
  }
  loading.value = true
  try {
    const { data } = await getServerOutbounds(serverFilter.value)
    if (data.code === 0) outbounds.value = data.data.items
    else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载出站失败'))
  } finally {
    loading.value = false
  }
}

async function reloadList() {
  await Promise.all([loadOutbounds(), loadServerInbounds()])
}

async function refreshAll() {
  await loadServers()
  await reloadList()
}

function openCreate() {
  editing.value = null
  editorOpen.value = true
}

function openEdit(row: EffectiveOutbound) {
  if (row.owner_inbound_id) return
  editing.value = row
  editorOpen.value = true
}

async function toggleOutbound(row: EffectiveOutbound) {
  if (!serverFilter.value) return
  try {
    const { data } = await updateServerOutbound(serverFilter.value, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      toast.success(data.data.outbound.enabled ? '已启用' : '已停用')
      loadOutbounds()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '操作失败'))
  }
}

async function onOutboundDeleted() {
  // 删除默认出口时后端会回退到 direct，顺带刷新服务器以更新关联状态；
  // 引用该出站 tag 的路由规则由后端级联删除，本页无需感知规则列表。
  await Promise.all([loadOutbounds(), loadServers(), loadServerInbounds()])
}

function protocolBadge(row: EffectiveOutbound): { text: string; color: BadgeColor } {
  if (isDraftOutbound(row)) return { text: '待连线', color: 'severity-medium' }
  switch (row.protocol) {
    case 'freedom':
      return { text: '直连 (freedom)', color: 'green' }
    case 'vless':
      return { text: 'VLESS 链', color: 'blue' }
    case 'hysteria':
      return { text: 'Hysteria 2', color: 'blue' }
    case 'socks':
      return { text: 'SOCKS5', color: 'severity-medium' }
    case 'http':
      return { text: 'HTTP(S)', color: 'severity-neutral' }
    case 'blackhole':
      return { text: '黑洞 (blackhole)', color: 'red' }
    case 'custom':
      return { text: '原生 JSON', color: 'severity-medium' }
    default:
      return { text: row.protocol, color: 'grey' }
  }
}

function derivedTooltip(row: EffectiveOutbound): string {
  return `随四层直通「${row.ownerInboundTag || '#' + row.owner_inbound_id}」自动维护，目标在入站页修改`
}

function statusTooltip(row: EffectiveOutbound): string {
  if (row.owner_inbound_id) {
    return `随四层直通「${row.ownerInboundTag || '#' + row.owner_inbound_id}」自动维护，不可单独启停`
  }
  if (isReservedOutboundTag(row.tag)) {
    return '系统内置出站不可停用'
  }
  return row.enabled ? '已启用，点击停用' : '已停用，点击启用'
}

let initialized = false
watch(serverFilter, (v) => {
  if (!initialized) return
  syncServerQuery()
  if (!v) {
    outbounds.value = []
    return
  }
  loadOutbounds()
  loadServerInbounds()
})

onMounted(async () => {
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  await loadServers()
  await reloadList()
  initialized = true
  syncServerQuery()

  if (route.query.create === '1') {
    openCreate()
    const query: LocationQueryRaw = { ...route.query }
    delete query.create
    router.replace({ query })
  }
})
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect
        v-model="serverFilter"
        :options="serverOptions"
        :loading="serverLoading"
        filterable
        placeholder="选择目标服务器"
        aria-label="选择目标服务器"
        class="filter-select filter-select--server"
      />
      <CsInput
        v-model="keyword"
        placeholder="搜索出站标识 / 备注"
        clearable
        prefix-icon="search"
        class="filter-search"
      />
      <CsSelect
        v-model="protocolFilter"
        :options="protocolFilterOptions"
        clearable
        placeholder="全部协议"
        aria-label="按协议筛选出站"
        class="filter-select filter-select--md"
      />
      <CsSelect
        v-model="statusFilter"
        :options="statusFilterOptions"
        clearable
        placeholder="全部状态"
        aria-label="按状态筛选出站"
        class="filter-select filter-select--sm"
      />
      <CsButton v-if="hasFilters" @click="resetFilters">重置</CsButton>
      <CsButton icon-name="refresh" @click="refreshAll">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" :disabled="!serverFilter" @click="openCreate">
          新增出站
        </CsButton>
      </template>
    </CsToolbar>

    <CsContainer header="出站通道">
      <CsTable
        compact
        :wrap-lines="false"
        track-by="id"
        :items="pagedOutbounds"
        :columns="outboundColumns"
        :loading="loading"
        loading-text="加载中…"
        :empty-text="emptyText"
      >
        <template #cell-tag="{ row }">
          <button
            v-if="!row.owner_inbound_id"
            type="button"
            class="name-link-btn cell-mono"
            :class="{ 'is-muted': !row.enabled }"
            title="点击编辑出站"
            @click="openEdit(row)"
          >
            {{ row.tag }}
          </button>
          <span
            v-else
            class="name-readonly cell-mono"
            :class="{ 'is-muted': !row.enabled }"
            :title="derivedTooltip(row)"
          >
            {{ row.tag }}
          </span>
        </template>

        <template #cell-protocol="{ row }">
          <CsBadge class="nowrap-badge" :color="protocolBadge(row).color">
            {{ protocolBadge(row).text }}
          </CsBadge>
        </template>

        <template #cell-priority="{ row }">
          <span class="cell-mono" :class="{ 'cell-muted': !row.enabled }">
            {{ row.priority ?? 0 }}
          </span>
        </template>

        <template #cell-remark="{ row }">
          <span
            v-if="row.remark"
            class="row-remark-cell"
            :class="{ 'cell-muted': !row.enabled }"
            :title="row.remark"
          >
            {{ row.remark }}
          </span>
          <span v-else class="cell-muted">无</span>
        </template>

        <template #cell-status="{ row }">
          <CsTooltip :content="statusTooltip(row)">
            <CsToggle
              :model-value="row.enabled"
              :disabled="isReservedOutboundTag(row.tag) || !!row.owner_inbound_id"
              :aria-label="`启用出站 ${row.tag}`"
              @change="toggleOutbound(row)"
            />
          </CsTooltip>
        </template>

        <template #cell-actions="{ row }">
          <CsTooltip v-if="row.owner_inbound_id" :content="derivedTooltip(row)">
            <CsButton :wrap-text="false" disabled>编辑</CsButton>
          </CsTooltip>
          <CsButton v-else :wrap-text="false" @click="openEdit(row)">编辑</CsButton>
        </template>
      </CsTable>

      <template #footer>
        <CsPaginationBar v-if="filteredOutbounds.length > 0">
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="filteredOutbounds.length"
            :page-sizes="[10, 20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @size-change="page = 1"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <OutboundConfigEditor
      v-if="editorOpen"
      :server-id="serverFilter ?? 0"
      :outbound="editing"
      @saved="loadOutbounds"
      @deleted="onOutboundDeleted"
      @close="editorOpen = false"
    />
  </CsPage>
</template>

<style scoped lang="scss">
.filter-search {
  width: 220px;
}

.filter-select--server {
  width: 240px;
}

.filter-select--md {
  width: 155px;
}

.filter-select--sm {
  width: 115px;
}

.name-link-btn,
.name-readonly {
  display: inline-block;
  max-width: 280px;
  vertical-align: middle;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &.is-muted {
    color: var(--color-text-status-inactive-fqjc97, #656871);
  }
}

.name-link-btn {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  text-align: start;
  cursor: pointer;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
    text-decoration: underline;
  }
}

.row-remark-cell {
  display: inline-block;
  max-width: 260px;
  vertical-align: middle;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

@media (max-width: $bp-tablet-max) {
  .cs-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-xs-m9b7jf, 8px);
  }

  :deep(.cs-toolbar-main) {
    inline-size: 100%;

    .filter-search {
      inline-size: 100% !important;
    }

    .filter-select {
      flex: 1 1 calc(50% - 4px);
      min-inline-size: 120px;
      inline-size: auto;
    }
  }

  :deep(.cs-toolbar-actions) {
    display: flex;
    flex-wrap: wrap;
    inline-size: 100%;
    gap: 6px;
  }
}
</style>
