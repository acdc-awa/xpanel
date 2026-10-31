<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useSlots } from 'vue'
import TopNavigationUtility from './TopNavigationUtility.vue'
import { CS } from './classes'
import { cx } from './utils'
import type { TopNavigationIdentity, TopNavigationUtility as TopNavigationUtilityItem } from './top-navigation-utils'

defineOptions({ name: 'TopNavigation' })

const props = withDefaults(
  defineProps<{
    identity?: TopNavigationIdentity
    utilities?: TopNavigationUtilityItem[]
    visualContext?: 'top-navigation' | 'none'
    searchMaxWidth?: string
  }>(),
  {
    identity: undefined,
    utilities: () => [],
    visualContext: 'none',
    searchMaxWidth: undefined,
  }
)

const emit = defineEmits<{
  (e: 'identityFollow', detail: { href: string }, event: MouseEvent): void
}>()

const slots = useSlots()
const cs = CS['top-navigation']

const rootRef = ref<HTMLElement | null>(null)
const sizeMode = ref<'wide' | 'medium' | 'narrow'>('wide')
let resizeObserver: ResizeObserver | null = null

function updateSizeMode() {
  const width = rootRef.value?.getBoundingClientRect().width ?? 0
  if (width === 0) return
  sizeMode.value = width < 560 ? 'narrow' : width < 900 ? 'medium' : 'wide'
}

onMounted(() => {
  const el = rootRef.value
  if (!el) return
  updateSizeMode()
  resizeObserver = new ResizeObserver(updateSizeMode)
  resizeObserver.observe(el)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})

const rootClasses = computed(() =>
  cx(
    cs['top-navigation'],
    sizeMode.value === 'medium' && cs.medium,
    sizeMode.value === 'narrow' && cs.narrow
  )
)

const visibleUtilities = computed(() =>
  props.utilities.filter((item) => !(item.collapseAt === 'narrow' && sizeMode.value === 'narrow'))
)

const hideText = computed(() => sizeMode.value !== 'wide')

const hasExtraSlot = computed(() => Boolean(slots['utility-extra']))

function offsetRightFor(index: number): 'none' | 'l' | 'xxl' | undefined {
  if (index !== visibleUtilities.value.length - 1) return undefined
  if (sizeMode.value !== 'wide') return 'l'
  return hasExtraSlot.value ? 'l' : 'xxl'
}

function onIdentityClick(event: MouseEvent) {
  if (!props.identity) return
  if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  emit('identityFollow', { href: props.identity.href }, event)
}
</script>

<template>
  <div
    ref="rootRef"
    :class="visualContext === 'top-navigation' ? 'awsui-context-top-navigation' : undefined"
  >
    <header :class="rootClasses">
      <div :class="cs['padding-box']">
        <div
          v-if="$slots.left || identity"
          :class="cx(cs.identity, !identity?.logo && cs['no-logo'])"
        >
          <slot name="left">
            <a
              v-if="identity"
              :class="cs['identity-link']"
              :href="identity.href"
              @click="onIdentityClick"
            >
              <img
                v-if="identity.logo"
                :class="cx(cs.logo, sizeMode === 'narrow' && cs.narrow)"
                :src="identity.logo.src"
                :alt="identity.logo.alt || ''"
              />
              <span v-else-if="identity.mark" class="cs-topnav-mark" aria-hidden="true">{{ identity.mark }}</span>
              <span v-if="identity.title" :class="cx(cs.title, 'cs-topnav-name')">{{ identity.title }}</span>
            </a>
          </slot>
        </div>
        <slot name="title" />

        <div
          v-if="$slots.search"
          :class="cs.inputs"
          :style="searchMaxWidth ? { maxInlineSize: searchMaxWidth } : undefined"
        >
          <div :class="cs.search">
            <slot name="search" />
          </div>
        </div>

        <div :class="cs.utilities">
          <TopNavigationUtility
            v-for="(item, index) in visibleUtilities"
            :key="index"
            :item="item"
            :hide-text="hideText"
            :offset-right="offsetRightFor(index)"
          />
          <span v-if="hasExtraSlot" class="cs-topnav-extra">
            <slot name="utility-extra" />
          </span>
        </div>
      </div>
    </header>
  </div>
</template>

<style scoped lang="scss">
.cs-topnav-extra {
  display: flex;
  align-items: center;
  padding-inline: var(--space-m-n8iq8o, 16px);
}

/* 没有图片 logo 时的字母标记，与旧外壳的渐变徽标保持同一观感。 */
.cs-topnav-mark {
  inline-size: 28px;
  block-size: 28px;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-body-s-89y7qg, 12px);
  font-weight: 800;
  flex: none;
}

/* 站点标题在所有宽度都保留：移动端抽屉改为从应用栏下沿展开后，顶栏成了唯一常驻的
   品牌位置，不能再让位（vendor 的 title 规则已带 nowrap + ellipsis，极窄时自动收缩）。 */

/* 窄屏三枚工具按钮（主题 / 用户端 / 头像是并排的）：vendor 给的 16px 两侧内边距
   在手机上太奢侈，收到 10px 才不会把品牌名挤成省略号。 */
@media (max-width: $bp-tablet-max) {
  :deep([class*='utility-wrapper']) {
    padding-inline: 10px !important;
  }

  .cs-topnav-extra {
    padding-inline: 10px;
  }
}

</style>
