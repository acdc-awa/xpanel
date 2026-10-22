import { CS } from './classes'
import { cx } from './utils'

export interface SideNavigationLinkItem {
  type: 'link'
  text: string
  href: string
  icon?: string
  external?: boolean
  info?: string
}

export interface SideNavigationDividerItem {
  type: 'divider'
}

export interface SideNavigationSectionItem {
  type: 'section'
  text: string
  items: SideNavigationItem[]
  defaultExpanded?: boolean
  /** 受控展开态；不传时使用 defaultExpanded。 */
  expanded?: boolean
}

export interface SideNavigationSectionGroupItem {
  type: 'section-group'
  title: string
  items: SideNavigationItem[]
}

export interface SideNavigationLinkGroupItem {
  type: 'link-group'
  text: string
  href: string
  icon?: string
  info?: string
  items: SideNavigationItem[]
}

export interface SideNavigationExpandableLinkGroupItem {
  type: 'expandable-link-group'
  text: string
  href?: string
  icon?: string
  items: SideNavigationItem[]
  defaultExpanded?: boolean
  /** 受控展开态；不传时按 defaultExpanded 或 activeHref 自动展开。 */
  expanded?: boolean
}

export type SideNavigationItem =
  | SideNavigationLinkItem
  | SideNavigationDividerItem
  | SideNavigationSectionItem
  | SideNavigationSectionGroupItem
  | SideNavigationLinkGroupItem
  | SideNavigationExpandableLinkGroupItem

export interface SideNavigationFollowDetail {
  text?: string
  href: string
  type?: 'link' | 'link-group' | 'expandable-link-group' | 'section-header'
  info?: string
}

export interface SideNavigationChangeDetail {
  item: SideNavigationItem
  expanded: boolean
  expandableParents: SideNavigationItem[]
}

export function hasActiveLink(items: readonly SideNavigationItem[], activeHref?: string): boolean {
  if (!activeHref) return false
  return items.some((item) => {
    if (item.type === 'link') return item.href === activeHref
    if (item.type === 'link-group') {
      return item.href === activeHref || hasActiveLink(item.items, activeHref)
    }
    if (item.type === 'expandable-link-group') {
      return item.href === activeHref || hasActiveLink(item.items, activeHref)
    }
    if (item.type === 'section' || item.type === 'section-group') {
      return hasActiveLink(item.items, activeHref)
    }
    return false
  })
}

export function hasNavigationIcons(items: readonly SideNavigationItem[]): boolean {
  return items.some((item) => {
    if (item.type === 'link') return Boolean(item.icon)
    if (item.type === 'link-group' || item.type === 'expandable-link-group') return Boolean(item.icon)
    if (item.type === 'section' || item.type === 'section-group') return hasNavigationIcons(item.items)
    return false
  })
}

export function isPlainLeftClick(event: MouseEvent): boolean {
  return event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey
}

export function sideNavListClasses(
  variant: 'root' | 'section' | 'section-group' | 'link-group' | 'expandable-link-group',
  options: { collapsed?: boolean; withIcons?: boolean; first?: boolean } = {}
): string {
  const cs = CS['side-navigation']
  return cx(
    cs.list,
    cs[`list-variant-${variant}`],
    options.first && cs['list-variant-root--first'],
    options.collapsed && cs[`list-variant-${variant}--collapsed`],
    options.withIcons && cs['list--with-icons']
  )
}

export function sideNavLinkClasses(
  item: { icon?: string; href: string },
  activeHref: string | undefined,
  collapsed: boolean
): string {
  const cs = CS['side-navigation']
  return cx(
    cs.link,
    item.href === activeHref && cs['link-active'],
    collapsed && cs['link--collapsed']
  )
}
