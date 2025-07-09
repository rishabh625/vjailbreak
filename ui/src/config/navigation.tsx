import { NavigationItem } from '../types/navigation'
import MigrationIcon from '@mui/icons-material/SwapHoriz'
import AgentsIcon from '@mui/icons-material/Computer'
import CredentialsIcon from '@mui/icons-material/VpnKey'
import ClusterIcon from '@mui/icons-material/Hub'
import ConfigIcon from '@mui/icons-material/Settings'

export const navigationItems: NavigationItem[] = [
  {
    id: 'migrations',
    label: 'Migrations',
    path: '/dashboard/migrations',
    icon: <MigrationIcon />
  },
  {
    id: 'agents',
    label: 'Agents',
    path: '/dashboard/agents',
    icon: <AgentsIcon />
  },
  {
    id: 'credentials',
    label: 'Credentials',
    path: '/dashboard/credentials',
    icon: <CredentialsIcon />
  },
  {
    id: 'cluster-conversions',
    label: 'Cluster Conversions',
    path: '/dashboard/cluster-conversions',
    icon: <ClusterIcon />
  },
  {
    id: 'maas-config',
    label: 'Maas Config',
    path: '/dashboard/maas-config',
    icon: <ConfigIcon />
  }
]

export const getNavigationItemById = (id: string): NavigationItem | undefined => {
  return navigationItems.find(item => item.id === id)
}

export const getNavigationItemByPath = (path: string): NavigationItem | undefined => {
  return navigationItems.find(item => item.path === path)
}