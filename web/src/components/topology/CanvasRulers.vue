<script setup lang="ts">
import { computed } from 'vue'

interface ViewportState {
  x: number
  y: number
  zoom: number
}

interface SelectionInfo {
  kind: 'server' | 'ap'
  name: string
  x: number
  y: number
  w: number
  h: number
}

const props = defineProps<{
  viewport: ViewportState
  dimensions: { width: number; height: number }
  selection?: SelectionInfo | null
}>()

/** 标尺厚度：与 CSS 中的 20px 保持一致。 */
const RULER = 20

const zoom = computed(() => props.viewport.zoom || 1)
const width = computed(() => props.dimensions?.width || 0)
const height = computed(() => props.dimensions?.height || 0)

const worldLeft = computed(() => (0 - props.viewport.x) / zoom.value)
const worldTop = computed(() => (0 - props.viewport.y) / zoom.value)
const worldRight = computed(() => (width.value - props.viewport.x) / zoom.value)
const worldBottom = computed(() => (height.value - props.viewport.y) / zoom.value)

/** 主刻度步长：保证屏上间距落在 56–160px，缩放极端时自动增减。 */
const majorStep = computed(() => {
  let step = 80
  while (step * zoom.value < 56) step *= 2
  while (step * zoom.value > 160 && step > 8) step /= 2
  return step
})

interface Tick {
  world: number
  pos: number
  major: boolean
}

function buildTicks(min: number, max: number, step: number): Tick[] {
  const ticks: Tick[] = []
  const minor = step / 10
  const showMinor = minor * zoom.value >= 6
  const start = Math.floor(min / step) * step
  for (let w = start; w <= max; w += step) {
    if (w >= min) ticks.push({ world: w, pos: (w - min) * zoom.value, major: true })
    if (showMinor) {
      for (let i = 1; i < 10; i++) {
        const mw = w + minor * i
        if (mw >= min && mw <= max) {
          ticks.push({ world: mw, pos: (mw - min) * zoom.value, major: false })
        }
      }
    }
  }
  return ticks
}

const xTicks = computed(() => buildTicks(worldLeft.value, worldRight.value, majorStep.value))
const yTicks = computed(() => buildTicks(worldTop.value, worldBottom.value, majorStep.value))

// 世界原点 (0,0) 在屏幕上的位置
const originX = computed(() => props.viewport.x)
const originY = computed(() => props.viewport.y)
const originVisible = computed(
  () =>
    originX.value >= RULER - 24 &&
    originX.value <= width.value &&
    originY.value >= RULER - 24 &&
    originY.value <= height.value,
)

function fmt(v: number): string {
  const n = Math.round(v)
  return n === 0 ? '0' : String(n)
}
</script>

<template>
  <div class="topo-rulers" aria-hidden="true">
    <!-- 左上角：当前视口左上角的世界坐标 + 缩放 -->
    <div class="ruler-corner">
      <span class="corner-zoom">{{ Math.round(zoom * 100) }}%</span>
      <span class="corner-coord">x {{ fmt(worldLeft) }}<br />y {{ fmt(worldTop) }}</span>
    </div>

    <!-- 顶部标尺 -->
    <div class="ruler ruler-x">
      <div
        v-for="t in xTicks"
        :key="`x-${t.world}`"
        class="tick"
        :class="{ major: t.major }"
        :style="{ left: `${t.pos + RULER}px` }"
      >
        <span v-if="t.major" class="tick-label">{{ fmt(t.world) }}</span>
      </div>
    </div>

    <!-- 左侧标尺 -->
    <div class="ruler ruler-y">
      <div
        v-for="t in yTicks"
        :key="`y-${t.world}`"
        class="tick"
        :class="{ major: t.major }"
        :style="{ top: `${t.pos + RULER}px` }"
      >
        <span v-if="t.major" class="tick-label">{{ fmt(t.world) }}</span>
      </div>
    </div>

    <!-- 世界原点标记 -->
    <div
      v-if="originVisible"
      class="origin-mark"
      :style="{ left: `${originX}px`, top: `${originY}px` }"
    >
      <span class="origin-label">0,0</span>
    </div>

    <!-- 底部坐标读数 -->
    <div class="coord-bar">
      <template v-if="selection">
        <span class="coord-kind">{{ selection.kind === 'ap' ? '接入点' : '服务器' }}</span>
        <span class="coord-name">{{ selection.name }}</span>
        <span class="coord-sep">·</span>
        <span class="coord-mono">x {{ selection.x }}</span>
        <span class="coord-mono">y {{ selection.y }}</span>
        <span class="coord-mono">{{ selection.w }} × {{ selection.h }}</span>
      </template>
      <template v-else>
        <span class="coord-mono">x {{ fmt(worldLeft) }} · y {{ fmt(worldTop) }}</span>
        <span class="coord-sep">·</span>
        <span class="coord-mono">{{ Math.round(zoom * 100) }}%</span>
      </template>
    </div>
  </div>
</template>

<style scoped lang="scss">
.topo-rulers {
  position: absolute;
  inset: 0;
  z-index: 15;
  pointer-events: none;
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-size: 10px;
  color: #64748b;
}

.ruler-corner {
  position: absolute;
  left: 0;
  top: 0;
  width: 20px;
  height: 20px;
  overflow: hidden;
  background: rgba(15, 23, 42, 0.94);
  border-right: 1px solid rgba(148, 163, 184, 0.16);
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  display: flex;
  align-items: center;
  justify-content: center;

  .corner-zoom {
    display: none;
  }
  .corner-coord {
    font-size: 7px;
    line-height: 8px;
    text-align: center;
    letter-spacing: -0.2px;
    color: #475569;
  }
}

.ruler {
  position: absolute;
  background: rgba(15, 23, 42, 0.78);
  border-color: rgba(148, 163, 184, 0.16);
}
.ruler-x {
  left: 20px;
  right: 0;
  top: 0;
  height: 20px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}
.ruler-y {
  top: 20px;
  bottom: 0;
  left: 0;
  width: 20px;
  border-right: 1px solid rgba(148, 163, 184, 0.16);
}

.tick {
  position: absolute;
  background: rgba(148, 163, 184, 0.22);

  &.major {
    background: rgba(148, 163, 184, 0.4);
  }
}
.ruler-x .tick {
  top: 14px;
  width: 1px;
  height: 6px;
  &.major {
    top: 10px;
    height: 10px;
  }
  .tick-label {
    position: absolute;
    left: 3px;
    top: -10px;
    white-space: nowrap;
    font-size: 9px;
    line-height: 10px;
    color: #64748b;
  }
}
.ruler-y .tick {
  left: 14px;
  height: 1px;
  width: 6px;
  &.major {
    left: 10px;
    width: 10px;
  }
  .tick-label {
    position: absolute;
    top: 1px;
    left: -16px;
    width: 24px;
    text-align: right;
    white-space: nowrap;
    font-size: 9px;
    line-height: 10px;
    color: #64748b;
    transform: rotate(-90deg);
  }
}

.origin-mark {
  position: absolute;
  width: 13px;
  height: 13px;
  margin-left: -6px;
  margin-top: -6px;
  border-left: 1px solid rgba(56, 189, 248, 0.55);
  border-top: 1px solid rgba(56, 189, 248, 0.55);

  .origin-label {
    position: absolute;
    left: 8px;
    top: 4px;
    font-size: 9px;
    color: rgba(56, 189, 248, 0.6);
    white-space: nowrap;
  }
}

.coord-bar {
  position: absolute;
  left: 28px;
  bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.9);
  border: 1px solid rgba(148, 163, 184, 0.16);
  color: #94a3b8;

  .coord-kind {
    color: #64748b;
  }
  .coord-name {
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #cbd5e1;
  }
  .coord-sep {
    color: #475569;
  }
  .coord-mono {
    color: #7dd3fc;
  }
}
</style>
