<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface TabItem {
  id: string
  label: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    activeTabId?: string
    modelValue?: string
    tabs: TabItem[]
    ariaLabel?: string
    disableContentPaddings?: boolean
  }>(),
  { activeTabId: undefined, modelValue: undefined, ariaLabel: undefined, disableContentPaddings: false }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', detail: { activeTabId: string }): void
}>()

const activeId = computed(() => props.activeTabId ?? props.modelValue ?? props.tabs[0]?.id ?? '')
const activeIndex = computed(() => Math.max(0, props.tabs.findIndex((tab) => tab.id === activeId.value)))

const listRef = ref<HTMLElement | null>(null)
const startOverflow = ref(false)
const endOverflow = ref(false)

/** 官方在表头两侧渲染翻页按钮，仅当该方向还有未显示页签时才可点。 */
function measureOverflow() {
  const list = listRef.value
  if (!list) return
  const max = list.scrollWidth - list.clientWidth
  startOverflow.value = list.scrollLeft > 1
  endOverflow.value = max > 1 && list.scrollLeft < max - 1
}

function scrollPage(direction: 'backward' | 'forward') {
  const list = listRef.value
  if (!list) return
  const step = Math.max(80, Math.round(list.clientWidth * 0.8))
  list.scrollBy({ left: direction === 'forward' ? step : -step, behavior: 'smooth' })
}

let observer: ResizeObserver | null = null
onMounted(() => {
  measureOverflow()
  if (listRef.value && typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(measureOverflow)
    observer.observe(listRef.value)
  }
  window.addEventListener('resize', measureOverflow)
})
onBeforeUnmount(() => {
  observer?.disconnect()
  window.removeEventListener('resize', measureOverflow)
})

/** 选中页签变化时把该页签滚进可视区（官方 scrollIntoViewIfPossible 的等价行为）。 */
function scrollActiveIntoView() {
  nextTick(() => {
    const list = listRef.value
    if (!list) return
    const active = list.querySelector<HTMLElement>(`[aria-controls="tabpanel-${activeId.value}"]`)
    active?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    measureOverflow()
  })
}

function select(tab: TabItem) {
  if (tab.disabled || tab.id === activeId.value) return
  emit('update:modelValue', tab.id)
  emit('change', { activeTabId: tab.id })
  scrollActiveIntoView()
}

function onKeydown(event: KeyboardEvent) {
  const enabled = props.tabs.filter((tab) => !tab.disabled)
  if (!enabled.length) return
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return
  event.preventDefault()
  const current = enabled.findIndex((tab) => tab.id === activeId.value)
  let next = current
  if (event.key === 'ArrowLeft') next = (current - 1 + enabled.length) % enabled.length
  if (event.key === 'ArrowRight') next = (current + 1) % enabled.length
  if (event.key === 'Home') next = 0
  if (event.key === 'End') next = enabled.length - 1
  select(enabled[next])
}
</script>

<template>
  <div :class="[CS.tabs.root, 'cs-tabs-root']">
    <div :class="[CS.tabs.tabs, 'cs-tabs-body']">
      <div :class="cx(CS.tabs['tabs-header'], CS.tabs.refresh, CS.tabs['tabs-header-with-divider'])">
        <div :class="CS.tabs['tab-header-scroll-container']">
          <!-- 官方实现：只有表头溢出时才渲染左右翻页按钮（窄屏 7 个页签会用到） -->
          <span
            v-if="startOverflow || endOverflow"
            :class="
              cx(
                CS.tabs['pagination-button'],
                CS.tabs['pagination-button-left'],
                startOverflow && CS.tabs['pagination-button-left-scrollable']
              )
            "
          >
            <Button
              variant="icon"
              icon-name="angle-left"
              :disabled="!startOverflow"
              aria-label="向左滚动页签"
              @click="scrollPage('backward')"
            />
          </span>
          <ul
            ref="listRef"
            :class="CS.tabs['tabs-header-list']"
            role="tablist"
            :aria-label="ariaLabel"
            @keydown="onKeydown"
            @scroll="measureOverflow"
          >
            <li v-for="(tab, index) in tabs" :key="tab.id" :class="CS.tabs['tabs-tab']" role="presentation">
              <div
                :class="
                  cx(
                    CS.tabs['tabs-tab-header-container'],
                    CS.tabs.refresh,
                    tab.id === activeId && CS.tabs['tabs-tab-active'],
                    tab.disabled && CS.tabs['tabs-tab-disabled'],
                    tab.id === activeId && CS.tabs['tabs-tab-focusable']
                  )
                "
              >
                <button
                  type="button"
                  role="tab"
                  :class="
                    cx(
                      CS.tabs['tabs-tab-link'],
                      CS.tabs.refresh,
                      tab.id === activeId && CS.tabs['tabs-tab-active'],
                      tab.disabled && CS.tabs['tabs-tab-disabled'],
                      tab.id === activeId && CS.tabs['tabs-tab-focusable']
                    )
                  "
                  :aria-selected="tab.id === activeId ? 'true' : 'false'"
                  :aria-controls="`tabpanel-${tab.id}`"
                  :tabindex="index === activeIndex ? 0 : -1"
                  :disabled="tab.disabled"
                  @click="select(tab)"
                >
                  <span :class="CS.tabs['tabs-tab-label']"><span>{{ tab.label }}</span></span>
                </button>
              </div>
            </li>
          </ul>
          <span
            v-if="startOverflow || endOverflow"
            :class="
              cx(
                CS.tabs['pagination-button'],
                CS.tabs['pagination-button-right'],
                endOverflow && CS.tabs['pagination-button-right-scrollable']
              )
            "
          >
            <Button
              variant="icon"
              icon-name="angle-right"
              :disabled="!endOverflow"
              aria-label="向右滚动页签"
              @click="scrollPage('forward')"
            />
          </span>
        </div>
        <div v-if="$slots.actions" :class="CS.tabs['actions-container']">
          <slot name="actions" />
        </div>
      </div>
      <!-- 官方结构是「外层 wrapper 持 with-paddings，内层 content 持 tabs-content-active」；
           vendored CSS 的内边距选择器写死为 `.tabs-content-wrapper.with-paddings > .tabs-content`，
           两层顺序颠倒会让 padding-block 完全不匹配（每个页签内容顶到表头分割线上）。 -->
      <div :class="cx(CS.tabs['tabs-content-wrapper'], !disableContentPaddings && CS.tabs['with-paddings'])">
        <div
          :class="cx(CS.tabs['tabs-content'], CS.tabs['tabs-content-active'])"
          :id="`tabpanel-${activeId}`"
          role="tabpanel"
        >
          <slot />
        </div>
      </div>
    </div>
  </div>
</template>
