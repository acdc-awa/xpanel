<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsBarChart,
  CsBox,
  CsButton,
  CsConfirmDialog,
  CsContainer,
  CsDatePicker,
  CsDivider,
  CsInputNumber,
  CsLoading,
  CsPage,
  CsSegmentedControl,
  CsSelect,
  CsToolbar,
} from '@/components/cloudscape'
import type { BarSeries, SegmentedOption } from '@/components/cloudscape'
import { getLogStats, cleanupLogs, vacuumDatabase, compactHistory, updateSettings } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { LogStats, CompactResult } from '@/api/admin'

/**
 * 破坏性操作确认统一走 CsConfirmDialog（AWS 控制台口径：红色 Warning 行 +
 * 红描边取消 / 红实心确认）。本页三个动作（清理、压缩、VACUUM）都不可逆。
 */
const confirmState = reactive({
  visible: false,
  header: '',
  warning: '',
  message: '',
  confirmText: '确认',
  action: null as null | (() => void | Promise<void>),
})

function askConfirm(opts: {
  header: string
  warning: string
  message?: string
  confirmText?: string
  action: () => void | Promise<void>
}) {
  confirmState.header = opts.header
  confirmState.warning = opts.warning
  confirmState.message = opts.message ?? ''
  confirmState.confirmText = opts.confirmText ?? '确认'
  confirmState.action = opts.action
  confirmState.visible = true
}

async function runConfirmed() {
  const action = confirmState.action
  confirmState.visible = false
  confirmState.action = null
  if (action) await action()
}

const loading = ref(false)
const stats = ref<LogStats | null>(null)
const rangeDays = ref<30 | 90>(30)

const rangeOptions: SegmentedOption[] = [
  { id: 30, text: '近 30 天' },
  { id: 90, text: '近 90 天' },
]

/** 分段控件的 v-model 是 string | number，这里收窄回本页的天数联合类型。 */
function onRangeChange(detail: { selectedId: string | number }) {
  rangeDays.value = Number(detail.selectedId) === 90 ? 90 : 30
}

// ---- 清理 ----
type LogTableKey = 'traffic_logs' | 'node_reports' | 'audit_logs'
interface LogTableMeta {
  key: LogTableKey
  label: string
  desc: string
  color: string
}
// 颜色取 Cloudscape 官方分类色板前三位，保证图例、图表与统计点同源。
const logTables: LogTableMeta[] = [
  { key: 'traffic_logs', label: '流量明细', desc: '每用户每小时的计费明细（历史分钟级数据可压缩归并），受计费周期与每日聚合保护', color: 'var(--color-charts-palette-categorical-1-r41m2k, #688ae8)' },
  { key: 'node_reports', label: '节点心跳', desc: '服务器心跳状态上报，仅用于节点监控曲线回看', color: 'var(--color-charts-palette-categorical-2-37jc27, #c33d69)' },
  { key: 'audit_logs', label: '审计日志', desc: '操作与登录审计记录，清理后不可再追溯', color: 'var(--color-charts-palette-categorical-3-2qk36g, #2ea597)' },
]

const logTableOptions = logTables.map((t) => ({ label: t.label, value: t.key }))

/** 日志量按天堆叠柱状图（Cloudscape BarChart 语义）。 */
const chartSeries = computed<BarSeries[]>(() => {
  const days = stats.value?.days || []
  return logTables.map((t) => ({
    type: 'bar',
    title: t.label,
    color: t.color,
    data: days.map((d) => ({ x: d.date.slice(5), y: Number((d as unknown as Record<string, number>)[t.key] || 0) })),
    valueFormatter: (value) => `${value.toLocaleString()} 行`,
  }))
})

const formatRowCount = (value: number) => (value >= 10000 ? `${(value / 1000).toFixed(0)}k` : String(value))
const cleanupTable = ref<LogTableKey>('audit_logs')
const cleanupDate = ref<string | null>('')
const cleanupRunning = ref(false)

const tableMeta = computed(() => logTables.find((t) => t.key === cleanupTable.value)!)
// 本地日期串（toISOString 按 UTC 会差一天）
function fmtLocalDate(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
// 流量明细仅可清理安全上界及更早；其余表最早到今天
const maxDeleteDate = computed(() => {
  if (cleanupTable.value === 'traffic_logs') return stats.value?.traffic_min_delete || ''
  return fmtLocalDate(new Date())
})
// 预估删除行数：仅统计已加载窗口内的日期；截止日早于窗口起点时无法估算
const estimatedRows = computed(() => {
  if (!stats.value || !cleanupDate.value) return null
  const first = stats.value.days[0]?.date
  if (!first) return null
  if (cleanupDate.value <= first) return { count: 0, fullOutside: true }
  let n = 0
  for (const d of stats.value.days) {
    if (d.date < cleanupDate.value) n += Number((d as any)[cleanupTable.value] || 0)
  }
  return { count: n, fullOutside: false }
})

function disabledDate(d: Date) {
  const s = fmtLocalDate(d)
  if (cleanupTable.value === 'traffic_logs') {
    const max = stats.value?.traffic_min_delete || ''
    return !max || s > max
  }
  return s > maxDeleteDate.value
}

async function load() {
  loading.value = true
  try {
    const { data } = await getLogStats(rangeDays.value)
    if (data.code === 0) {
      stats.value = data.data
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载数据统计失败'))
  } finally {
    loading.value = false
  }
}

// ---- 清理动作 ----
async function runCleanup() {
  if (!cleanupDate.value) {
    ElMessage.warning('请先选择清理截止日期')
    return
  }
  const meta = tableMeta.value
  const before = cleanupDate.value
  askConfirm({
    header: `清理${meta.label}？`,
    warning: `删除后不可恢复${cleanupTable.value === 'audit_logs' ? '，相关操作将不再可追溯' : ''}。`,
    message: `将删除 ${meta.label}中「${before} 00:00」之前的全部记录。`,
    confirmText: '清理',
    action: async () => {
      cleanupRunning.value = true
      try {
        const { data } = await cleanupLogs(cleanupTable.value, before)
        if (data.code === 0) {
          ElMessage.success(`已清理 ${data.data.deleted.toLocaleString()} 条${meta.label}记录`)
          load()
        } else {
          ElMessage.error(data.message)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '清理失败'))
      } finally {
        cleanupRunning.value = false
      }
    },
  })
}

// ---- 空间回收 ----
const vacuumRunning = ref(false)
const vacuumResult = ref<{ reclaimed: number } | null>(null)
async function runVacuum() {
  askConfirm({
    header: '回收数据库空间？',
    warning: '回收期间数据库写入会短暂阻塞，建议在低峰时段执行。',
    message: 'VACUUM 会重写数据库文件以释放已删除数据占用的空间，需要约 2 倍文件大小的空闲磁盘。',
    confirmText: '开始回收',
    action: async () => {
      vacuumRunning.value = true
      try {
        const { data } = await vacuumDatabase()
        if (data.code === 0) {
          vacuumResult.value = data.data
          ElMessage.success(`已回收 ${fmtSize(Math.max(0, data.data.reclaimed))}`)
          load()
        } else {
          ElMessage.error(data.message)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '空间回收失败'))
      } finally {
        vacuumRunning.value = false
      }
    },
  })
}

// ---- 历史数据压缩 ----
const compactRunning = ref(false)
const compactResult = ref<CompactResult | null>(null)

async function runCompact() {
  askConfirm({
    header: '压缩历史数据？',
    warning: '期间数据库写入会短暂阻塞，并额外占用约 2 倍数据库文件的空闲空间。',
    message: '将归并存量流量明细、清理超出保留期的数据并回收磁盘空间。归并只做求和/抽样，计费与用量口径不变，可重复执行。',
    confirmText: '开始压缩',
    action: async () => {
      compactRunning.value = true
      try {
        const { data } = await compactHistory()
        if (data.code === 0) {
          compactResult.value = data.data
          ElMessage.success('历史数据压缩完成')
          load()
        } else {
          ElMessage.error(data.message)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '压缩失败'))
      } finally {
        compactRunning.value = false
      }
    },
  })
}

// ---- 保留天数 ----
const retentionForm = ref({
  retention_traffic_days: 90,
  retention_node_reports_days: 30,
  retention_audit_days: 180,
})
const retentionSaving = ref(false)

watch(
  () => stats.value?.retention,
  (r) => {
    if (!r) return
    retentionForm.value = {
      retention_traffic_days: Number(r.retention_traffic_days) || 90,
      retention_node_reports_days: Number(r.retention_node_reports_days) || 30,
      retention_audit_days: Number(r.retention_audit_days) || 180,
    }
  },
  { immediate: true },
)

async function saveRetention() {
  const f = retentionForm.value
  for (const v of [f.retention_traffic_days, f.retention_node_reports_days, f.retention_audit_days]) {
    if (v === null || !Number.isInteger(v) || v < 1 || v > 3650) {
      ElMessage.warning('保留天数需为 1–3650 的整数')
      return
    }
  }
  retentionSaving.value = true
  try {
    const { data } = await updateSettings({ retention: { ...mapValues(f) } })
    if (data.code === 0) {
      ElMessage.success('保留天数已保存，每日凌晨自动清理将按新周期执行')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    retentionSaving.value = false
  }
}

function mapValues(f: typeof retentionForm.value) {
  return {
    retention_traffic_days: String(f.retention_traffic_days),
    retention_node_reports_days: String(f.retention_node_reports_days),
    retention_audit_days: String(f.retention_audit_days),
  }
}

function fmtSize(n?: number) {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

function fmtCount(n?: number) {
  return (n || 0).toLocaleString()
}

// 主库与 WAL 是两个独立文件，占用为两者之和（此前文案写成「含 WAL」，易读成包含关系）。
const totalDbSize = computed(() => (stats.value?.db_size || 0) + (stats.value?.wal_size || 0))

watch(rangeDays, () => load())
watch(cleanupTable, () => {
  cleanupDate.value = ''
})

onMounted(() => {
  load()
})
</script>

<template>
  <CsPage>
    <CsToolbar>
      <div>
        <CsBox variant="div" font-size="heading-m" font-weight="bold">数据管理</CsBox>
        <CsBox variant="div" font-size="body-s" color="text-body-secondary">
          日志量可视化、按日期清理与空间回收（统计数据按天分桶）
        </CsBox>
      </div>
      <template #actions>
        <CsButton :loading="loading" icon-name="refresh" @click="load">刷新</CsButton>
      </template>
    </CsToolbar>

    <!-- 日志量可视化 + 按日期清理 -->
    <CsContainer header="日志量与清理">
      <template #actions>
        <CsSegmentedControl :model-value="rangeDays" :options="rangeOptions" label="统计窗口" @change="onRangeChange" />
      </template>

      <CsLoading :loading="loading" min-height="240px">
        <div class="stat-row">
          <div v-for="t in logTables" :key="t.key" class="stat-cell">
            <span class="dot" :style="{ background: t.color }" />
            <CsBox variant="span" color="text-body-secondary">{{ t.label }}</CsBox>
            <CsBox variant="span" font-weight="bold">{{ fmtCount(stats?.totals?.[t.key]) }}</CsBox>
          </div>
          <div v-if="stats?.sqlite_avail" class="stat-cell">
            <span class="dot" style="background: #94a3b8" />
            <CsBox variant="span" color="text-body-secondary">数据库文件</CsBox>
            <CsBox variant="span" font-weight="bold">
              {{ fmtSize(stats?.db_size) }} + WAL {{ fmtSize(stats?.wal_size) }} = {{ fmtSize(totalDbSize) }}
            </CsBox>
          </div>
        </div>

        <CsBarChart
          class="log-volume-chart"
          :series="chartSeries"
          :height="300"
          stacked-bars
          y-title="行"
          :y-tick-formatter="formatRowCount"
          aria-label="日志量趋势"
          empty-text="暂无日志数据"
        />

        <CsDivider style="margin: 12px 0" />

        <div class="cleanup-row">
          <CsSelect v-model="cleanupTable" :options="logTableOptions" style="width: 160px" aria-label="清理目标" />
          <CsDatePicker
            v-model="cleanupDate"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="选择清理截止日期（不含当日）"
            :disabled-date="disabledDate"
            aria-label="清理截止日期"
            style="width: 240px"
          />
          <CsBox variant="span" font-size="body-s" color="text-body-secondary">
            <template v-if="estimatedRows?.fullOutside">所选日期早于统计窗口，实际删除量以执行结果为准</template>
            <template v-else-if="estimatedRows">预估删除 {{ fmtCount(estimatedRows.count) }} 行（不含窗口外更早数据）</template>
            <template v-else>—</template>
          </CsBox>
          <CsButton icon-name="remove" :loading="cleanupRunning" :disabled="!cleanupDate" @click="runCleanup">
            清理
          </CsButton>
        </div>
        <CsBox variant="p" font-size="body-s" color="text-body-secondary" :margin="{ top: 's' }">
          {{ tableMeta.desc }}。<template v-if="cleanupTable === 'traffic_logs'">
            仅可清理 <b>{{ maxDeleteDate || '—' }}</b> 及更早的日期（受计费周期与每日聚合窗口保护）。
          </template>
          <TipIcon
            v-if="cleanupTable === 'node_reports'"
            content="清理只缩短节点监控曲线的可回看窗口，不影响服务器状态。"
          />
          <TipIcon
            v-if="cleanupTable === 'audit_logs'"
            type="warn"
            content="审计日志删除后，对应时段的操作记录将无法追溯，请谨慎清理。"
          />
        </CsBox>
      </CsLoading>
    </CsContainer>

    <!-- 历史数据压缩 -->
    <CsContainer header="历史数据压缩">
      <template #info>
        <TipIcon content="把存量流量明细归并到小时桶、节点心跳按行龄抽稀（近 6 小时 1 分钟，更早 10 分钟），并清理超出保留期的数据。归并只做求和/抽样，计费与用量口径不变，可重复执行。" />
      </template>
      <div class="cleanup-row" style="align-items: center">
        <CsButton variant="primary" :loading="compactRunning" @click="runCompact">压缩历史数据</CsButton>
        <TipIcon
          type="warn"
          content="压缩后会执行一次 VACUUM 回收磁盘，期间数据库写入会短暂阻塞且需约 2 倍文件大小的空闲空间，建议在低峰时段执行；已归并到小时/分级桶的数据不会重复压缩。"
        />
      </div>
      <CsBox v-if="compactResult" variant="div" font-size="body-s" color="text-body-secondary" :margin="{ top: 's' }">
        <div>
          流量明细：{{ fmtCount(compactResult.stats.traffic_rows_before) }} →
          {{ fmtCount(compactResult.stats.traffic_rows_after) }} 行（减少
          {{ fmtCount(compactResult.stats.traffic_rows_removed) }}）
        </div>
        <div>
          节点心跳：{{ fmtCount(compactResult.stats.node_rows_before) }} →
          {{ fmtCount(compactResult.stats.node_rows_after) }} 行（减少
          {{ fmtCount(compactResult.stats.node_rows_removed) }}）
        </div>
        <div v-if="compactResult.reclaimed !== undefined">
          释放空间 <CsBox variant="b">{{ fmtSize(Math.max(0, compactResult.reclaimed || 0)) }}</CsBox>
        </div>
        <div v-if="compactResult.vacuum_error" style="color: var(--color-text-status-warning-fabq6h, #8d6605)">
          {{ compactResult.vacuum_error }}
        </div>
      </CsBox>
    </CsContainer>

    <!-- 空间回收 -->
    <CsContainer header="空间回收（VACUUM）">
      <template #info>
        <TipIcon content="SQLite 删除数据后文件不会自动缩小，此处重写数据库文件以释放空间。" />
      </template>
      <div class="cleanup-row" style="align-items: center">
        <CsBox variant="span" font-size="body-s" color="text-body-secondary">
          当前占用：
          <template v-if="stats?.sqlite_avail">
            主库 <CsBox variant="b">{{ fmtSize(stats?.db_size) }}</CsBox> + WAL
            <CsBox variant="span">{{ fmtSize(stats?.wal_size) }}</CsBox> = 共
            <CsBox variant="b">{{ fmtSize(totalDbSize) }}</CsBox>
          </template>
          <template v-else>当前数据库类型不支持在线回收</template>
        </CsBox>
        <CsButton variant="primary" :loading="vacuumRunning" :disabled="!stats?.sqlite_avail" @click="runVacuum">
          回收空间
        </CsButton>
        <TipIcon type="warn" content="回收期间数据库写入会短暂阻塞，建议在低峰时段执行；日常无需频繁回收。" />
      </div>
      <CsBox v-if="vacuumResult" variant="p" font-size="body-s" color="text-body-secondary" :margin="{ top: 's' }">
        上次回收释放了 <CsBox variant="b">{{ fmtSize(Math.max(0, vacuumResult.reclaimed)) }}</CsBox>
      </CsBox>
    </CsContainer>

    <!-- 自动保留天数 -->
    <CsContainer header="自动保留天数">
      <template #info>
        <TipIcon content="每日凌晨 4 点自动清理超过保留期的日志数据；保存后次日生效。手动清理请使用上方「按日期清理」。" />
      </template>
      <div class="retention-row">
        <div class="retention-item">
          <span class="cs-text-secondary">流量明细</span>
          <TipIcon type="warn" content="建议 ≥ 在售套餐最长计费周期（如年付套餐建议 ≥ 366 天），否则自动清理会提前抹掉部分用户的已用流量。" />
          <CsInputNumber v-model="retentionForm.retention_traffic_days" :min="1" :max="3650" style="width: 120px" />
          <span class="cs-text-secondary" style="font-size: 12px">天</span>
        </div>
        <div class="retention-item">
          <span class="cs-text-secondary">节点心跳</span>
          <CsInputNumber v-model="retentionForm.retention_node_reports_days" :min="1" :max="3650" style="width: 120px" />
          <span class="cs-text-secondary" style="font-size: 12px">天</span>
        </div>
        <div class="retention-item">
          <span class="cs-text-secondary">审计日志</span>
          <CsInputNumber v-model="retentionForm.retention_audit_days" :min="1" :max="3650" style="width: 120px" />
          <span class="cs-text-secondary" style="font-size: 12px">天</span>
        </div>
        <CsButton variant="primary" :loading="retentionSaving" @click="saveRetention">保存</CsButton>
      </div>
    </CsContainer>

    <!-- 破坏性操作确认（AWS 控制台口径：红色 Warning 行 + 红描边/红实心按钮） -->
    <CsConfirmDialog
      v-model:visible="confirmState.visible"
      :header="confirmState.header"
      :warning="confirmState.warning"
      :message="confirmState.message"
      :confirm-text="confirmState.confirmText"
      :loading="cleanupRunning || compactRunning || vacuumRunning"
      @confirm="runConfirmed"
    />
  </CsPage>
</template>

<style scoped>
.log-volume-chart {
  margin-top: 8px;
}

.stat-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 28px;
  font-size: 13px;
}
.stat-cell {
  display: flex;
  align-items: center;
  gap: 7px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.cleanup-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
.retention-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px 24px;
}
.retention-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
