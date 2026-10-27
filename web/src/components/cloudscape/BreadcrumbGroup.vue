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
      <li
        v-for="(item, index) in items"
        :key="`${index}-${item.text}`"
        :class="[CS['breadcrumb-group'].item, 'cs-crumb-item']"
      >
        <div :class="cx(CS['breadcrumb-group/item'].breadcrumb, index === items.length - 1 && CS['breadcrumb-group/item'].last)">
          <!-- `text` 必须是 anchor 的子元素：vendor 的省略号规则是 `.anchor > .text`
               （nowrap + ellipsis），把两个类挂在同一元素上会退化成可换行的普通文本 -->
          <span
            v-if="index === items.length - 1"
            :class="CS['breadcrumb-group/item'].anchor"
            aria-current="page"
          >
            <span :class="CS['breadcrumb-group/item'].text">{{ item.text }}</span>
          </span>
          <span v-else-if="!item.href" :class="CS['breadcrumb-group/item'].anchor">
            <span :class="CS['breadcrumb-group/item'].text">{{ item.text }}</span>
          </span>
          <a
            v-else
            :class="CS['breadcrumb-group/item'].anchor"
            :href="item.href"
            @click="onClick(item, $event)"
          >
            <span :class="CS['breadcrumb-group/item'].text">{{ item.text }}</span>
          </a>
          <span v-if="index !== items.length - 1" :class="CS['breadcrumb-group/item'].icon">
            <Icon name="angle-right" />
          </span>
        </div>
      </li>
    </ol>
  </nav>
</template>

<style scoped>
/* 每项都能收缩到 0（否则 min-content 会顶住，内层省略号没机会生效） */
.cs-crumb-item {
  min-inline-size: 0;
}

/* 当前位置（最后一项）优先保证可读：窄屏先省略前面的层级，vendor 默认给它 flex-shrink:1 */
.cs-crumb-item:last-child {
  flex-shrink: 0 !important;
}
</style>
