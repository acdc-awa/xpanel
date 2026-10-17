export interface WidgetDefinition {
  id: string
  title: string
  description: string
  category?: string
  defaultW: number
  defaultH: number
  minW?: number
  minH?: number
  maxW?: number
  maxH?: number
}

export interface WidgetLayoutItem {
  id: string
  x: number
  y: number
  w: number
  h: number
  minW?: number
  minH?: number
  maxW?: number
  maxH?: number
}
