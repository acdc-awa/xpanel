<script setup lang="ts">
import { computed } from 'vue'

interface ViewportState {
  x: number
  y: number
  zoom: number
}

const props = defineProps<{
  viewport: ViewportState
  dimensions: { width: number; height: number }
}>()

const width = computed(() => props.dimensions?.width || 0)
const height = computed(() => props.dimensions?.height || 0)

// 世界原点 (0,0) 在画布视口内的屏幕坐标（0 * zoom + pan = pan）
const originX = computed(() => props.viewport.x)
const originY = computed(() => props.viewport.y)

const showYAxis = computed(() => originX.value >= -2 && originX.value <= width.value + 2)
const showXAxis = computed(() => originY.value >= -2 && originY.value <= height.value + 2)
const originVisible = computed(
  () =>
    originX.value >= -32 &&
    originX.value <= width.value + 16 &&
    originY.value >= -32 &&
    originY.value <= height.value + 16,
)
</script>

<template>
  <div class="topo-rulers" aria-hidden="true">
    <!-- X / Y 世界坐标主轴（贯穿视口，以交叉点指示原点） -->
    <div v-if="showXAxis" class="axis-line axis-x" :style="{ top: `${originY}px` }" />
    <div v-if="showYAxis" class="axis-line axis-y" :style="{ left: `${originX}px` }" />

    <!-- 世界原点交叉指示 -->
    <div
      v-if="originVisible"
      class="origin-indicator"
      :style="{ left: `${originX}px`, top: `${originY}px` }"
    >
      <span class="origin-dot" />
      <span class="origin-label">0,0</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.topo-rulers {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
  overflow: hidden;
}

.axis-line {
  position: absolute;
  background: var(--topo-axis-line);
}

.axis-x {
  left: 0;
  right: 0;
  height: 1.5px;
  transform: translateY(-50%);
}

.axis-y {
  top: 0;
  bottom: 0;
  width: 1.5px;
  transform: translateX(-50%);
}

.origin-indicator {
  position: absolute;
}

.origin-dot {
  position: absolute;
  left: 0;
  top: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  transform: translate(-50%, -50%);
  background: var(--topo-accent);
  box-shadow: 0 0 0 2px var(--topo-canvas-bg);
}

.origin-label {
  position: absolute;
  left: 7px;
  top: 5px;
  font-size: 10px;
  line-height: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--topo-accent);
  white-space: nowrap;
}
</style>
