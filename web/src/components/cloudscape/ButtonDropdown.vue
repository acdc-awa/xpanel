<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Button from './Button.vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface ButtonDropdownItem {
  id: string | number
  text: string
  disabled?: boolean
  iconName?: string
  dividerBefore?: boolean
}

const props = withDefaults(
  defineProps<{
    items: ButtonDropdownItem[]
    /** 非图标模式下显示在按钮上的文字。 */
    text?: string
    /** 只渲染图标按钮（无文字、无下拉箭头），用于卡片角标菜单。 */
    iconOnly?: boolean
    /** 图标模式下的图标名。 */
    iconName?: string
    ariaLabel?: string
    variant?: 'normal' | 'primary'
    disabled?: boolean
    loading?: boolean
    fullWidth?: boolean
    emptyText?: string
  }>(),
  {
    text: '',
    iconOnly: false,
    iconName: 'ellipsis',
    ariaLabel: undefined,
    variant: 'normal',
    disabled: false,
    loading: false,
    fullWidth: false,
    emptyText: '暂无选项',
  }
)

const emit = defineEmits<{
  (e: 'select', id: string | number): void
  (e: 'command', id: string | number): void
}>()

const rootRef = ref<HTMLElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)
const open = ref(false)
const highlighted = ref(-1)
const panelStyle = ref<Record<string, string>>({})

const enabledIndexes = computed(() =>
  props.items.map((item, index) => (item.disabled ? -1 : index)).filter((index) => index >= 0)
)

function updatePosition() {
  const trigger = rootRef.value
  const panel = panelRef.value
  if (!trigger || !panel) return
  const rect = trigger.getBoundingClientRect()
  const height = panel.offsetHeight
  const width = Math.max(rect.width, 180)
  const gap = 4
  let top = rect.bottom + gap
  if (top + height > window.innerHeight && rect.top - height - gap > 0) {
    top = rect.top - height - gap
  }
  // 图标按钮通常贴在卡片右缘，面板需要对右对齐并夹在视口内。
  const maxLeft = Math.max(8, window.innerWidth - width - 8)
  const left = props.iconOnly ? Math.min(Math.max(8, rect.right - width), maxLeft) : Math.min(Math.max(8, rect.left), maxLeft)
  panelStyle.value = {
    position: 'fixed',
    left: `${left}px`,
    top: `${top}px`,
    minWidth: `${width}px`,
    maxWidth: 'min(360px, calc(100vw - 16px))',
    zIndex: '9000',
  }
}

function bindPositionListeners() {
  window.addEventListener('scroll', updatePosition, true)
  window.addEventListener('resize', updatePosition)
}

function unbindPositionListeners() {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

function openPanel() {
  if (props.disabled || props.loading || open.value) return
  open.value = true
  highlighted.value = enabledIndexes.value[0] ?? -1
  nextTick(() => {
    updatePosition()
    panelRef.value?.focus({ preventScroll: true })
    bindPositionListeners()
  })
}

function closePanel() {
  if (!open.value) return
  open.value = false
  unbindPositionListeners()
}

function togglePanel() {
  if (open.value) closePanel()
  else openPanel()
}

function choose(item: ButtonDropdownItem) {
  if (item.disabled) return
  emit('select', item.id)
  emit('command', item.id)
  closePanel()
}

function moveHighlight(delta: number) {
  const indexes = enabledIndexes.value
  if (!indexes.length) return
  const current = indexes.indexOf(highlighted.value)
  const next = current === -1 ? 0 : (current + delta + indexes.length) % indexes.length
  highlighted.value = indexes[next]
}

function onTriggerKeydown(event: KeyboardEvent) {
  if (props.disabled || props.loading) return
  if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    openPanel()
  } else if (event.key === 'Escape') {
    closePanel()
  }
}

function onPanelKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      moveHighlight(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      moveHighlight(-1)
      break
    case 'Enter':
    case ' ':
      event.preventDefault()
      if (highlighted.value >= 0) choose(props.items[highlighted.value])
      break
    case 'Escape':
      event.preventDefault()
      closePanel()
      break
    case 'Tab':
      closePanel()
      break
  }
}

function onPointerDown(event: PointerEvent) {
  const target = event.target as Node
  if (rootRef.value?.contains(target) || panelRef.value?.contains(target)) return
  closePanel()
}

watch(open, (value) => {
  if (value) document.addEventListener('pointerdown', onPointerDown, true)
  else document.removeEventListener('pointerdown', onPointerDown, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown, true)
  unbindPositionListeners()
})
</script>

<template>
  <div
    ref="rootRef"
    :class="[CS['button-dropdown']['button-dropdown'], fullWidth && CS['button-dropdown']['full-width']]"
  >
    <Button
      v-if="iconOnly"
      variant="icon"
      :disabled="disabled"
      :loading="loading"
      :icon-name="iconName"
      :aria-label="ariaLabel || text"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    />
    <Button
      v-else
      :variant="variant"
      :disabled="disabled"
      :loading="loading"
      :full-width="fullWidth"
      icon-name="angle-down"
      icon-align="right"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    >
      {{ text }}
    </Button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="panelRef"
        :class="[
          CS.dropdown.dropdown,
          CS.dropdown.refresh,
          CS.dropdown.open,
          CS.dropdown['use-portal'],
          'cs-button-dropdown-panel',
        ]"
        :style="panelStyle"
        role="menu"
        tabindex="-1"
        @keydown="onPanelKeydown"
      >
        <div :class="[CS.dropdown['dropdown-content-wrapper'], CS.dropdown.refresh]">
          <div :class="CS.dropdown['dropdown-content']">
            <ul :class="CS['internal/components/options-list']['options-list']">
              <li
                v-for="(item, index) in items"
                :key="item.id"
                role="none"
                :class="[
                  CS['button-dropdown/item-element']['item-element'],
                  item.disabled && CS['button-dropdown/item-element'].disabled,
                  index === highlighted && CS['button-dropdown/item-element'].highlighted,
                  item.dividerBefore && CS['button-dropdown/item-element']['show-divider'],
                ]"
                @mouseenter="!item.disabled && (highlighted = index)"
              >
                <button
                  type="button"
                  role="menuitem"
                  class="cs-button-dropdown-item"
                  :class="
                    cx(
                      CS['button-dropdown/item-element']['menu-item'],
                      item.disabled && CS['button-dropdown/item-element'].disabled
                    )
                  "
                  :disabled="item.disabled"
                  @click="choose(item)"
                >
                  <Icon
                    v-if="item.iconName"
                    :name="item.iconName"
                    size="x-small"
                    class="cs-button-dropdown-item-icon"
                  />
                  <span class="cs-button-dropdown-item-text">{{ item.text }}</span>
                </button>
              </li>
              <li v-if="!items.length" :class="CS['button-dropdown/item-element']['item-element']">
                <span :class="CS['button-dropdown/item-element']['menu-item']">{{ emptyText }}</span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped lang="scss">
.cs-button-dropdown-panel {
  z-index: 9000;
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-dropdown-container-zmsfa8, #c6c6cd);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
  overflow: hidden;
  outline: none;
  padding-block: 4px;
}

.cs-button-dropdown-item-icon {
  margin-inline-end: 6px;
  flex: none;
}

.cs-button-dropdown-item-text {
  min-inline-size: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cs-button-dropdown-item {
  inline-size: 100%;
  border: 0;
  background: transparent;
  font: inherit;
  text-align: start;
}
</style>
