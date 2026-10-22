<script setup lang="ts">
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface BreadcrumbItem {
  text: string
  href?: string
}

withDefaults(
  defineProps<{
    items: BreadcrumbItem[]
    ariaLabel?: string
  }>(),
  { ariaLabel: 'Breadcrumb' }
)

const emit = defineEmits<{
  (e: 'follow', detail: { text: string; href?: string }, event: MouseEvent): void
}>()

function onClick(item: BreadcrumbItem, event: MouseEvent) {
  if (!item.href) {
    event.preventDefault()
    return
  }
  emit('follow', { text: item.text, href: item.href }, event)
}
</script>

<template>
  <nav :class="CS['breadcrumb-group']['breadcrumb-group']" :aria-label="ariaLabel">
    <ol :class="CS['breadcrumb-group']['breadcrumb-group-list']">
      <li v-for="(item, index) in items" :key="`${index}-${item.text}`" :class="CS['breadcrumb-group'].item">
        <div :class="cx(CS['breadcrumb-group/item'].breadcrumb, index === items.length - 1 && CS['breadcrumb-group/item'].last)">
          <span
            v-if="index === items.length - 1"
            :class="cx(CS['breadcrumb-group/item'].anchor, CS['breadcrumb-group/item'].text)"
            aria-current="page"
          >
            {{ item.text }}
          </span>
          <span
            v-else-if="!item.href"
            :class="cx(CS['breadcrumb-group/item'].anchor, CS['breadcrumb-group/item'].text)"
          >
            {{ item.text }}
          </span>
          <a
            v-else
            :class="cx(CS['breadcrumb-group/item'].anchor, CS['breadcrumb-group/item'].text)"
            :href="item.href"
            @click="onClick(item, $event)"
          >
            {{ item.text }}
          </a>
          <span v-if="index !== items.length - 1" :class="CS['breadcrumb-group/item'].icon">
            <Icon name="angle-right" />
          </span>
        </div>
      </li>
    </ol>
  </nav>
</template>
