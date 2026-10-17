import { Fragment, defineComponent, h, type PropType, type VNode } from 'vue'
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
    return () => {
      const children: VNode[] = flattenVNodes(slots.default?.() ?? [])
      const colspan = Math.max(1, Math.round(12 / props.columns))
      const definition = children.map(() => ({ colspan }))
      const gridClass = cx(
        CS['column-layout'].grid,
        CS['column-layout'][`grid-columns-${props.columns}`],
        CS['column-layout'][`grid-variant-${props.variant}`],
        props.borders === 'vertical' && CS['column-layout']['grid-vertical-borders'],
        props.borders === 'horizontal' && CS['column-layout']['grid-horizontal-borders'],
        (props.disableGutters || props.variant !== 'text-grid') && CS['column-layout']['grid-no-gutters']
      )
      return h('div', { class: CS['column-layout']['column-layout'] }, [
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
