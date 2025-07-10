import Box from "@mui/material/Box"
import Button from "@mui/material/Button"
import { cleanupAllResources } from "src/api/helpers"
import MenuItem from "@mui/material/MenuItem"
import Menu from "@mui/material/Menu"
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown"
import MigrationIcon from "@mui/icons-material/SwapHoriz"
import ClusterIcon from "@mui/icons-material/Hub"
import { useState } from "react"
import ThemeToggle from "./ThemeToggle"

export default function ButtonAppBar({ setOpenMigrationForm, hide = false }) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);


  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleMigrationSelect = (type: string) => {
    setOpenMigrationForm(true, type);
    handleMenuClose();
  };

  return (
    <Box sx={{ visibility: hide ? "hidden" : "visible", display: "flex", gap: 2, alignItems: "center", justifyContent: "flex-end", mr: 10, height: 80 }}>
      <ThemeToggle />
      {
        import.meta.env.MODE === "development" && (
          <Button
            size="large"
            onClick={() => cleanupAllResources()}
            color="error"
            variant="outlined"
          >
            DEV ONLY: Clean Up Resources
          </Button>
        )
      }
      <Button
        size="large"
        onClick={handleMenuClick}
        color="primary"
        variant="contained"
        endIcon={<KeyboardArrowDownIcon />}
      >
        Start Migration
      </Button>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => handleMigrationSelect('standard')}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <MigrationIcon fontSize="small" />
            <span>Start Migration</span>
          </Box>
        </MenuItem>
        <MenuItem onClick={() => handleMigrationSelect('rolling')}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <ClusterIcon fontSize="small" />
            <span>Start Cluster Conversion</span>
          </Box>
        </MenuItem>
      </Menu>
    </Box>
  )
}
