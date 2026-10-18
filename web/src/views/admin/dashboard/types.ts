import type { Ref } from 'vue'
import type { DashboardData, ServerMatrixItem } from '@/api/types'
import type { RankPeriod } from '@/api/admin'

export interface DashboardContext {
  dashData: Ref<DashboardData | null>
  loading: Ref<boolean>
  pollFailed: Ref<boolean>
  trendRange: Ref<number>
  /** 服务器流量分布（饼图）的统计粒度。 */
  rankPeriod: Ref<RankPeriod>
  /** 排行卡片的统计粒度，与饼图各自独立。 */
  rankingPeriod: Ref<RankPeriod>
  /** 排行卡片实际展示的数据：与饼图同粒度时复用主数据，否则是单独拉的那份。 */
  rankingData: Ref<DashboardData | null>
  openServerMetrics: (s: ServerMatrixItem) => void
  openOnlineUsers: (s: ServerMatrixItem) => void
  refreshDashboard: () => Promise<void>
  /** 切换排行粒度（接口不变，仍打 /admin/dashboard）。 */
  setRankingPeriod: (period: RankPeriod) => Promise<void>
}
