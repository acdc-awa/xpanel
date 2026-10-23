<script setup lang="ts">
/**
 * 应用栏（官方 visual-refresh-toolbar 的 universal toolbar）。
 *
 * 官方控制台的层级是两条 bar：
 *   1. 站点级顶栏（品牌 / 账号 / 主题）——本项目用 `CsTopNavigation`；
 *   2. 应用栏（汉堡 + 面包屑，横跨全宽）——就是本组件。
 * 侧栏与内容区都从应用栏下方开始。
 *
 * 只渲染 nav / breadcrumbs / drawers 三个区域，与 vendor 的 toolbar CSS 对应。
 */
import { computed, useSlots } from 'vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

defineOptions({ name: 'AppToolbar' })

const props = withDefaults(
  defineProps<{
    navigationLabel?: string
    /** 固定在站点顶栏下方：与 `CsTopNavigation` 的高度一致 */
    stickyTop?: number | string
    height?: number
  }>(),
  {
    navigationLabel: '打开导航菜单',
    stickyTop: 56,
    height: 56,
  }
)

const emit = defineEmits<{
  (e: 'navigationClick', event: MouseEvent): void
}>()

const slots = useSlots()
const cs = CS['app-toolbar']
const trigger = CS['app-toolbar/trigger']

const style = computed(() => ({
  '--cs-app-toolbar-height': `${props.height}px`,
  '--cs-app-toolbar-top': typeof props.stickyTop === 'number' ? `${props.stickyTop}px` : props.stickyTop,
}))
</script>

<template>
  <div :class="[cs['universal-toolbar'], 'cs-app-toolbar']" :style="style">
    <div :class="cs['toolbar-container']">
      <nav :class="cs['universal-toolbar-nav']">
        <button
          type="button"
          :class="cx(trigger.trigger, trigger.default)"
          :aria-label="navigationLabel"
          :title="navigationLabel"
          @click="emit('navigationClick', $event)"
        >
          <Icon name="menu" />
        </button>
      </nav>

      <div :class="cs['universal-toolbar-breadcrumbs']">
        <slot />
      </div>

      <div v-if="slots.tools" :class="cs['universal-toolbar-drawers']">
        <slot name="tools" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.cs-app-toolbar {
  block-size: var(--cs-app-toolbar-height, 56px);
  inset-block-start: var(--cs-app-toolbar-top, 56px);
  border-block-end: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
}
</style>
