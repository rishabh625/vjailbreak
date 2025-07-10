import { ReactNode } from 'react'

export interface NavigationItem {
  id: string
  label: string
  path: string
  icon?: ReactNode
  badge?: {
    label: string
    color: 'primary' | 'secondary' | 'error' | 'warning' | 'info' | 'success'
    variant: 'filled' | 'outlined'
  }
  disabled?: boolean
  hidden?: boolean
  external?: boolean
  separator?: boolean
  children?: NavigationItem[]
}

export interface NavigationState {
  isCollapsed: boolean
  activeItem: string | null
  expandedItems: string[]
}

export interface SidenavProps {
  items: NavigationItem[]
  isCollapsed?: boolean
  onToggleCollapse?: () => void
  onItemClick?: (item: NavigationItem) => void
  activeItem?: string
}