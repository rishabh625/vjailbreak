import { styled, Typography } from '@mui/material'
import cubeIcon from '../assets/platform9-cube.svg'

const LogoContainer = styled('div')(() => ({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: '8px',
}))

const CubeIcon = styled('img')<{ collapsed?: boolean }>(({ collapsed }) => ({
  height: collapsed ? '20px' : '28px',
  width: 'auto',
  transition: 'all 0.3s ease',
  cursor: collapsed ? 'pointer' : 'default',
  transformOrigin: 'center',
  
  ...(collapsed && {
    '&:hover': {
      transform: 'scale(1.1) rotate(5deg)',
      transition: 'transform 0.2s cubic-bezier(0.34, 1.56, 0.64, 1)',
    },
    '&:active': {
      transform: 'scale(0.95) rotate(-2deg)',
      transition: 'transform 0.1s ease-out',
    }
  }),
}))

const BrandText = styled(Typography)(({ theme }) => ({
  fontWeight: 700,
  fontSize: '1.5rem',
  background: theme.palette.primary.main,
  backgroundClip: 'text',
  WebkitBackgroundClip: 'text',
  WebkitTextFillColor: 'transparent',
  transition: 'all 0.3s ease',
}))

interface Platform9LogoProps {
  collapsed?: boolean
}

export default function Platform9Logo({ collapsed = false }: Platform9LogoProps) {
  return (
    <LogoContainer>
      <CubeIcon
        src={cubeIcon}
        alt="Platform9"
        collapsed={collapsed}
      />
      {!collapsed && (
        <BrandText variant="h6">
          vJailbreak
        </BrandText>
      )}
    </LogoContainer>
  )
}