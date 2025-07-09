import { Paper, styled } from "@mui/material"
import { Outlet, Navigate, useLocation } from "react-router-dom"
import { useEffect } from "react"
import { useNavigate } from "react-router-dom"
import { useMigrationsQuery } from "src/hooks/api/useMigrationsQuery"
import { useNodesQuery } from "src/hooks/api/useNodesQuery"
import Sidenav from "src/components/Sidenav"
import { navigationItems } from "src/config/navigation"

const DashboardContainer = styled("div")({
  display: "flex",
  height: "100vh",
  width: "100%",
  overflow: "hidden"
})

const ContentContainer = styled("div")({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  padding: "16px",
  overflow: "hidden",
  minHeight: 0
})

const StyledPaper = styled(Paper)({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  minHeight: 0,
  "& .MuiDataGrid-root": {
    flex: 1,
    border: "none",
    height: "100%"
  },
  "& .MuiDataGrid-main": {
    overflow: "hidden"
  },
  "& .MuiDataGrid-virtualScroller": {
    overflow: "auto !important"
  }
})

export default function DashboardLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { data: migrations } = useMigrationsQuery()
  const { data: nodes } = useNodesQuery()

  useEffect(() => {
    if (!!migrations && migrations.length === 0 && (!nodes || nodes.length === 0)) {
      navigate("/onboarding")
    }
  }, [migrations, nodes, navigate])

  // Handle redirect from old /dashboard route to default page  
  if (location.pathname === '/dashboard') {
    return <Navigate to="/dashboard/migrations" replace />
  }

  return (
    <DashboardContainer>
      <Sidenav items={navigationItems} />
      <ContentContainer>
        <StyledPaper elevation={0}>
          <Outlet />
        </StyledPaper>
      </ContentContainer>
    </DashboardContainer>
  )
}