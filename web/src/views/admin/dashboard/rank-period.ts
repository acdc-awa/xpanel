import type { SegmentedOption } from '@/components/cloudscape'

/**
 * 排行 / 流量分布卡片的统计粒度选项。两张卡片各自持有粒度（饼图用
 * ctx.rankPeriod、排行用 ctx.rankingPeriod），接口不变、互不影响。
 */
export const rankPeriodOptions: SegmentedOption[] = [
  { id: 'today', text: '今日' },
  { id: '7d', text: '7 天' },
  { id: '30d', text: '30 天' },
]
