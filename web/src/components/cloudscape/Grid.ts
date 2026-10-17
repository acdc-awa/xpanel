import { Fragment, defineComponent, h, type PropType, type VNode } from 'vue'
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

export interface GridDefinition {
  colspan?: number
  offset?: number
  pull?: number
  push?: number
}

export default defineComponent({
  name: 'Grid',
  props: {
    gridDefinition: {
      type: Array as PropType<Array<GridDefinition | number>>,
      default: () => [],
    },
    disableGutters: { type: Boolean, default: false },
    tag: { type: String, default: 'div' },
  },
  setup(props, { slots }) {
    return () => {
      const children: VNode[] = flattenVNodes(slots.default?.() ?? [])
      return h(
        props.tag,
        { class: cx(CS.grid.grid, props.disableGutters && CS.grid['no-gutters']) },
        children.map((child, index) => {
          const raw = props.gridDefinition[index]
          const def: GridDefinition = typeof raw === 'number' ? { colspan: raw } : raw ?? {}
          const classes = cx(
            CS.grid['grid-column'],
            def.colspan !== undefined ? CS.grid[`colspan-${def.colspan}`] : undefined,
            def.offset !== undefined ? CS.grid[`offset-${def.offset}`] : undefined,
            def.pull !== undefined ? CS.grid[`pull-${def.pull}`] : undefined,
            def.push !== undefined ? CS.grid[`push-${def.push}`] : undefined
          )
          return h('div', { key: child.key ?? index, class: classes }, [
            h('div', { class: CS.grid['restore-pointer-events'] }, [child]),
          ])
        })
      )
    }
  },
})
