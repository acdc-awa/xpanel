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

type Size = 'xxxs' | 'xxs' | 'xs' | 's' | 'm' | 'l' | 'xl' | 'xxl'

/**
 * Official Cloudscape SpaceBetween: every direct child is wrapped in its own
 * `.child` element, which is where the spacing rules live. A plain template
 * cannot wrap an arbitrary slot per child, so this component uses a render
 * function.
 */
export default defineComponent({
  name: 'SpaceBetween',
  props: {
    direction: {
      type: String as PropType<'vertical' | 'horizontal'>,
      default: 'vertical',
    },
    size: {
      type: String as PropType<Size>,
      default: 'm',
    },
    alignItems: {
      type: String as PropType<'center' | 'start' | 'end'>,
      default: undefined,
    },
  },
  setup(props, { slots }) {
    return () => {
      const children: VNode[] = flattenVNodes(slots.default?.() ?? [])
      const propagateAlignment = props.direction === 'horizontal' && Boolean(props.alignItems)
      return h(
        'div',
        {
          class: cx(
            CS['space-between'].root,
            CS['space-between'][props.direction],
            CS['space-between'][`${props.direction}-${props.size}`],
            props.alignItems && CS['space-between'][`align-${props.alignItems}`]
          ),
        },
        children.map((child, index) =>
          h(
            'div',
            {
              key: child.key ?? index,
              class: cx(CS['space-between'].child, propagateAlignment && CS['space-between']['child-aligned']),
            },
            [child]
          )
        )
      )
    }
  },
})
