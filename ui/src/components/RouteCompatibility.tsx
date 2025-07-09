import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

const LEGACY_TAB_MAPPING = {
  'migrations': '/dashboard/migrations',
  'agents': '/dashboard/agents',
  'credentials': '/dashboard/credentials',
  'clusterconversions': '/dashboard/cluster-conversions',
  'clustermigrations': '/dashboard/cluster-conversions', // legacy mapping
  'maasconfig': '/dashboard/maas-config'
}

export default function RouteCompatibility() {
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search)
    const tabParam = searchParams.get('tab')
    
    if (tabParam && location.pathname === '/dashboard') {
      const newRoute = LEGACY_TAB_MAPPING[tabParam as keyof typeof LEGACY_TAB_MAPPING]
      if (newRoute) {
        navigate(newRoute, { replace: true })
      }
    }
  }, [location, navigate])

  return null
}