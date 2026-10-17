<script setup lang="ts">
import ColumnLayout from './ColumnLayout'
import { CS } from './classes'

export interface KeyValuePair {
  label: string
  value?: string | number
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
        <dd :class="CS['key-value-pairs'].detail">{{ item.value }}</dd>
      </div>
    </ColumnLayout>
  </div>
</template>
