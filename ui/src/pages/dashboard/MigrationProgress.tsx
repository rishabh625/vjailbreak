import { useMemo } from "react"
import { CircularProgress, styled, Typography, Box } from "@mui/material"
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline"
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline"
import HourglassBottomIcon from "@mui/icons-material/HourglassBottom"
import { Phase } from "src/api/migrations/model"
import BlockIcon from "@mui/icons-material/Block";

const ProgressContainer = styled(Box)({
  display: "flex",
  alignItems: "center",
  height: "100%",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "100%",
  overflow: "hidden",
})

interface MigrationProgressProps {
  phase: Phase
  progressText: string
}

export default function MigrationProgress({
  progressText,
  phase
}: MigrationProgressProps) {
  const statusIcon = useMemo(() => {
    if (phase === Phase.Blocked) {
      return <BlockIcon style={{ color: "red" }} />;
    } else if (phase === Phase.Failed) {
      return <ErrorOutlineIcon style={{ color: "red" }} />;
    } else if (phase === Phase.Succeeded) {
      return <CheckCircleOutlineIcon style={{ color: "green" }} />
    } else if ([
      Phase.Validating,
      Phase.AwaitingDataCopyStart,
      Phase.CopyingBlocks,
      Phase.CopyingChangedBlocks,
      Phase.ConvertingDisk,
      Phase.AwaitingCutOverStartTime,
      Phase.AwaitingAdminCutOver
    ].includes(phase as Phase)) {
      return <CircularProgress size={20} style={{ marginRight: 3 }} />
    } else {
      return <HourglassBottomIcon style={{ color: "grey" }} />
    }
  }, [phase])


  return (
    <>
      <ProgressContainer>
        {statusIcon}
        <Typography variant="body2" sx={{ ml: 2 }}>
          {progressText}
        </Typography>
      </ProgressContainer>
    </>
  )
}