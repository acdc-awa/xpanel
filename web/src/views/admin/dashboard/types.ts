import type { Ref } from 'vue'
import type { DashboardData, ServerMatrixItem } from '@/api/types'
import type { RankPeriod } from '@/api/admin'

export interface DashboardContext {
  dashData: Ref<DashboardData | null>
  loading: Ref<boolean>
  pollFailed: Ref<boolean>
  useMockData: Ref<boolean>
  trendRange: Ref<number>
  rankPeriod: Ref<RankPeriod>
  openServerMetrics: (s: ServerMatrixItem) => void
  openOnlineUsers: (s: ServerMatrixItem) => void
  refreshDashboard: () => Promise<void>
}
