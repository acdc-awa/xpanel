<script setup lang="ts">
import { computed } from 'vue'
import Spinner from './Spinner.vue'
import { CS } from './classes'
import { TABLE_BODY_CELL, TABLE_HEADER_CELL } from './table-classes'
import { cx } from './utils'

export interface TableColumn {
  id: string
  /** 表头文案，可用 #header-<id> 插槽覆盖 */
  header?: string
  /** 默认取 item[key]；用 #cell-<id> 插槽或 cell 函数可自定义 */
  key?: string
  width?: number | string
  minWidth?: number | string
  align?: 'start' | 'center' | 'end'
  cell?: (item: any, index: number) => string | number | null | undefined
}

const props = withDefaults(
  defineProps<{
    items: any[]
    columns: TableColumn[]
    /** embedded 适合放进 Container 内容区；container 自带横向留白 */
    variant?: 'container' | 'embedded' | 'borderless' | 'stacked'
    loading?: boolean
    loadingText?: string
    emptyText?: string
    ariaLabel?: string
    trackBy?: string | ((item: any, index: number) => string | number)
    wrapLines?: boolean
    stripedRows?: boolean
    /** 仪表盘等密集场景：套用官方 compact 间距并收紧单元格内边距 */
    compact?: boolean
  }>(),
  {
    variant: 'embedded',
    loading: false,
    loadingText: '正在加载...',
    emptyText: '暂无数据',
    ariaLabel: undefined,
    trackBy: undefined,
    wrapLines: true,
    stripedRows: false,
    compact: true,
  }
)

const rootClasses = computed(() =>
  cx(CS.table.root, 'cs-cloudscape-table', props.compact && 'awsui-context-compact-table')
)

const wrapperClasses = computed(() =>
  cx(CS.table.wrapper, (CS.table as Record<string, string>)[`variant-${props.variant}`])
)

const headerClasses = computed(() =>
  cx(
    TABLE_HEADER_CELL['header-cell'],
    TABLE_HEADER_CELL['is-visual-refresh'],
    TABLE_HEADER_CELL['with-paddings'],
    (TABLE_HEADER_CELL as Record<string, string | undefined>)[`header-cell-variant-${props.variant}`]
  )
)

const bodyCellClasses = computed(() =>
  cx(
    TABLE_BODY_CELL['body-cell'],
    TABLE_BODY_CELL['is-visual-refresh'],
    props.stripedRows && TABLE_BODY_CELL['has-striped-rows']
  )
)

const bodyContentClasses = computed(() =>
  cx(
    TABLE_BODY_CELL['body-cell-content'],
    TABLE_BODY_CELL['with-paddings'],
    props.wrapLines && TABLE_BODY_CELL['body-cell-wrap']
  )
)

function rowKey(item: any, index: number): string | number {
  if (typeof props.trackBy === 'function') return props.trackBy(item, index)
  if (typeof props.trackBy === 'string') return item?.[props.trackBy] ?? index
  return index
}

function valueFor(item: any, column: TableColumn, index = 0) {
  if (column.cell) return column.cell(item, index)
  return column.key ? item?.[column.key] : undefined
}

function renderValue(item: any, index: number, column: TableColumn): string | number {
  const explicit = column.cell?.(item, index)
  if (explicit !== undefined) return explicit ?? ''
  const value = column.key ? item?.[column.key] : undefined
  return value === undefined || value === null ? '' : value
}

function cellStyle(column: TableColumn) {
  return {
    width: typeof column.width === 'number' ? `${column.width}px` : column.width,
    minWidth: typeof column.minWidth === 'number' ? `${column.minWidth}px` : column.minWidth,
  }
}

function contentStyle(column: TableColumn) {
  return column.align && column.align !== 'start' ? { textAlign: column.align } : undefined
}
</script>

<template>
  <div :class="rootClasses">
    <div :class="wrapperClasses">
      <table :class="CS.table.table" :aria-label="ariaLabel">
        <thead :class="CS.table['thead-active']">
          <tr :class="CS.table.row">
            <th
              v-for="(column, colIndex) in columns"
              :key="column.id"
              :class="headerClasses"
              scope="col"
              :data-rightmost="colIndex === columns.length - 1 || undefined"
              :style="cellStyle(column)"
            >
              <div :class="TABLE_HEADER_CELL['header-cell-main']">
                <div :class="TABLE_HEADER_CELL['header-cell-content']">
                  <div :class="TABLE_HEADER_CELL['header-cell-text']">
                    <slot :name="`header-${column.id}`" :column="column">{{ column.header }}</slot>
                  </div>
                </div>
              </div>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(item, rowIndex) in items" :key="rowKey(item, rowIndex)" :class="CS.table.row">
            <td
              v-for="column in columns"
              :key="column.id"
              :class="bodyCellClasses"
              :style="cellStyle(column)"
            >
              <div :class="bodyContentClasses" :style="contentStyle(column)">
                <slot
                  :name="`cell-${column.id}`"
                  :item="item"
                  :row="item"
                  :index="rowIndex"
                  :value="valueFor(item, column, rowIndex)"
                >
                  {{ renderValue(item, rowIndex, column) }}
                </slot>
              </div>
            </td>
          </tr>
          <tr v-if="!items.length" :class="CS.table.row">
            <td :class="[CS.table['cell-merged'], bodyCellClasses]" :colspan="columns.length">
              <div :class="CS.table['cell-merged-content']">
                <span v-if="loading" class="cs-table-loading">
                  <Spinner size="normal" />
                  <span class="cs-table-empty-text">{{ loadingText }}</span>
                </span>
                <span v-else :class="[CS.table.empty, 'cs-table-empty-text']">{{ emptyText }}</span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.cs-table-loading {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.cs-table-empty-text {
  font-size: 13px;
}
</style>
