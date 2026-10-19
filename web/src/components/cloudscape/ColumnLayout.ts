import { Fragment, defineComponent, h, onBeforeUnmount, onMounted, ref, type PropType, type VNode } from 'vue'
import Grid from './Grid'
import { CS } from './classes'
import { cx } from './utils'

function flattenVNodes(nodes: VNode[]): VNode[] {
  const out: VNode[] = []
  for (const node of nodes) {
    if (node.type === Fragment) {
      out.push(...flattenVNodes((node.children as VNode[]) ?? []))
    } else {
      out.push(node)
    }
  }
  return out
}

/**
 * 官方 COLUMN_DEFS：各断点下单列的栅格宽度。列数不是恒定的——容器窄于 465px
 * 时全宽单列，`xxs`（≥465）列数减半，`xs`（≥688）才是完整的 N 列。
 */
const COLUMN_DEFS: Record<number, Record<'default' | 'xxs' | 'xs', number>> = {
  1: { default: 12, xxs: 12, xs: 12 },
  2: { default: 12, xxs: 6, xs: 6 },
  3: { default: 12, xxs: 6, xs: 4 },
  4: { default: 12, xxs: 6, xs: 3 },
}

/** 官方 useContainerBreakpoints 的触发阈值；本组件只订阅 default/xxs/xs 三档。 */
const TRIGGERS: Array<{ name: 'xs' | 'xxs'; minWidth: number }> = [
  { name: 'xs', minWidth: 688 },
  { name: 'xxs', minWidth: 465 },
]

type Breakpoint = 'default' | 'xxs' | 'xs'

function matchingBreakpoint(width: number): Breakpoint {
  for (const trigger of TRIGGERS) {
    if (width >= trigger.minWidth) return trigger.name
  }
  return 'default'
}

export default defineComponent({
  name: 'ColumnLayout',
  props: {
    columns: { type: Number as PropType<1 | 2 | 3 | 4>, default: 1 },
    variant: { type: String as PropType<'default' | 'text-grid'>, default: 'default' },
    borders: { type: String as PropType<'none' | 'vertical' | 'horizontal' | 'all'>, default: 'none' },
    disableGutters: { type: Boolean, default: false },
    tag: { type: String, default: 'div' },
  },
  setup(props, { slots }) {
    const rootRef = ref<HTMLElement | null>(null)
    const breakpoint = ref<Breakpoint>('default')
    let observer: ResizeObserver | null = null

    function measure(width: number) {
      const next = matchingBreakpoint(width)
      if (next !== breakpoint.value) breakpoint.value = next
    }

    onMounted(() => {
      const el = rootRef.value
      if (!el) return
      // 官方按容器宽度判定而非视口：侧栏折叠、弹窗内嵌都会改变实际可用宽度。
      measure(el.getBoundingClientRect().width)
      if (typeof ResizeObserver === 'undefined') return
      observer = new ResizeObserver((entries) => {
        const width = entries[0]?.contentRect.width
        if (typeof width === 'number') measure(width)
      })
      observer.observe(el)
    })

    onBeforeUnmount(() => {
      observer?.disconnect()
      observer = null
    })

    return () => {
      const children: VNode[] = flattenVNodes(slots.default?.() ?? [])
      const def = COLUMN_DEFS[props.columns] ?? COLUMN_DEFS[1]
      const definition = children.map(() => ({ colspan: def[breakpoint.value] }))
      const gridClass = cx(
        CS['column-layout'].grid,
        CS['column-layout'][`grid-columns-${props.columns}`],
        CS['column-layout'][`grid-variant-${props.variant}`],
        // 官方 CSS 靠断点类来清零每行首列的左边框；没有这个类，首列左侧会多出一条竖线。
        CS['column-layout'][`grid-breakpoint-${breakpoint.value}`],
        props.borders === 'vertical' && CS['column-layout']['grid-vertical-borders'],
        props.borders === 'horizontal' && CS['column-layout']['grid-horizontal-borders'],
        (props.disableGutters || props.variant !== 'text-grid') && CS['column-layout']['grid-no-gutters']
      )
      return h('div', { ref: rootRef, class: CS['column-layout']['column-layout'] }, [
        h(
          Grid,
          {
            tag: props.tag,
            class: gridClass,
            disableGutters: true,
            gridDefinition: definition,
          },
          slots
        ),
      ])
    }
  },
})
