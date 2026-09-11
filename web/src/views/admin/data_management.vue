<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Refresh } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import BaseCard from '@/components/base/BaseCard.vue'
import { useThemeStore } from '@/stores/theme'
import { getLogStats, cleanupLogs, vacuumDatabase, compactHistory, updateSettings } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { LogStats, CompactResult } from '@/api/admin'

const theme = useThemeStore()
const loading = ref(false)
const stats = ref<LogStats | null>(null)
const rangeDays = ref<30 | 90>(30)

// ---- 图表 ----
const chartRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

// ---- 清理 ----
type LogTableKey = 'traffic_logs' | 'node_reports' | 'audit_logs'
interface LogTableMeta {
  key: LogTableKey
  label: string
  desc: string
  color: string
}
const logTables: LogTableMeta[] = [
  { key: 'traffic_logs', label: '流量明细', desc: '每用户每小时的计费明细（历史分钟级数据可压缩归并），受计费周期与每日聚合保护', color: '#38bdf8' },
  { key: 'node_reports', label: '节点心跳', desc: '服务器心跳状态上报，仅用于节点监控曲线回看', color: '#a78bfa' },
  { key: 'audit_logs', label: '审计日志', desc: '操作与登录审计记录，清理后不可再追溯', color: '#f59e0b' },
]
const cleanupTable = ref<LogTableKey>('audit_logs')
const cleanupDate = ref('')
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
      await nextTick()
      initChart()
      updateChart()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载数据统计失败'))
  } finally {
    loading.value = false
  }
}

function initChart() {
  if (chartRef.value && !chart) {
    chart = echarts.init(chartRef.value)
  }
}

function updateChart() {
  if (!chart || !stats.value) return
  const isDark = theme.isDark
  const textColor = isDark ? '#94a3b8' : '#64748b'
  const axisLineColor = isDark ? '#25334d' : '#e2e8f0'
  const splitLineColor = isDark ? '#1e293b' : '#f1f5f9'
  const tooltipBg = isDark ? 'rgba(19, 27, 46, 0.95)' : 'rgba(15, 23, 42, 0.9)'
  const tooltipBorder = isDark ? '#25334d' : '#334155'

  const days = stats.value.days
  chart.setOption({
    tooltip: {
      trigger: 'axis',
      confine: true,
      axisPointer: { type: 'shadow' },
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: '#ffffff', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.25); border-radius: 8px; z-index: 99;',
      formatter: (params: any) => {
        let res = `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params[0]?.axisValue}</div>`
        let total = 0
        params.forEach((item: any) => {
          const val = Number(item.value || 0)
          total += val
          res += `<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:2px">
            <span style="display:flex;align-items:center;gap:6px">${item.marker} <span style="color:#cbd5e1">${item.seriesName}</span></span>
            <b style="color:#fff">${val.toLocaleString()} 行</b>
          </div>`
        })
        res += `<div style="border-top:1px dashed rgba(255,255,255,0.2);margin-top:6px;padding-top:4px;display:flex;justify-content:space-between;color:#e2e8f0;font-size:11.5px"><span>合计:</span> <b style="color:#38bdf8">${total.toLocaleString()} 行</b></div>`
        return res
      },
    },
    legend: {
      data: logTables.map((t) => t.label),
      top: 0,
      right: 10,
      textStyle: { color: textColor, fontSize: 12 },
    },
    grid: { top: 35, right: 20, bottom: 25, left: 60 },
    xAxis: {
      type: 'category',
      data: days.map((d) => d.date.slice(5)),
      axisLine: { lineStyle: { color: axisLineColor } },
      axisLabel: { color: textColor, fontSize: 11, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      name: '行',
      nameTextStyle: { color: textColor },
      splitLine: { lineStyle: { color: splitLineColor } },
      axisLabel: { color: textColor, formatter: (v: number) => (v >= 10000 ? `${(v / 1000).toFixed(0)}k` : String(v)) },
    },
    series: logTables.map((t) => ({
      name: t.label,
      type: 'bar',
      stack: 'logs',
      barMaxWidth: 16,
      itemStyle: { color: t.color },
      data: days.map((d) => Number((d as any)[t.key] || 0)),
    })),
  })
}

function resizeChart() {
  chart?.resize()
}

// ---- 清理动作 ----
async function runCleanup() {
  if (!cleanupDate.value) {
    ElMessage.warning('请先选择清理截止日期')
    return
  }
  const meta = tableMeta.value
  try {
    await ElMessageBox.confirm(
      `确认清理 ${meta.label}中「${cleanupDate.value} 00:00」之前的记录？删除后不可恢复${
        cleanupTable.value === 'audit_logs' ? '，相关操作将不再可追溯' : ''
      }。`,
      `清理${meta.label}`,
      { type: 'error', confirmButtonText: '清理', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  cleanupRunning.value = true
  try {
    const { data } = await cleanupLogs(cleanupTable.value, cleanupDate.value)
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
}

// ---- 空间回收 ----
const vacuumRunning = ref(false)
const vacuumResult = ref<{ reclaimed: number } | null>(null)
async function runVacuum() {
  try {
    await ElMessageBox.confirm(
      '空间回收（VACUUM）会重写数据库文件以释放已删除数据占用的空间，期间数据库写入会短暂阻塞，建议在低峰时段执行。',
      '回收空间',
      { type: 'warning', confirmButtonText: '开始回收', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
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
}

// ---- 历史数据压缩 ----
const compactRunning = ref(false)
const compactResult = ref<CompactResult | null>(null)

async function runCompact() {
  try {
    await ElMessageBox.confirm(
      '将存量流量明细按小时归并、节点心跳按分钟抽稀，并清理超出保留期的数据，随后回收磁盘空间。归并只做求和/抽样，计费与用量口径不变，可重复执行。期间数据库写入会短暂阻塞，建议低峰执行。',
      '压缩历史数据',
      { type: 'warning', confirmButtonText: '开始压缩', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
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
    if (!Number.isInteger(v) || v < 1 || v > 3650) {
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

watch(rangeDays, () => load())
watch(cleanupTable, () => {
  cleanupDate.value = ''
})

onMounted(() => {
  load()
  window.addEventListener('resize', resizeChart)
})

onUnmounted(() => {
  window.removeEventListener('resize', resizeChart)
  chart?.dispose()
})
</script>

<template>
  <div class="x-page">
    <!-- 顶栏 -->
    <div class="x-toolbar" style="margin-bottom: 20px">
      <div class="x-toolbar-left">
        <div>
          <div style="font-size: 18px; font-weight: 600">数据管理</div>
          <div style="font-size: 12.5px; color: var(--x-text-3); margin-top: 2px">
            日志量可视化、按日期清理与空间回收（统计数据按天分桶）
          </div>
        </div>
      </div>
      <el-button :loading="loading" :icon="Refresh" @click="load">刷新</el-button>
    </div>

    <!-- 日志量可视化 + 按日期清理 -->
    <BaseCard title="日志量与清理" style="margin-bottom: 16px">
      <template #extra>
        <el-radio-group v-model="rangeDays" size="small">
          <el-radio-button :value="30">近 30 天</el-radio-button>
          <el-radio-button :value="90">近 90 天</el-radio-button>
        </el-radio-group>
      </template>

      <div v-loading="loading" style="min-height: 240px">
        <div class="stat-row">
          <div v-for="t in logTables" :key="t.key" class="stat-cell">
            <span class="dot" :style="{ background: t.color }" />
            <span class="stat-label">{{ t.label }}</span>
            <span class="stat-val cell-mono">{{ fmtCount(stats?.totals?.[t.key]) }}</span>
          </div>
          <div v-if="stats?.sqlite_avail" class="stat-cell">
            <span class="dot" style="background: #94a3b8" />
            <span class="stat-label">数据库文件</span>
            <span class="stat-val cell-mono">{{ fmtSize(stats?.db_size) }}（含 WAL {{ fmtSize(stats?.wal_size) }}）</span>
          </div>
        </div>

        <div ref="chartRef" style="width: 100%; height: 300px; margin-top: 8px" />

        <el-divider style="margin: 12px 0" />

        <div class="cleanup-row">
          <el-select v-model="cleanupTable" style="width: 160px">
            <el-option v-for="t in logTables" :key="t.key" :label="t.label" :value="t.key" />
          </el-select>
          <el-date-picker
            v-model="cleanupDate"
            type="date"
            placeholder="选择清理截止日期（不含当日）"
            value-format="YYYY-MM-DD"
            :disabled-date="disabledDate"
            style="width: 240px"
          />
          <span class="muted" style="font-size: 12.5px">
            <template v-if="estimatedRows?.fullOutside">所选日期早于统计窗口，实际删除量以执行结果为准</template>
            <template v-else-if="estimatedRows">预估删除 {{ fmtCount(estimatedRows.count) }} 行（不含窗口外更早数据）</template>
            <template v-else>—</template>
          </span>
          <el-button type="danger" :icon="Delete" :loading="cleanupRunning" :disabled="!cleanupDate" @click="runCleanup">
            清理
          </el-button>
        </div>
        <p class="muted" style="font-size: 12.5px; margin-top: 10px; line-height: 1.7">
          {{ tableMeta.desc }}。<template v-if="cleanupTable === 'traffic_logs'">
            流量明细是配额判定与用量统计的数据源，仅可清理 <b>{{ maxDeleteDate || '—' }}</b> 及更早的日期（受计费周期与每日聚合窗口保护）。
          </template>
          <template v-else-if="cleanupTable === 'node_reports'">清理只缩短节点监控曲线的可回看窗口，不影响服务器状态。</template>
          <template v-else>审计日志删除后，对应时段的操作记录将无法追溯，请谨慎清理。</template>
        </p>
      </div>
    </BaseCard>

    <!-- 历史数据压缩 -->
    <BaseCard title="历史数据压缩" style="margin-bottom: 16px">
      <div class="cleanup-row" style="align-items: center">
        <span class="muted" style="font-size: 12.5px">
          把存量流量明细归并到小时桶、节点心跳抽稀到每分钟，并清理超出保留期的数据，随后回收磁盘空间。
          归并只做求和/抽样，计费与用量口径不变，可重复执行。
        </span>
        <el-button type="primary" :loading="compactRunning" @click="runCompact">压缩历史数据</el-button>
      </div>
      <div v-if="compactResult" class="muted" style="font-size: 12.5px; margin-top: 10px; line-height: 1.9">
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
          释放空间 <b class="cell-mono">{{ fmtSize(Math.max(0, compactResult.reclaimed || 0)) }}</b>
        </div>
        <div v-if="compactResult.vacuum_error" style="color: var(--el-color-warning)">
          {{ compactResult.vacuum_error }}
        </div>
      </div>
      <p class="muted" style="font-size: 12.5px; margin-top: 10px">
        压缩后会执行一次 VACUUM 回收磁盘，期间数据库写入会短暂阻塞且需约 2 倍文件大小的空闲空间，建议在低峰时段执行；已归并到小时/分钟的数据不会重复压缩。
      </p>
    </BaseCard>

    <!-- 空间回收 -->
    <BaseCard title="空间回收（VACUUM）" style="margin-bottom: 16px">
      <div class="cleanup-row" style="align-items: center">
        <span class="muted" style="font-size: 12.5px">
          SQLite 删除数据后文件不会自动缩小，此处重写数据库文件以释放空间。当前占用：
          <template v-if="stats?.sqlite_avail">
            <b class="cell-mono">{{ fmtSize(stats?.db_size) }}</b>（WAL <span class="cell-mono">{{ fmtSize(stats?.wal_size) }}</span>）
          </template>
          <template v-else>当前数据库类型不支持在线回收</template>
        </span>
        <el-button type="primary" :loading="vacuumRunning" :disabled="!stats?.sqlite_avail" @click="runVacuum">
          回收空间
        </el-button>
      </div>
      <p v-if="vacuumResult" class="muted" style="font-size: 12.5px; margin-top: 10px">
        上次回收释放了 <b class="cell-mono">{{ fmtSize(Math.max(0, vacuumResult.reclaimed)) }}</b>
      </p>
      <p class="muted" style="font-size: 12.5px; margin-top: 10px">回收期间数据库写入会短暂阻塞，建议在低峰时段执行；日常无需频繁回收。</p>
    </BaseCard>

    <!-- 自动保留天数 -->
    <BaseCard title="自动保留天数">
      <p class="muted" style="font-size: 12.5px; margin-bottom: 12px">
        每日凌晨 4 点自动清理超过保留期的日志数据；保存后次日生效。手动清理请使用上方「按日期清理」。
      </p>
      <div class="retention-row">
        <div class="retention-item">
          <span class="stat-label">流量明细</span>
          <el-input-number v-model="retentionForm.retention_traffic_days" :min="1" :max="3650" size="small" />
          <span class="muted" style="font-size: 12px">天</span>
        </div>
        <div class="retention-item">
          <span class="stat-label">节点心跳</span>
          <el-input-number v-model="retentionForm.retention_node_reports_days" :min="1" :max="3650" size="small" />
          <span class="muted" style="font-size: 12px">天</span>
        </div>
        <div class="retention-item">
          <span class="stat-label">审计日志</span>
          <el-input-number v-model="retentionForm.retention_audit_days" :min="1" :max="3650" size="small" />
          <span class="muted" style="font-size: 12px">天</span>
        </div>
        <el-button type="primary" size="small" :loading="retentionSaving" @click="saveRetention">保存</el-button>
      </div>
      <p class="muted" style="font-size: 12px; margin-top: 10px">
        流量明细建议 ≥ 在售套餐最长计费周期（如年付套餐建议 ≥ 366 天），否则自动清理会提前抹掉部分用户的已用流量。
      </p>
    </BaseCard>
  </div>
</template>

<style scoped>
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
.stat-label {
  color: var(--x-text-2);
}
.stat-val {
  font-weight: 600;
}
.cleanup-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
.muted {
  color: var(--x-text-3);
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
