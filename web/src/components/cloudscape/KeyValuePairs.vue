<script setup lang="ts">
import ColumnLayout from './ColumnLayout'
import { CS } from './classes'

export interface KeyValuePair {
  label: string
  value?: string | number
  /** 供 #value 插槽按行分派富内容时使用。 */
  key?: string
}

withDefaults(
  defineProps<{
    items: KeyValuePair[]
    columns?: 1 | 2 | 3 | 4
    ariaLabel?: string
  }>(),
  { columns: 1, ariaLabel: undefined }
)
</script>

<template>
  <div :class="CS['key-value-pairs']['key-value-pairs']" :aria-label="ariaLabel">
    <ColumnLayout :columns="columns" variant="text-grid" tag="dl">
      <div v-for="(item, index) in items" :key="index">
        <dt :class="CS['key-value-pairs']['key-label']">{{ item.label }}</dt>
        <dd :class="CS['key-value-pairs'].detail">
          <!-- 官方 detail 支持 ReactNode；这里用 value 插槽承载徽标/等宽代码等富内容，
               默认插槽内容仍回退到字符串 value。 -->
          <slot name="value" :item="item" :index="index">{{ item.value }}</slot>
        </dd>
      </div>
    </ColumnLayout>
  </div>
</template>
