<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import Select from './Select.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    currentPage?: number
    pageSize?: number
    total?: number
    pageSizes?: number[]
    layout?: string
    disabled?: boolean
    small?: boolean
    background?: boolean
  }>(),
  {
    currentPage: 1,
    pageSize: 20,
    total: 0,
    pageSizes: () => [10, 20, 50],
    layout: 'prev, pager, next',
    disabled: false,
    small: false,
    background: false,
  }
)

const emit = defineEmits<{
  (e: 'update:currentPage', value: number): void
  (e: 'update:pageSize', value: number): void
  (e: 'current-change', value: number): void
  (e: 'size-change', value: number): void
  (e: 'change', page: number, size: number): void
}>()

const pagesCount = computed(() => Math.max(1, Math.ceil(props.total / Math.max(1, props.pageSize))))
const current = computed(() => Math.min(Math.max(1, props.currentPage), pagesCount.value))
const layoutParts = computed(() => props.layout.split(',').map((part) => part.trim()))

type PageEntry = number | 'left-dots' | 'right-dots'
const pages = computed<PageEntry[]>(() => {
  const count = pagesCount.value
  const page = current.value
  if (count <= 7) return Array.from({ length: count }, (_, index) => index + 1)
  const result: PageEntry[] = [1]
  const start = Math.max(2, Math.min(page - 2, count - 4))
  const end = Math.min(count - 1, Math.max(page + 2, 5))
  if (start > 2) result.push('left-dots')
  for (let index = start; index <= end; index += 1) result.push(index)
  if (end < count - 1) result.push('right-dots')
  result.push(count)
  return result
})

const sizeOptions = computed(() =>
  props.pageSizes.map((size) => ({ label: `${size} 条/页`, value: size }))
)

function goTo(page: number) {
  if (props.disabled) return
  const next = Math.min(Math.max(1, page), pagesCount.value)
  if (next === current.value) return
  emit('update:currentPage', next)
  emit('current-change', next)
  emit('change', next, props.pageSize)
}

function onSizeChange(value: string | number | (string | number)[] | '') {
  if (props.disabled) return
  const size = Number(Array.isArray(value) ? value[0] : value)
  if (!Number.isFinite(size) || size <= 0) return
  emit('update:pageSize', size)
  emit('size-change', size)
  emit('change', current.value, size)
}
</script>

<template>
  <div class="cs-pagination-wrapper">
    <span v-if="layoutParts.includes('total')" class="cs-pagination-total">共 {{ total }} 条</span>
    <div v-if="layoutParts.includes('sizes')" class="cs-pagination-sizes">
      <Select :model-value="pageSize" :options="sizeOptions" @update:model-value="onSizeChange" />
    </div>
    <ul
      :class="cx(CS.pagination.root, disabled && CS.pagination['root-disabled'])"
      aria-label="分页"
    >
      <li :class="CS.pagination['page-item']">
        <button
          type="button"
          :class="
            cx(
              CS.pagination.button,
              CS.pagination.arrow,
              current <= 1 && CS.pagination['button-disabled']
            )
          "
          aria-label="上一页"
          :aria-disabled="current <= 1 || undefined"
          :disabled="current <= 1"
          @click="goTo(current - 1)"
        >
          <Icon name="angle-left" />
        </button>
      </li>
      <template v-for="entry in pages" :key="entry">
        <li v-if="typeof entry !== 'number'" :class="CS.pagination.dots">...</li>
        <li v-else :class="CS.pagination['page-item']">
          <button
            type="button"
            :class="
              cx(
                CS.pagination.button,
                CS.pagination['page-number'],
                entry === current && CS.pagination['button-current']
              )
            "
            :aria-current="entry === current"
            :aria-label="`第 ${entry} 页`"
            @click="goTo(entry)"
          >
            {{ entry }}
          </button>
        </li>
      </template>
      <li :class="CS.pagination['page-item']">
        <button
          type="button"
          :class="
            cx(
              CS.pagination.button,
              CS.pagination.arrow,
              current >= pagesCount && CS.pagination['button-disabled']
            )
          "
          aria-label="下一页"
          :aria-disabled="current >= pagesCount || undefined"
          :disabled="current >= pagesCount"
          @click="goTo(current + 1)"
        >
          <Icon name="angle-right" />
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.cs-pagination-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.cs-pagination-total {
  font-size: 13px;
  color: var(--color-text-body-secondary, #5f6b7a);
}

.cs-pagination-sizes {
  inline-size: 120px;
}
</style>
