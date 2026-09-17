<script setup lang="ts">
import { ref, watch, nextTick, computed, onMounted, onUnmounted } from 'vue'
import { ZoomIn, ZoomOut, RefreshRight, RefreshLeft, Check, Close, Refresh } from '@element-plus/icons-vue'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    imageSrc: string
    title?: string
    targetSize?: number
  }>(),
  {
    title: '裁剪图片',
    targetSize: 256,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'crop', dataUrl: string): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const canvasRef = ref<HTMLCanvasElement | null>(null)
const previewCanvasRef = ref<HTMLCanvasElement | null>(null)

// 视图与变换状态
const scale = ref(1)
const minScale = ref(0.2)
const maxScale = ref(5)
const rotation = ref(0) // 0, 90, 180, 270
const offsetX = ref(0)
const offsetY = ref(0)

const isDragging = ref(false)
const dragStart = { x: 0, y: 0 }
const dragOffsetStart = { x: 0, y: 0 }

const imgObj = ref<HTMLImageElement | null>(null)
const imageLoaded = ref(false)
const estimatedKb = ref(0)

// 裁剪舞台与裁剪框的基准尺寸（逻辑像素，正方形）。
// 裁剪框固定占舞台的 CROP_BOX_SIZE / STAGE_SIZE，窄视口下按同一比例一起收缩：
// 原来是两个写死的 340 / 260，≤360px 视口里弹窗被压到 94vw，340px 的舞台右侧
// 被 .el-dialog 的 overflow:hidden 裁掉约 34px，裁剪框也不再居中。
const STAGE_SIZE = 340
const CROP_BOX_SIZE = 260
const STAGE_MIN = 240
// 小预览的 CSS 尺寸（后备缓冲再乘 dpr）
const PREVIEW_SIZE = 32

const stageSize = ref(STAGE_SIZE)
const cropBoxSize = computed(() => Math.round((stageSize.value * CROP_BOX_SIZE) / STAGE_SIZE))

// canvas 后备缓冲必须按设备像素比放大：直接用 CSS 像素当 width/height，
// Retina 屏和浏览器 125%/150% 缩放下裁剪预览与 64px 小预览都会发虚。
// 上限取 3，避免 4K/5K 屏上无意义地放大内存占用。
const dpr = ref(typeof window !== 'undefined' ? Math.min(window.devicePixelRatio || 1, 3) : 1)

function syncStageSize() {
  if (typeof window === 'undefined') return
  // 弹窗宽 ≤94vw，body 左右各 18px 内边距，再留 8px 余量
  const avail = Math.floor(window.innerWidth * 0.94) - 44
  const next = Math.max(STAGE_MIN, Math.min(STAGE_SIZE, avail))
  if (next === stageSize.value) return

  // 按裁剪框的缩放比例等比换算，保住用户已经调好的构图。
  // 这里不能直接 resetTransform：移动端软键盘弹出会触发 resize，
  // 那样会把用户刚拖好的裁剪位置清掉。
  const prevBox = cropBoxSize.value
  stageSize.value = next
  const k = cropBoxSize.value / prevBox
  if (imgObj.value && Number.isFinite(k) && k > 0) {
    scale.value *= k
    minScale.value *= k
    maxScale.value *= k
    offsetX.value *= k
    offsetY.value *= k
  }

  nextTick(() => {
    draw()
    updatePreview()
  })
}

function syncDpr() {
  const next = Math.min(window.devicePixelRatio || 1, 3)
  if (next === dpr.value) return
  dpr.value = next
  draw()
  updatePreview()
}

function onViewportChange() {
  syncStageSize()
  syncDpr()
}

onMounted(() => {
  syncStageSize()
  window.addEventListener('resize', onViewportChange)
})

onUnmounted(() => {
  window.removeEventListener('resize', onViewportChange)
})

watch(
  () => [props.modelValue, props.imageSrc],
  ([val, src]) => {
    if (val && src) {
      syncStageSize()
      syncDpr()
      loadImage(src as string)
    } else {
      imageLoaded.value = false
    }
  }
)

function loadImage(src: string) {
  const img = new Image()
  img.crossOrigin = 'anonymous'
  img.onload = () => {
    imgObj.value = img
    imageLoaded.value = true
    resetTransform()
    nextTick(() => {
      draw()
      updatePreview()
    })
  }
  img.src = src
}

function resetTransform() {
  if (!imgObj.value) return
  const { width, height } = imgObj.value
  const fitScale = Math.max(cropBoxSize.value / width, cropBoxSize.value / height)
  scale.value = fitScale
  minScale.value = fitScale * 0.5
  maxScale.value = fitScale * 4
  offsetX.value = 0
  offsetY.value = 0
  rotation.value = 0
}

function rotate(deg: number) {
  rotation.value = (rotation.value + deg + 360) % 360
  draw()
  updatePreview()
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  const factor = e.deltaY < 0 ? 1.08 : 0.92
  const next = Math.max(minScale.value, Math.min(maxScale.value, scale.value * factor))
  scale.value = Number(next.toFixed(3))
  draw()
  updatePreview()
}

function onMouseDown(e: MouseEvent) {
  isDragging.value = true
  dragStart.x = e.clientX
  dragStart.y = e.clientY
  dragOffsetStart.x = offsetX.value
  dragOffsetStart.y = offsetY.value
}

function onMouseMove(e: MouseEvent) {
  if (!isDragging.value) return
  const dx = e.clientX - dragStart.x
  const dy = e.clientY - dragStart.y
  offsetX.value = dragOffsetStart.x + dx
  offsetY.value = dragOffsetStart.y + dy
  draw()
  updatePreview()
}

function onMouseUp() {
  isDragging.value = false
}

// 触摸事件
function onTouchStart(e: TouchEvent) {
  if (e.touches.length === 1) {
    isDragging.value = true
    dragStart.x = e.touches[0].clientX
    dragStart.y = e.touches[0].clientY
    dragOffsetStart.x = offsetX.value
    dragOffsetStart.y = offsetY.value
  }
}

function onTouchMove(e: TouchEvent) {
  if (!isDragging.value || e.touches.length !== 1) return
  const dx = e.touches[0].clientX - dragStart.x
  const dy = e.touches[0].clientY - dragStart.y
  offsetX.value = dragOffsetStart.x + dx
  offsetY.value = dragOffsetStart.y + dy
  draw()
  updatePreview()
}

function draw() {
  const canvas = canvasRef.value
  if (!canvas || !imgObj.value) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  ctx.save()

  // 后备缓冲是 stageSize × dpr，先把坐标系换算回 CSS 像素，
  // 之后所有几何量都按 stageSize 计算，与裁剪框、导出比例保持同一套口径
  ctx.scale(dpr.value, dpr.value)

  // 移至画布中心
  ctx.translate(stageSize.value / 2 + offsetX.value, stageSize.value / 2 + offsetY.value)
  ctx.rotate((rotation.value * Math.PI) / 180)
  ctx.scale(scale.value, scale.value)

  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(imgObj.value, -imgObj.value.width / 2, -imgObj.value.height / 2)
  ctx.restore()
}

function generateCroppedDataUrl(): string {
  if (!imgObj.value) return ''
  const size = props.targetSize || 256
  const cropCanvas = document.createElement('canvas')
  cropCanvas.width = size
  cropCanvas.height = size
  const ctx = cropCanvas.getContext('2d')
  if (!ctx) return ''

  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'

  const ratio = size / cropBoxSize.value
  ctx.translate(size / 2 + offsetX.value * ratio, size / 2 + offsetY.value * ratio)
  ctx.rotate((rotation.value * Math.PI) / 180)
  ctx.scale(scale.value * ratio, scale.value * ratio)
  ctx.drawImage(imgObj.value, -imgObj.value.width / 2, -imgObj.value.height / 2)

  return cropCanvas.toDataURL('image/png')
}

function updatePreview() {
  const previewCanvas = previewCanvasRef.value
  if (!previewCanvas || !imgObj.value) return
  const ctx = previewCanvas.getContext('2d')
  if (!ctx) return

  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, previewCanvas.width, previewCanvas.height)
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.scale(dpr.value, dpr.value)

  const ratio = PREVIEW_SIZE / cropBoxSize.value
  ctx.save()
  ctx.translate(PREVIEW_SIZE / 2 + offsetX.value * ratio, PREVIEW_SIZE / 2 + offsetY.value * ratio)
  ctx.rotate((rotation.value * Math.PI) / 180)
  ctx.scale(scale.value * ratio, scale.value * ratio)
  ctx.drawImage(imgObj.value, -imgObj.value.width / 2, -imgObj.value.height / 2)
  ctx.restore()

  // 计算输出 Base64 大小
  const dataUrl = generateCroppedDataUrl()
  estimatedKb.value = Math.round((dataUrl.length * 0.75) / 1024)
}

function handleConfirm() {
  const dataUrl = generateCroppedDataUrl()
  emit('crop', dataUrl)
  visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="420px"
    destroy-on-close
    append-to-body
    :close-on-click-modal="false"
    class="image-cropper-dialog"
  >
    <div class="cropper-container">
      <div
        class="cropper-stage"
        :style="{ width: stageSize + 'px', height: stageSize + 'px' }"
        @mousedown="onMouseDown"
        @mousemove="onMouseMove"
        @mouseup="onMouseUp"
        @mouseleave="onMouseUp"
        @touchstart.passive="onTouchStart"
        @touchmove.passive="onTouchMove"
        @touchend="onMouseUp"
        @wheel="onWheel"
      >
        <canvas
          ref="canvasRef"
          :width="stageSize * dpr"
          :height="stageSize * dpr"
          class="cropper-canvas"
        />

        <!-- 裁剪遮罩与裁剪框 -->
        <div class="cropper-mask">
          <div
            class="crop-box"
            :style="{ width: cropBoxSize + 'px', height: cropBoxSize + 'px' }"
          >
            <div class="crop-grid">
              <span class="grid-line h1" />
              <span class="grid-line h2" />
              <span class="grid-line v1" />
              <span class="grid-line v2" />
            </div>
          </div>
        </div>
      </div>

      <!-- 控制工具栏 -->
      <div class="cropper-controls">
        <div class="zoom-slider-row">
          <el-icon :size="16" class="ctrl-icon"><ZoomOut /></el-icon>
          <el-slider
            v-model="scale"
            :min="minScale"
            :max="maxScale"
            :step="0.02"
            :show-tooltip="false"
            @input="() => { draw(); updatePreview() }"
          />
          <el-icon :size="16" class="ctrl-icon"><ZoomIn /></el-icon>
        </div>

        <div class="action-btn-row">
          <el-button-group size="small">
            <el-button :icon="RefreshLeft" @click="rotate(-90)">向左旋转</el-button>
            <el-button :icon="RefreshRight" @click="rotate(90)">向右旋转</el-button>
            <el-button :icon="Refresh" @click="() => { resetTransform(); draw(); updatePreview() }">复位</el-button>
          </el-button-group>

          <div class="preview-mini">
            <canvas
              ref="previewCanvasRef"
              :width="PREVIEW_SIZE * dpr"
              :height="PREVIEW_SIZE * dpr"
              class="preview-canvas"
            />
            <div class="size-hint">
              <span>{{ targetSize }}×{{ targetSize }}</span>
              <span class="kb-text">~{{ estimatedKb }} KB</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button :icon="Close" @click="visible = false">取消</el-button>
        <el-button type="primary" :icon="Check" @click="handleConfirm">
          完成裁剪并应用
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.cropper-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.cropper-stage {
  position: relative;
  border-radius: var(--x-radius, 12px);
  overflow: hidden;
  background: #1e1e24;
  cursor: grab;
  user-select: none;
  touch-action: none;
  box-shadow: inset 0 0 10px rgba(0, 0, 0, 0.4);

  &:active {
    cursor: grabbing;
  }
}

.cropper-canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.cropper-mask {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.55);
}

.crop-box {
  position: relative;
  border: 2px solid var(--el-color-primary, #6366f1);
  border-radius: var(--x-radius-lg, 14px);
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.55);
  box-sizing: border-box;
}

.crop-grid {
  width: 100%;
  height: 100%;
  position: relative;

  .grid-line {
    position: absolute;
    background: rgba(255, 255, 255, 0.25);

    &.h1 { top: 33.33%; left: 0; right: 0; height: 1px; }
    &.h2 { top: 66.66%; left: 0; right: 0; height: 1px; }
    &.v1 { left: 33.33%; top: 0; bottom: 0; width: 1px; }
    &.v2 { left: 66.66%; top: 0; bottom: 0; width: 1px; }
  }
}

.cropper-controls {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.zoom-slider-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 4px;

  .ctrl-icon {
    color: var(--x-text-2, #64748b);
  }

  .el-slider {
    flex: 1;
  }
}

.action-btn-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.preview-mini {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--x-bg, #f8fafc);
  padding: 4px 8px;
  border-radius: var(--x-radius-sm, 8px);
  border: 1px solid var(--x-border, #e2e8f0);
}

.preview-canvas {
  width: 32px;
  height: 32px;
  border-radius: var(--x-radius-xs, 6px);
  background: #fff;
  border: 1px solid rgba(0, 0, 0, 0.08);
}

.size-hint {
  display: flex;
  flex-direction: column;
  font-size: 10.5px;
  line-height: 1.2;
  color: var(--x-text-3, #94a3b8);

  .kb-text {
    color: var(--el-color-primary, #6366f1);
    font-weight: 600;
  }
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
