import { styled } from "@mui/material"
import { useEffect, useState } from "react"
import { Route, Routes, useLocation, useNavigate } from "react-router-dom"
import "./assets/reset.css"
import AppBar from "./components/AppBar"
import RouteCompatibility from "./components/RouteCompatibility"
import MigrationFormDrawer from "./features/migration/MigrationForm"
import RollingMigrationFormDrawer from "./features/migration/RollingMigrationForm"
import { useMigrationsQuery } from "./hooks/api/useMigrationsQuery"
import DashboardLayout from "./pages/dashboard/DashboardLayout"
import MigrationsPage from "./pages/dashboard/MigrationsPage"
import AgentsPage from "./pages/dashboard/AgentsPage"
import CredentialsPage from "./pages/dashboard/CredentialsPage"
import ClusterConversionsPage from "./pages/dashboard/ClusterConversionsPage"
import MaasConfigPage from "./pages/dashboard/MaasConfigPage"
import Onboarding from "./pages/onboarding/Onboarding"
import { useNodesQuery } from "./hooks/api/useNodesQuery"

const AppFrame = styled("div")(() => ({
  position: "relative",
  display: "grid",
  gridTemplateRows: "auto 1fr",
  height: "100vh",
  overflow: "hidden"
}))

const AppContent = styled("div")(({ theme }) => ({
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  flex: 1,
  [theme.breakpoints.up("lg")]: {
    maxWidth: "1600px",
    margin: "0 auto",
    width: "100%",
  },
}))

function App() {
  const navigate = useNavigate()
  const location = useLocation()
  const [openMigrationForm, setOpenMigrationForm] = useState(false)
  const [migrationType, setMigrationType] = useState('standard')

  const { data: migrations } = useMigrationsQuery()
  const { data: nodes } = useNodesQuery()

  useEffect(() => {
    if (!migrations || !nodes) {
      return
    } else if (migrations.length === 0 && nodes.length === 0) {
      navigate("/onboarding")
    } else if (location.pathname === "/") {
      navigate("/dashboard/migrations")
    }
  }, [migrations, nodes, navigate, location.pathname])

  const hideAppbar =
    location.pathname === "/onboarding" || location.pathname === "/"

  const handleOpenMigrationForm = (open, type = 'standard') => {
    setOpenMigrationForm(open);
    setMigrationType(type);
  };

  return (
    <AppFrame>
      <RouteCompatibility />
      <AppBar setOpenMigrationForm={handleOpenMigrationForm} hide={hideAppbar} />
      <AppContent>
        {openMigrationForm && migrationType === 'standard' && (
          <MigrationFormDrawer
            open
            onClose={() => setOpenMigrationForm(false)}
          />
        )}
        {openMigrationForm && migrationType === 'rolling' && (
          <RollingMigrationFormDrawer
            open
            onClose={() => setOpenMigrationForm(false)}
          />
        )}
        <Routes>
          <Route path="/" element={<div></div>} />
          <Route path="/dashboard" element={<DashboardLayout />}>
            <Route path="migrations" element={<MigrationsPage />} />
            <Route path="agents" element={<AgentsPage />} />
            <Route path="credentials" element={<CredentialsPage />} />
            <Route path="cluster-conversions" element={<ClusterConversionsPage />} />
            <Route path="maas-config" element={<MaasConfigPage />} />
          </Route>
          <Route path="/onboarding" element={<Onboarding />} />
        </Routes>
      </AppContent>
    </AppFrame>
  )
}

export default App
