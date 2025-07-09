import {
  Drawer,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  IconButton,
  Box,
  Chip,
  Tooltip,
  styled,
  alpha
} from '@mui/material'
import { ChevronLeft, ChevronRight } from '@mui/icons-material'
import { useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { NavigationItem, SidenavProps } from '../../types/navigation'
import { useVersionQuery } from '../../hooks/api/useVersionQuery'


const DRAWER_WIDTH = 280
const DRAWER_WIDTH_COLLAPSED = 72

const ToggleButtonCollapsed = styled(IconButton)(({ theme }) => ({
  position: 'fixed',
  top: '50%',
  left: 'calc(72px - 12px)',
  transform: 'translateY(-50%)',
  backgroundColor: theme.palette.background.paper,
  border: `1px solid ${theme.palette.divider}`,
  borderRadius: '50%',
  width: 32,
  height: 32,
  zIndex: theme.zIndex.drawer + 2,
  boxShadow: theme.shadows[2],
  transition: theme.transitions.create(['left', 'box-shadow'], {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.enteringScreen,
  }),
  '&:hover': {
    backgroundColor: theme.palette.action.hover,
    boxShadow: theme.shadows[4],
  },
}))

const StyledDrawer = styled(Drawer, {
  shouldForwardProp: (prop) => prop !== 'collapsed'
})<{ collapsed: boolean }>(({ theme, collapsed }) => ({
  width: collapsed ? DRAWER_WIDTH_COLLAPSED : DRAWER_WIDTH,
  flexShrink: 0,
  whiteSpace: 'nowrap',
  boxSizing: 'border-box',
  '& .MuiDrawer-paper': {
    width: collapsed ? DRAWER_WIDTH_COLLAPSED : DRAWER_WIDTH,
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
    overflow: 'hidden',
    overflowX: 'hidden',
    overflowY: 'hidden',
    borderRight: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    '&::-webkit-scrollbar': {
      display: 'none',
    },
    scrollbarWidth: 'none',
    msOverflowStyle: 'none',
  },
}))

const DrawerHeader = styled('div')(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: theme.spacing(2, 2),
  ...theme.mixins.toolbar,
  minHeight: '80px !important',
  borderBottom: `1px solid ${alpha(theme.palette.divider, 0.12)}`,
  marginBottom: theme.spacing(1),
}))

const BrandContainer = styled(Box)(() => ({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '100%',
}))

const BrandText = styled(Box)(({ theme }) => ({
  fontWeight: 700,
  fontSize: '1.5rem',
  // color: theme.palette.primary.main,
  // fontFamily: 'system-ui, -apple-system, sans-serif',
  background: theme.palette.primary.main,
  backgroundClip: 'text',
  WebkitBackgroundClip: 'text',
  WebkitTextFillColor: 'transparent',
}))

const VersionBadge = styled(Box)(({ theme }) => ({
  position: 'absolute',
  bottom: theme.spacing(1.5),
  left: '50%',
  transform: 'translateX(-50%)',
  fontSize: '0.8rem',
  color: alpha(theme.palette.text.secondary, 0.6),
  fontWeight: 400,
}))

const VersionDisplay = () => {
  const { data: versionInfo, isLoading, error } = useVersionQuery()

  if (isLoading) {
    return (
      <VersionBadge>
        Loading version...
      </VersionBadge>
    )
  }

  if (error) {
    return (
      <VersionBadge>
        Version: Unable to load
      </VersionBadge>
    )
  }

  return (
    <VersionBadge>
      Version: {versionInfo?.version}
      {versionInfo?.upgradeAvailable && versionInfo?.upgradeVersion && (
        <Box component="span" sx={{ display: 'block', fontSize: '0.7rem', mt: 0.5 }}>
          Update available: {versionInfo.upgradeVersion}
        </Box>
      )}
    </VersionBadge>
  )
}


const StyledListItemButton = styled(ListItemButton, {
  shouldForwardProp: (prop) => prop !== 'active' && prop !== 'collapsed'
})<{ active?: boolean; collapsed?: boolean }>(({ theme, active, collapsed }) => ({
  minHeight: 48,
  margin: collapsed ? theme.spacing(0.5, 1) : theme.spacing(0.5, 1),
  borderRadius: theme.spacing(1),
  paddingLeft: collapsed ? 'auto' : theme.spacing(2),
  paddingRight: collapsed ? 'auto' : theme.spacing(2),
  justifyContent: collapsed ? 'center' : 'initial',
  position: 'relative',
  overflow: 'hidden',
  transition: theme.transitions.create(['background-color', 'transform'], {
    duration: theme.transitions.duration.shorter,
  }),
  ...(active && {
    backgroundColor: alpha(theme.palette.primary.main, 0.08),
    color: theme.palette.primary.main,
    '&::before': {
      content: '""',
      position: 'absolute',
      left: 0,
      top: 0,
      bottom: 0,
      width: 3,
      backgroundColor: theme.palette.primary.main,
    },
    '& .MuiListItemIcon-root': {
      color: theme.palette.primary.main,
    },
  }),
  '&:hover': {
    backgroundColor: alpha(theme.palette.primary.main, 0.04),
    transform: 'translateX(2px)',
  },
}))

const StyledListItemIcon = styled(ListItemIcon, {
  shouldForwardProp: (prop) => prop !== 'collapsed'
})<{ collapsed?: boolean }>(({ collapsed }) => ({
  minWidth: collapsed ? 0 : 56,
  justifyContent: 'center',
}))

const NavigationBadge = styled(Chip)(({ theme }) => ({
  fontSize: '0.6rem',
  height: '16px',
  fontWeight: 600,
  marginLeft: theme.spacing(1),
}))

const ToggleButton = styled(IconButton)(({ theme }) => ({
  position: 'fixed',
  top: '50%',
  left: 'calc(280px - 12px)',
  transform: 'translateY(-50%)',
  backgroundColor: theme.palette.background.paper,
  border: `1px solid ${theme.palette.divider}`,
  borderRadius: '50%',
  width: 32,
  height: 32,
  zIndex: theme.zIndex.drawer + 2,
  boxShadow: theme.shadows[2],
  transition: theme.transitions.create(['left', 'box-shadow'], {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.enteringScreen,
  }),
  '&:hover': {
    backgroundColor: theme.palette.action.hover,
    boxShadow: theme.shadows[4],
  },
}))

interface NavigationItemProps {
  item: NavigationItem
  isActive: boolean
  isCollapsed: boolean
  onClick: (item: NavigationItem) => void
}

function NavigationItemComponent({ item, isActive, isCollapsed, onClick }: NavigationItemProps) {

  const handleClick = () => {
    if (!item.disabled) {
      onClick(item)
    }
  }

  const listItemContent = (
    <StyledListItemButton
      active={isActive}
      collapsed={isCollapsed}
      onClick={handleClick}
      disabled={item.disabled}
    >
      {item.icon && (
        <StyledListItemIcon collapsed={isCollapsed}>
          {item.icon}
        </StyledListItemIcon>
      )}
      {!isCollapsed && (
        <ListItemText
          primary={
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              {item.label}
              {item.badge && (
                <NavigationBadge
                  label={item.badge.label}
                  size="small"
                  color={item.badge.color}
                  variant={item.badge.variant}
                />
              )}
            </Box>
          }
          sx={{
            opacity: isCollapsed ? 0 : 1,
            '& .MuiTypography-root': {
              fontSize: '0.875rem',
            },
          }}
        />
      )}
    </StyledListItemButton>
  )

  if (isCollapsed && (item.label || item.badge)) {
    return (
      <Tooltip
        title={
          <Box>
            {item.label}
            {item.badge && (
              <NavigationBadge
                label={item.badge.label}
                size="small"
                color={item.badge.color}
                variant={item.badge.variant}
                sx={{ ml: 1 }}
              />
            )}
          </Box>
        }
        placement="right"
        arrow
      >
        <ListItem disablePadding sx={{ display: 'block' }}>
          {listItemContent}
        </ListItem>
      </Tooltip>
    )
  }

  return (
    <ListItem disablePadding sx={{ display: 'block' }}>
      {listItemContent}
    </ListItem>
  )
}

export default function Sidenav({
  items,
  isCollapsed: controlledCollapsed,
  onToggleCollapse,
  onItemClick,
  activeItem: controlledActiveItem,
}: SidenavProps) {
  const navigate = useNavigate()
  const location = useLocation()

  const [internalCollapsed, setInternalCollapsed] = useState(() => {
    const saved = localStorage.getItem('sidenav-collapsed')
    return saved ? JSON.parse(saved) : false
  })

  const isCollapsed = controlledCollapsed ?? internalCollapsed
  const activeItem = controlledActiveItem ?? location.pathname

  useEffect(() => {
    if (controlledCollapsed === undefined) {
      localStorage.setItem('sidenav-collapsed', JSON.stringify(internalCollapsed))
    }
  }, [internalCollapsed, controlledCollapsed])


  const handleToggleCollapse = () => {
    if (onToggleCollapse) {
      onToggleCollapse()
    } else {
      setInternalCollapsed((prev: boolean) => !prev)
    }
  }

  const handleItemClick = (item: NavigationItem) => {
    if (onItemClick) {
      onItemClick(item)
    } else {
      navigate(item.path)
    }
  }

  const getActiveItem = (currentPath: string): string => {
    const exactMatch = items.find(item => item.path === currentPath)
    if (exactMatch) return exactMatch.path

    const partialMatch = items.find(item =>
      currentPath.startsWith(item.path) && item.path !== '/dashboard'
    )
    return partialMatch?.path || currentPath
  }

  const currentActiveItem = getActiveItem(activeItem)

  const drawerContent = (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <DrawerHeader>
        <BrandContainer>
          <BrandText>
            {isCollapsed ? 'vJ' : 'vJailbreak'}
          </BrandText>
        </BrandContainer>
      </DrawerHeader>

      <List sx={{
        pt: 1,
        overflow: 'hidden',
        '&::-webkit-scrollbar': {
          display: 'none',
        },
        scrollbarWidth: 'none',
        msOverflowStyle: 'none',
      }}>
        {items
          .filter(item => !item.hidden)
          .map((item) => (
            <NavigationItemComponent
              key={item.id}
              item={item}
              isActive={currentActiveItem === item.path}
              isCollapsed={isCollapsed}
              onClick={handleItemClick}
            />
          ))}
      </List>

      {!isCollapsed && (
        <VersionDisplay />
      )}
    </Box>
  )

  return (
    <Box sx={{ position: 'relative' }}>
      <StyledDrawer
        variant="permanent"
        collapsed={isCollapsed}
      >
        {drawerContent}
      </StyledDrawer>

      {isCollapsed ? (
        <ToggleButtonCollapsed onClick={handleToggleCollapse}>
          <ChevronRight fontSize="small" />
        </ToggleButtonCollapsed>
      ) : (
        <ToggleButton onClick={handleToggleCollapse}>
          <ChevronLeft fontSize="small" />
        </ToggleButton>
      )}
    </Box>
  )
}