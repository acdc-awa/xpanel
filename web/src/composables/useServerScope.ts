import { computed, ref, type Ref } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { getInbounds, getServers, type InboundItem, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'
import { ElMessage } from '@/components/cloudscape'

export interface ServerScopeOptions {
  /**
   * 没有已选服务器时是否自动选中第一台。
   * 出站页/路由页按单台服务器取数，需要自动选中；接入点页要保留「全部服务器」语义，传 false。
   */
  autoSelectFirst?: boolean
}

/**
 * 按服务器维度的页面共用状态：服务器列表、当前选择，以及 URL 上的 server_id 同步。
 *
 * 出站页与路由页都按 server_id 取数，但两页各自还有 view/tab/create 等页面级参数，
 * 所以同步时只改 server_id，其余 query 原样保留。
 */
export function useServerScope(options: ServerScopeOptions = {}) {
  const route = useRoute()
  const router = useRouter()

  const servers = ref<ServerItem[]>([])
  const serverFilter = ref<number | undefined>(undefined)
  const serverLoading = ref(false)
  const currentServer = computed(() => servers.value.find((s) => s.id === serverFilter.value) ?? null)

  async function loadServers() {
    serverLoading.value = true
    try {
      const { data } = await getServers()
      if (data.code === 0) {
        servers.value = data.data.items
        // 已选服务器被移除时回落
        if (serverFilter.value && !servers.value.some((s) => s.id === serverFilter.value)) {
          serverFilter.value = undefined
        }
        if (!serverFilter.value && options.autoSelectFirst !== false) {
          serverFilter.value = servers.value[0]?.id
        }
      } else {
        ElMessage.error(data.message)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '加载服务器失败'))
    } finally {
      serverLoading.value = false
    }
  }

  function syncServerQuery() {
    const query: LocationQueryRaw = { ...route.query }
    delete query.server_id
    if (serverFilter.value) query.server_id = String(serverFilter.value)
    router.replace({ query })
  }

  return { servers, serverFilter, serverLoading, currentServer, loadServers, syncServerQuery }
}

/**
 * 当前服务器的入站列表与 tag 映射。
 *
 * - inboundTags 给路由规则编辑器的「入站来源」下拉用；
 * - ownerTagOf 给直通派生行显示归属（owner_inbound_id → 直通入站 tag）。
 */
export function useServerInbounds(serverFilter: Ref<number | undefined>) {
  const serverInbounds = ref<InboundItem[]>([])
  const inboundTags = ref<string[]>([])
  const ownerTagOf = computed<Record<number, string>>(() => {
    const m: Record<number, string> = {}
    for (const i of serverInbounds.value) m[Number(i.id)] = i.tag
    return m
  })

  async function loadServerInbounds() {
    if (!serverFilter.value) {
      serverInbounds.value = []
      inboundTags.value = []
      return
    }
    try {
      const { data } = await getInbounds(serverFilter.value)
      if (data.code === 0) {
        serverInbounds.value = data.data.items
        inboundTags.value = data.data.items.map((i) => i.tag)
      }
    } catch {
      /* 标签加载失败不阻塞页面主数据 */
    }
  }

  return { serverInbounds, inboundTags, ownerTagOf, loadServerInbounds }
}
