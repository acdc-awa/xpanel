<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import Spinner from '../Spinner.vue'
import ChartLegend from './ChartLegend.vue'
import ChartSeriesMarker from './ChartSeriesMarker.vue'
import { CHART } from './classes'
import { createCategoryColorScale } from './colors'
import { computeDomainX, computeDomainY } from './domain'
import { createBandScale, createLinearScale } from './scales'
import type { ChartDataTypes, ChartProps, ChartSeries } from './types'
import {
  calculateStackedBarValues,
  createOneSideRoundedRectPath,
  formatDefaultNumber,
  getKeyValue,
  isDataSeries,
  isXThreshold,
  isYThreshold,
  matchesX,
  measureText,
  reduceTicks,
} from './utils'
import { cx } from '../utils'

const props = withDefaults(defineProps<ChartProps>(), {
  series: () => [],
  height: 300,
  fitHeight: false,
  stackedBars: false,
  xScaleType: 'categorical',
  xTickFormatter: undefined,
  yTickFormatter: undefined,
  xTitle: undefined,
  yTitle: undefined,
  legendTitle: undefined,
  hideLegend: false,
  emptyText: '暂无数据',
  ariaLabel: '图表',
  loading: false,
  emphasizeBaselineAxis: true,
})

const TICK_LENGTH = 8
const TICK_MARGIN = 4
const TICK_LINE_HEIGHT = 16
const INLINE_START_LABELS_MARGIN = 16
const MIN_PLOT_HEIGHT = 60

const rootRef = ref<HTMLDivElement | null>(null)
const plotAreaRef = ref<HTMLDivElement | null>(null)
const containerWidth = ref(0)
const containerHeight = ref(0)
const hoverIndex = ref<number | null>(null)
const hoveredSeriesTitle = ref<string | null>(null)
const pointer = ref({ x: 0, y: 0 })
const clipId = useId()

let observer: ResizeObserver | null = null

function measure() {
  const root = rootRef.value
  const area = plotAreaRef.value
  if (!root) return
  containerWidth.value = root.clientWidth
  // The plot area owns the vertical space, so measuring it (rather than the root)
  // keeps `fitHeight` correct once a legend or axis title is added below.
  containerHeight.value = area ? area.clientHeight : root.clientHeight
}

onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => measure())
    if (rootRef.value) observer.observe(rootRef.value)
  }
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})

const series = computed<ChartSeries[]>(() => props.series ?? [])
const colors = computed(() => createCategoryColorScale(series.value))
const hasData = computed(() => series.value.some((s) => (isDataSeries(s) ? s.data.length > 0 : true)))

const domainX = computed<ChartDataTypes[]>(() => computeDomainX(series.value))
const useCategoricalX = computed(
  () =>
    props.xScaleType !== 'linear' ||
    domainX.value.length === 0 ||
    !domainX.value.every((x) => x instanceof Date || typeof x === 'number')
)

const bottomLabelsHeight = computed(() => {
  if (useCategoricalX.value) return TICK_LENGTH + TICK_MARGIN + TICK_LINE_HEIGHT
  return TICK_LENGTH + TICK_MARGIN + TICK_LINE_HEIGHT
})

function formatXTick(x: ChartDataTypes): string {
  return props.xTickFormatter ? props.xTickFormatter(x) : String(x)
}

function formatYTick(y: number): string {
  return props.yTickFormatter ? props.yTickFormatter(y) : formatDefaultNumber(y)
}

/** Height available to the plot itself, excluding the bottom axis labels. */
const plotHeight = computed(() => {
  if (props.fitHeight) {
    return Math.max(MIN_PLOT_HEIGHT, containerHeight.value - bottomLabelsHeight.value)
  }
  return Math.max(MIN_PLOT_HEIGHT, props.height - bottomLabelsHeight.value)
})

const yDomain = computed(() => computeDomainY(series.value, props.stackedBars))

/** Y scale is computed first because its labels determine the left gutter. */
const yScale = computed(() => {
  const tickCount = Math.max(2, Math.ceil(plotHeight.value / 50))
  return createLinearScale(yDomain.value, plotHeight.value, 0, tickCount)
})

const yTicks = computed(() => {
  const formatted = yScale.value.ticks.map((tick) => formatYTick(tick))
  const maxLabelWidth = formatted.reduce((max, label) => Math.max(max, measureText(label)), 0)
  const items = yScale.value.ticks.map((tick, index) => ({
    value: tick,
    position: yScale.value.scale(tick),
    label: formatted[index],
    width: measureText(formatted[index]),
  }))
  return { items, maxLabelWidth }
})

const leftGutter = computed(() => Math.ceil(yTicks.value.maxLabelWidth) + INLINE_START_LABELS_MARGIN)

const plotWidth = computed(() => Math.max(40, containerWidth.value - leftGutter.value))

const xScale = computed(() => {
  if (useCategoricalX.value) {
    const inner = series.value.some((s) => s.type === 'bar') ? 0.2 : 0.7
    const outer = series.value.some((s) => s.type === 'bar') ? 0.05 : 0
    return createBandScale(domainX.value, 0, plotWidth.value, inner, outer)
  }
  const numbers = domainX.value.map((x) => (x instanceof Date ? x.getTime() : Number(x)))
  const min = Math.min(...numbers)
  const max = Math.max(...numbers)
  return createLinearScale([min, max], 0, plotWidth.value, Math.max(2, Math.ceil(plotWidth.value / 100)))
})

const xTicks = computed(() => {
  if (useCategoricalX.value) {
    const band = xScale.value as ReturnType<typeof createBandScale>
    const offset = Math.max(0, band.bandwidth() - 1) / 2
    const items = domainX.value.map((value) => ({
      value,
      position: band.position(value) + offset,
      label: formatXTick(value),
      width: measureText(formatXTick(value)),
    }))
    return reduceTicks(items.filter((item) => isFinite(item.position)))
  }
  const linear = xScale.value as ReturnType<typeof createLinearScale>
  const isDate = domainX.value[0] instanceof Date
  const items = linear.ticks.map((tick) => {
    const value = isDate ? new Date(tick) : tick
    return {
      value: value as ChartDataTypes,
      position: linear.scale(tick),
      label: formatXTick(value as ChartDataTypes),
      width: measureText(formatXTick(value as ChartDataTypes)),
    }
  })
  return reduceTicks(items.filter((item) => isFinite(item.position)))
})

function scaleX(value: ChartDataTypes): number {
  if (useCategoricalX.value) {
    const band = xScale.value as ReturnType<typeof createBandScale>
    const position = band.position(value)
    if (!isFinite(position)) return NaN
    return position + Math.max(0, band.bandwidth() - 1) / 2
  }
  const numeric = value instanceof Date ? value.getTime() : Number(value)
  return (xScale.value as ReturnType<typeof createLinearScale>).scale(numeric)
}

interface BarShape {
  key: string
  title: string
  color: string
  side: 'all' | 'none' | 'left' | 'right' | 'top' | 'bottom'
  rx: number
  x: number
  y: number
  width: number
  height: number
}

const stackedValues = computed(() => {
  if (!props.stackedBars) return null
  const dataBySeries = series.value.map((item) => (item.type === 'bar' ? item.data : []))
  return calculateStackedBarValues(dataBySeries)
})

const barShapes = computed<BarShape[]>(() => {
  const shapes: BarShape[] = []
  const barSeriesIndices = series.value.map((item, index) => (item.type === 'bar' ? index : -1)).filter((i) => i >= 0)
  if (barSeriesIndices.length === 0) return shapes

  const band = useCategoricalX.value
    ? (xScale.value as ReturnType<typeof createBandScale>)
    : null
  const zeroY = yScale.value.scale(0)
  const baseY = isFinite(zeroY) ? Math.min(plotHeight.value, zeroY) : plotHeight.value
  const bandwidth = band ? band.bandwidth() : 0
  const PADDING = 4
  const MIN_WIDTH = 4
  const isStacked = !!props.stackedBars

  let barWidth = bandwidth
  if (!isStacked && barSeriesIndices.length > 1) {
    barWidth = Math.max(MIN_WIDTH, (bandwidth - (barSeriesIndices.length - 1) * PADDING) / barSeriesIndices.length)
  }

  barSeriesIndices.forEach((seriesIndex, ordinal) => {
    const item = series.value[seriesIndex]
    if (item.type !== 'bar') return

    for (const datum of item.data) {
      const x = band ? band.position(datum.x) : scaleX(datum.x) - barWidth / 2
      if (!isFinite(x)) continue

      let cumulative = datum.y
      let roundedStart = !isStacked
      let roundedEnd = !isStacked
      if (isStacked && stackedValues.value) {
        const byIndex = stackedValues.value.get(getKeyValue(datum.x))
        cumulative = byIndex?.get(seriesIndex) ?? 0
        const all = byIndex ? Array.from(byIndex.values()).sort((a, b) => a - b) : []
        roundedStart = all.length > 0 && cumulative === all[0]
        roundedEnd = all.length > 0 && cumulative === all[all.length - 1]
      }

      const lower = cumulative < 0 ? cumulative - datum.y : cumulative
      const top = yScale.value.scale(lower)
      const height = Math.abs(yScale.value.scale(datum.y) - baseY)
      if (!isFinite(top) || !isFinite(height) || height <= 0) continue

      const stackedOffset = isStacked ? 3 : 0
      const heightOffset = height < 4 ? 0 : stackedOffset
      const widthOffset = 2
      const rx = height < 4 ? 2 : 4
      const barX = isStacked ? x : x + ordinal * (barWidth + PADDING)

      let side: BarShape['side'] = 'none'
      if (roundedStart && roundedEnd) side = 'all'
      else if (roundedStart) side = 'bottom'
      else if (roundedEnd) side = 'top'

      shapes.push({
        key: `${seriesIndex}-${getKeyValue(datum.x)}`,
        title: item.title,
        color: colors.value[seriesIndex],
        side,
        rx,
        x: barX + widthOffset / 2,
        y: top + heightOffset / 2,
        width: Math.max(0, barWidth - widthOffset),
        height: Math.max(0, height - heightOffset),
      })
    }
  })

  return shapes
})

interface LineShape {
  key: string
  title: string
  color: string
  path: string
  dashed: boolean
}

const lineShapes = computed<LineShape[]>(() => {
  const shapes: LineShape[] = []
  series.value.forEach((item, index) => {
    if (item.type !== 'line') return
    const points = item.data
      .map((datum) => ({ x: scaleX(datum.x), y: yScale.value.scale(datum.y) }))
      .filter((point) => isFinite(point.x) && isFinite(point.y))
      .sort((a, b) => a.x - b.x)
    if (points.length === 0) return
    const path = points.map((point, i) => `${i === 0 ? 'M' : 'L'}${point.x},${point.y}`).join('')
    shapes.push({ key: String(index), title: item.title, color: colors.value[index], path, dashed: !!item.dashed })
  })
  return shapes
})

const yThresholds = computed(() =>
  series.value
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => isYThreshold(item))
    .map(({ item, index }) => ({
      key: String(index),
      color: colors.value[index],
      y: yScale.value.scale((item as { y: number }).y),
      title: item.title,
    }))
)

const xThresholds = computed(() =>
  series.value
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => isXThreshold(item))
    .map(({ item, index }) => ({
      key: String(index),
      color: colors.value[index],
      x: scaleX((item as { x: ChartDataTypes }).x),
      title: item.title,
    }))
)

const legendItems = computed(() =>
  series.value.map((item, index) => ({
    title: item.title,
    color: colors.value[index],
    type: (item.type === 'bar' ? 'rectangle' : item.type === 'threshold' ? 'dashed' : 'line') as
      | 'line'
      | 'rectangle'
      | 'dashed',
  }))
)

/** Per-series marker points drawn while a column is hovered (line charts only). */
const hoveredPoints = computed(() => {
  if (hoverIndex.value === null) return []
  const x = domainX.value[hoverIndex.value]
  if (x === undefined) return []
  const points: { key: string; color: string; x: number; y: number }[] = []
  series.value.forEach((item, index) => {
    if (item.type !== 'line') return
    const datum = item.data.find((d) => matchesX(d.x, x))
    if (!datum) return
    const px = scaleX(datum.x)
    const py = yScale.value.scale(datum.y)
    if (isFinite(px) && isFinite(py)) {
      points.push({ key: `${index}-${px}`, color: colors.value[index], x: px, y: py })
    }
  })
  return points
})

const markerX = computed(() => {
  if (hoverIndex.value === null) return null
  const x = domainX.value[hoverIndex.value]
  if (x === undefined) return null
  if (useCategoricalX.value) {
    const band = xScale.value as ReturnType<typeof createBandScale>
    return band.position(x) + band.bandwidth() / 2
  }
  return scaleX(x)
})

interface DetailRow {
  title: string
  color: string
  markerType: 'line' | 'rectangle' | 'dashed'
  value: string
}

const hoverDetails = computed<DetailRow[]>(() => {
  if (hoverIndex.value === null) return []
  const x = domainX.value[hoverIndex.value]
  if (x === undefined) return []
  const rows: DetailRow[] = []
  series.value.forEach((item, index) => {
    if (isYThreshold(item)) {
      rows.push({
        title: item.title,
        color: colors.value[index],
        markerType: 'dashed',
        value: item.valueFormatter ? item.valueFormatter(item.y, x) : formatDefaultNumber(item.y),
      })
      return
    }
    if (isXThreshold(item)) {
      if (matchesX(item.x, x)) {
        rows.push({ title: item.title, color: colors.value[index], markerType: 'dashed', value: '' })
      }
      return
    }
    const datum = item.data.find((d) => matchesX(d.x, x))
    if (!datum) return
    rows.push({
      title: item.title,
      color: colors.value[index],
      markerType: item.type === 'line' ? 'line' : 'rectangle',
      value: item.valueFormatter ? item.valueFormatter(datum.y, x) : formatDefaultNumber(datum.y),
    })
  })
  return rows
})

const hoverTitle = computed(() => {
  if (hoverIndex.value === null) return ''
  const x = domainX.value[hoverIndex.value]
  return x === undefined ? '' : formatXTick(x)
})

const popoverRef = ref<HTMLElement | null>(null)
/** Measured popover size, so the panel can be clamped inside the plot instead of overflowing it. */
const popoverSize = ref({ width: 0, height: 0 })

watch(
  [hoverIndex, hoverDetails],
  async () => {
    if (hoverIndex.value === null) {
      popoverSize.value = { width: 0, height: 0 }
      return
    }
    await nextTick()
    const el = popoverRef.value
    if (!el) return
    popoverSize.value = { width: el.offsetWidth, height: el.offsetHeight }
  },
  { flush: 'post' }
)

const popoverStyle = computed(() => {
  const maxWidth = 260
  const areaWidth = containerWidth.value || maxWidth + 32
  const areaHeight = containerHeight.value || 0
  const { width, height } = popoverSize.value
  const gap = 16
  const margin = 8

  let left = pointer.value.x + gap
  if (left + width > areaWidth - margin) left = pointer.value.x - width - gap
  left = Math.min(Math.max(margin, left), Math.max(margin, areaWidth - width - margin))

  // Prefer below-right of the cursor, then flip up, then clamp — the popover must
  // stay fully visible inside the card even when the pointer is at the bottom edge.
  let top = pointer.value.y - 12
  if (height > 0) {
    if (top + height > areaHeight - margin) top = pointer.value.y - height + 12
    top = Math.min(Math.max(margin, top), Math.max(margin, areaHeight - height - margin))
  }

  return {
    left: `${left}px`,
    top: `${top}px`,
    maxWidth: `${Math.min(maxWidth, areaWidth - margin * 2)}px`,
  }
})

function onMouseMove(event: MouseEvent) {
  const svg = event.currentTarget as SVGSVGElement
  const rect = svg.getBoundingClientRect()
  const offsetX = event.clientX - rect.left - leftGutter.value
  if (domainX.value.length === 0) return

  let index = -1
  if (useCategoricalX.value) {
    const band = xScale.value as ReturnType<typeof createBandScale>
    let best = Infinity
    domainX.value.forEach((value, i) => {
      const center = band.position(value) + band.bandwidth() / 2
      const distance = Math.abs(center - offsetX)
      if (isFinite(center) && distance < best) {
        best = distance
        index = i
      }
    })
    // Only highlight when the pointer is reasonably close to a column.
    if (best > Math.max(band.step, 24)) index = -1
  } else {
    let best = Infinity
    domainX.value.forEach((value, i) => {
      const distance = Math.abs(scaleX(value) - offsetX)
      if (distance < best) {
        best = distance
        index = i
      }
    })
  }

  hoverIndex.value = index < 0 ? null : index
  pointer.value = { x: event.clientX - rect.left, y: event.clientY - rect.top }
}

function onMouseLeave() {
  hoverIndex.value = null
}

const rootClasses = computed(() =>
  cx(
    CHART.wrapper.wrapper,
    'cs-chart-root',
    props.fitHeight && CHART.wrapper['wrapper--fit-height'],
    props.fitHeight && 'cs-chart-root-fit'
  )
)

const plotTotalHeight = computed(() => plotHeight.value + bottomLabelsHeight.value)

function isDimmed(seriesTitle: string) {
  return hoveredSeriesTitle.value !== null && hoveredSeriesTitle.value !== seriesTitle
}

/** Same opacity treatment the official chart applies to non-highlighted series. */
function seriesClass(seriesTitle: string) {
  return isDimmed(seriesTitle) ? CHART.series['series--dimmed'] : ''
}
</script>

<template>
  <div ref="rootRef" :class="rootClasses">
    <!-- Axis titles sit outside the plot, matching the official AxisLabel placement. -->
    <div v-if="yTitle" :class="[CHART.cartesian['axis-label'], CHART.cartesian['axis-label--y'], 'cs-chart-axis-title']">
      {{ yTitle }}
    </div>
    <div ref="plotAreaRef" class="cs-chart-plot-area">
      <Spinner v-if="loading" size="normal" />
      <div v-else-if="series.length === 0 || !hasData" :class="cx(CHART.status.empty, 'cs-chart-empty')">
        {{ emptyText }}
      </div>
      <template v-else>
        <svg
          :width="Math.max(0, containerWidth)"
          :height="plotTotalHeight"
          :class="CHART.plot.root"
          role="application"
          :aria-label="ariaLabel"
          @mousemove="onMouseMove"
          @mouseleave="onMouseLeave"
        >
          <defs>
            <clipPath :id="clipId">
              <rect x="0" y="0" :width="plotWidth" :height="plotHeight" />
            </clipPath>
          </defs>

          <g :transform="`translate(${leftGutter}, 0)`">
            <!-- value axis grid + labels -->
            <g :class="CHART.cartesian['labels-inline-start']" aria-hidden="true">
              <g
                v-for="tick in yTicks.items"
                :key="`y-${tick.value}`"
                :transform="`translate(0,${tick.position})`"
                :class="[CHART.cartesian.ticks, CHART.cartesian['ticks--y']]"
              >
                <line
                  :class="[CHART.cartesian.grid, CHART.cartesian.ticks__line]"
                  :x1="-TICK_LENGTH"
                  y1="0"
                  :x2="plotWidth"
                  y2="0"
                />
                <text
                  :class="CHART.cartesian.ticks__text"
                  :x="-(TICK_LENGTH + TICK_MARGIN)"
                  y="0"
                  style="text-anchor: end; dominant-baseline: central"
                >
                  {{ tick.label }}
                </text>
              </g>
            </g>

            <!-- zero baseline -->
            <line
              v-if="emphasizeBaselineAxis && yScale.min <= 0 && yScale.max >= 0"
              :class="[CHART.cartesian.axis, CHART.cartesian['axis--emphasized']]"
              :x1="-TICK_MARGIN"
              :x2="plotWidth"
              :y1="yScale.scale(0)"
              :y2="yScale.scale(0)"
              aria-hidden="true"
            />

            <g :clip-path="`url(#${clipId})`">
              <!-- bars -->
              <g
                v-for="(shape, index) in barShapes"
                :key="`bar-${index}`"
                :class="[CHART.series.series, CHART.series['series--bar'], seriesClass(shape.title)]"
                aria-hidden="true"
              >
                <rect
                  v-if="shape.side === 'all' || shape.side === 'none'"
                  :class="CHART.series.series__rect"
                  :x="shape.x"
                  :y="shape.y"
                  :width="shape.width"
                  :height="shape.height"
                  :rx="shape.side === 'all' ? shape.rx : 0"
                  :fill="shape.color"
                />
                <path
                  v-else
                  :class="CHART.series.series__rect"
                  :d="createOneSideRoundedRectPath({ x: shape.x, y: shape.y, width: shape.width, height: shape.height }, shape.rx, shape.side)"
                  :fill="shape.color"
                />
              </g>

              <!-- lines -->
              <path
                v-for="shape in lineShapes"
                :key="`line-${shape.key}`"
                :class="[CHART.series.series, shape.dashed && CHART.series['series--threshold'], seriesClass(shape.title)]"
                :d="shape.path"
                :stroke="shape.color"
                aria-hidden="true"
              />

              <!-- y thresholds -->
              <line
                v-for="threshold in yThresholds"
                :key="`yth-${threshold.key}`"
                :class="[CHART.series.series, CHART.series['series--threshold']]"
                x1="0"
                :x2="plotWidth"
                :y1="threshold.y"
                :y2="threshold.y"
                :stroke="threshold.color"
                aria-hidden="true"
              />

              <!-- x thresholds -->
              <line
                v-for="threshold in xThresholds"
                :key="`xth-${threshold.key}`"
                :class="[CHART.series.series, CHART.series['series--threshold']]"
                :x1="threshold.x"
                :x2="threshold.x"
                y1="0"
                :y2="plotHeight"
                :stroke="threshold.color"
                aria-hidden="true"
              />
            </g>

            <!-- hover marker -->
            <template v-if="markerX !== null">
              <line
                :class="CHART.cartesian['vertical-marker']"
                :x1="markerX"
                :x2="markerX"
                y1="0"
                :y2="plotHeight"
                aria-hidden="true"
              />
              <circle
                v-for="point in hoveredPoints"
                :key="point.key"
                :class="CHART.cartesian['vertical-marker-circle']"
                :cx="point.x"
                :cy="point.y"
                r="4"
                :stroke="point.color"
                aria-hidden="true"
              />
            </template>

            <!-- category axis -->
            <g :transform="`translate(0,${plotHeight})`" :class="CHART.cartesian['labels-block-end']" aria-hidden="true">
              <g
                v-for="tick in xTicks"
                :key="`x-${tick.position}`"
                :transform="`translate(${tick.position},0)`"
                :class="[CHART.cartesian.ticks, CHART.cartesian['ticks--bottom']]"
              >
                <line :class="CHART.cartesian.ticks__line" x1="0" x2="0" y1="0" :y2="TICK_LENGTH" />
                <text
                  :class="CHART.cartesian.ticks__text"
                  x="0"
                  :y="TICK_LENGTH + TICK_MARGIN"
                >
                  {{ tick.label }}
                </text>
              </g>
            </g>
          </g>
        </svg>

        <div
          v-if="hoverIndex !== null && hoverDetails.length > 0"
          ref="popoverRef"
          class="cs-chart-popover"
          :style="popoverStyle"
        >
          <div :class="CHART.popoverBody['container-body']" role="tooltip">
            <div :class="[CHART.popoverBody.body, CHART.popoverBody['body-variant-chart']]">
              <div :class="CHART.popoverBody['header-row']">
                <div :class="CHART.popoverBody.header">
                  <h2>{{ hoverTitle }}</h2>
                </div>
              </div>
              <div :class="CHART.popoverBody.content">
                <ul :class="CHART.details.list">
                  <li
                    v-for="row in hoverDetails"
                    :key="row.title"
                    :class="[CHART.details['list-item'], isDimmed(row.title) && 'cs-chart-detail-dimmed']"
                  >
                    <div :class="CHART.details['key-value-pair']">
                      <div :class="CHART.details.key">
                        <ChartSeriesMarker :type="row.markerType" :color="row.color" />
                        <span>{{ row.title }}</span>
                      </div>
                      <span :class="CHART.details.value">{{ row.value }}</span>
                    </div>
                  </li>
                </ul>
                <slot name="popover-footer" :x="hoverIndex" />
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <div
      v-if="xTitle"
      :class="[CHART.cartesian['axis-label'], CHART.cartesian['axis-label--x'], 'cs-chart-axis-title', 'cs-chart-axis-title-x']"
    >
      {{ xTitle }}
    </div>

    <ChartLegend
      v-if="!hideLegend && hasData && !loading"
      :items="legendItems"
      :legend-title="legendTitle"
      :highlighted-title="hoveredSeriesTitle"
      @hover="hoveredSeriesTitle = $event"
    />
  </div>
</template>

<style scoped lang="scss">
.cs-chart-axis-title {
  color: var(--color-text-body-default-b2us04, #0f141a);
  font-weight: var(--font-weight-bold-xpar9m, 700);
  padding-block-end: var(--space-xs-m9b7jf, 8px);
}

.cs-chart-axis-title-x {
  padding-block: var(--space-xxs-3o1yoq, 4px) 0;
  text-align: center;
}

.cs-chart-plot-area {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1;
}

/* The plot width is measured from the DOM, so the root must never size itself
   from its content — otherwise a flex parent locks the first measured width. */
.cs-chart-root {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}

/* The official wrapper class carries `display: block` with `:not(#\9)`
   specificity, so the flex column needed for `fitHeight` must be forced. */
.cs-chart-root-fit {
  display: flex !important;
  flex-direction: column !important;
  block-size: 100% !important;
  min-block-size: 0 !important;
  overflow: hidden !important;
}

.cs-chart-plot-area > svg {
  display: block;
  max-width: 100%;
}

.cs-chart-empty {
  padding: 24px 0;
  text-align: center;
}

.cs-chart-popover {
  position: absolute;
  z-index: 20;
  pointer-events: none;
}

.cs-chart-detail-dimmed {
  opacity: 0.35;
}
</style>
