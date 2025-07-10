import { styled, Typography, Box } from "@mui/material"
import { ReactNode } from "react"

const StyledHeader = styled("header")(({ theme }) => ({
  borderBottom: `1px solid ${theme.palette.divider}`,
}))

interface HeaderProps {
  title: string
  icon?: ReactNode
}

export default function Header({ title, icon }: HeaderProps) {
  return (
    <StyledHeader>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, padding: 2 }}>
        {icon}
        <Typography variant="h3">
          {title}
        </Typography>
      </Box>
    </StyledHeader>
  )
}
