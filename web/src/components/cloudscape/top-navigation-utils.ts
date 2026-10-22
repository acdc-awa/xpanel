export interface TopNavigationIdentity {
  title?: string
  href: string
  logo?: { src: string; alt?: string }
}

export interface TopNavigationUtilityBase {
  text?: string
  title?: string
  iconName?: string
  ariaLabel?: string
  /** 仅在窄档隐藏该工具；文字收起由 disableTextCollapse 控制。 */
  collapseAt?: 'medium' | 'narrow'
  disableTextCollapse?: boolean
  disableUtilityCollapse?: boolean
}

export interface TopNavigationButtonUtility extends TopNavigationUtilityBase {
  type: 'button'
  variant?: 'link' | 'primary-button'
  onClick?: () => void
}

export type TopNavigationUtility = TopNavigationButtonUtility
