import {
  Checkbox,
  FormControlLabel,
  MenuItem,
  Select,
  styled,
  TextField,
} from "@mui/material"
import { AdapterDayjs } from "@mui/x-date-pickers/AdapterDayjs"
import { DateTimePicker } from "@mui/x-date-pickers/DateTimePicker"
import { LocalizationProvider } from "@mui/x-date-pickers/LocalizationProvider"
import dayjs from "dayjs"
import { useCallback, useEffect } from "react"
import Step from "src/components/forms/Step"
import {
  FieldErrors,
  FormValues,
  SelectedMigrationOptionsType,
} from "./MigrationForm"

// Accordian Imports
import ExpandMoreIcon from "@mui/icons-material/ExpandMore"
import Accordion from "@mui/material/Accordion"
import AccordionDetails from "@mui/material/AccordionDetails"
import AccordionSummary from "@mui/material/AccordionSummary"
import {
  CUTOVER_TYPES,
  DATA_COPY_OPTIONS,
  OS_TYPES,
  OS_TYPES_OPTIONS,
  VM_CUTOVER_OPTIONS,
} from "./constants"

// Styles
const FieldsContainer = styled("div")(({ theme }) => ({
  marginLeft: theme.spacing(4),
}))

const Fields = styled("div")(({ theme }) => ({
  display: "grid",
  gridTemplateColumns: "1fr 2fr 1fr",
  gridGap: "16px", // Adds spacing between the columns
  marginTop: theme.spacing(2),
}))

const CustomTextField = styled(TextField)({
  "& .MuiOutlinedInput-root": {
    fontFamily: "Monospace",
  },
})

// Intefaces
interface MigrationOptionsPropsInterface {
  params: FormValues
  onChange: (key: string) => (value: unknown) => void
  selectedMigrationOptions: SelectedMigrationOptionsType
  updateSelectedMigrationOptions: (
    key: keyof SelectedMigrationOptionsType
  ) => (value: unknown) => void
  errors: FieldErrors
  getErrorsUpdater: (key: string | number) => (value: string) => void
}

// TODO - Commented out the non-required field from the options for now
// const PrePostWebHooksList = [
//   { label: "Pre data-copy web hook", identifier: "preDataCopyWebHook" },
//   { label: "Post data-copy web hook", identifier: "postDataCopyWebHook" },
//   { label: "Pre cutover web hook", identifier: "preCutoverWebHook" },
//   { label: "Post cutover web hook", identifier: "postCutoverWebHook" },
// ]

export default function MigrationOptions({
  params,
  onChange,
  selectedMigrationOptions,
  updateSelectedMigrationOptions,
  errors,
  getErrorsUpdater,
}: MigrationOptionsPropsInterface) {
  // Iniitialize fields
  useEffect(() => {
    onChange("dataCopyMethod")("hot")
    onChange("cutoverOption")(CUTOVER_TYPES.IMMEDIATE)
    onChange("osFamily")(OS_TYPES.AUTO_DETECT)
  }, [])

  const getMinEndTime = useCallback(() => {
    let minDate = params.cutoverStartTime
    if (selectedMigrationOptions.dataCopyStartTime) {
      // Which ever is greater
      minDate =
        dayjs(params.cutoverStartTime).diff(
          dayjs(params.dataCopyStartTime),
          "seconds"
        ) > 0
          ? params.cutoverStartTime
          : params.dataCopyStartTime
    }

    return dayjs(minDate).add(1, "minute")
  }, [params, selectedMigrationOptions])

  return (
    <LocalizationProvider dateAdapter={AdapterDayjs}>
      <Accordion
        sx={{
          boxShadow: "none", // Removes box shadow
          border: "none", // Removes border
          "&:before": {
            display: "none", // Removes the default divider line before the accordion
          },
        }}
        defaultExpanded
      >
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="panel2-content"
          id="panel2-header"
        >
          <Step
            stepNumber="4"
            label="Migration Options (Optional)"
            sx={{ mb: "0" }}
          />
        </AccordionSummary>
        <AccordionDetails>
          <FieldsContainer>
            {/* Retry on failure */}
            <FormControlLabel
              label="Retry On Failure"
              control={
                <Checkbox
                  checked={params?.retryOnFailure || false}
                  onChange={(e) => {
                    onChange("retryOnFailure")(e.target.checked)
                  }}
                />
              }
            />

            {/* Data Copy */}
            <Fields>
              <FormControlLabel
                id="data-copy-method"
                label="Data Copy Method"
                control={
                  <Checkbox
                    checked={selectedMigrationOptions.dataCopyMethod}
                    onChange={(e) => {
                      updateSelectedMigrationOptions("dataCopyMethod")(
                        e.target.checked
                      )
                    }}
                  />
                }
              />
              <Select
                size="small"
                disabled={!selectedMigrationOptions.dataCopyMethod}
                labelId="source-item-label"
                value={params?.dataCopyMethod || "hot"}
                onChange={(e) => {
                  onChange("dataCopyMethod")(e.target.value)
                }}
              >
                {DATA_COPY_OPTIONS.map((item) => (
                  <MenuItem key={item.value} value={item.value}>
                    {item.label}
                  </MenuItem>
                ))}
              </Select>
            </Fields>

            {/* Data Copy Time Window */}
            <Fields>
              <FormControlLabel
                label={"Data Copy Start Time"}
                control={
                  <Checkbox
                    checked={selectedMigrationOptions?.dataCopyStartTime}
                    onChange={(e) => {
                      updateSelectedMigrationOptions("dataCopyStartTime")(
                        e.target.checked
                      )
                    }}
                  />
                }
              />
              <TimePicker
                label="Data Copy Start Time"
                identifier="dataCopyStartTime"
                params={params}
                errors={errors}
                getErrorsUpdater={getErrorsUpdater}
                onChange={onChange}
                disabled={!selectedMigrationOptions.dataCopyStartTime}
                required={!!selectedMigrationOptions.dataCopyStartTime}
              />
            </Fields>

            {/* Cutover settings*/}
            <Fields>
              <FormControlLabel
                id="data-copy-method"
                label="Cutover Options"
                control={
                  <Checkbox
                    checked={selectedMigrationOptions.cutoverOption}
                    onChange={(e) => {
                      updateSelectedMigrationOptions("cutoverOption")(
                        e.target.checked
                      )
                    }}
                  />
                }
              />
              <Select
                size="small"
                disabled={!selectedMigrationOptions?.cutoverOption}
                value={params?.cutoverOption || CUTOVER_TYPES.IMMEDIATE}
                onChange={(e) => {
                  onChange("cutoverOption")(e.target.value)
                }}
              >
                {VM_CUTOVER_OPTIONS.map((item) => (
                  <MenuItem key={item.value} value={item.value}>
                    {item.label}
                  </MenuItem>
                ))}
              </Select>
            </Fields>

            {params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW &&
              selectedMigrationOptions.cutoverOption && (
                <Fields sx={{ mt: "20px", gridTemplateColumns: "1fr 1fr 1fr" }}>
                  <TimePicker
                    label="Cutover Start Time"
                    identifier="cutoverStartTime"
                    params={params}
                    errors={errors}
                    getErrorsUpdater={getErrorsUpdater}
                    onChange={onChange}
                    sx={{ ml: "32px" }}
                    required={
                      params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW
                    }
                  />
                  <TimePicker
                    label="Cutover End Time"
                    identifier="cutoverEndTime"
                    params={params}
                    errors={errors}
                    getErrorsUpdater={getErrorsUpdater}
                    onChange={onChange}
                    required={
                      params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW
                    }
                    minDateTime={getMinEndTime()}
                    helperText="Should be greater than data copy/cutover start time"
                  />
                </Fields>
              )}

            <Fields>
              <FormControlLabel
                label="Post Migration Script"
                control={
                  <Checkbox
                    checked={selectedMigrationOptions.postMigrationScript}
                    onChange={(e) => {
                      updateSelectedMigrationOptions("postMigrationScript")(
                        e.target.checked
                      )
                    }}
                  />
                }
              />
              <CustomTextField
                label="Post Migration Script"
                size="small"
                multiline
                rows={4}
                value={params?.postMigrationScript || ""}
                onChange={(e) =>
                  onChange("postMigrationScript")(String(e.target.value))
                }
                disabled={!selectedMigrationOptions.postMigrationScript}
                error={!!errors["postMigrationScript"]}
                required={selectedMigrationOptions.postMigrationScript}
                placeholder="Enter your post-migration script here..."
              />
            </Fields>

            <Fields>
              <FormControlLabel
                id="os-family"
                label="OS Family"
                control={
                  <Checkbox
                    checked={selectedMigrationOptions.osFamily}
                    onChange={(e) => {
                      updateSelectedMigrationOptions("osFamily")(e.target.checked)
                    }}
                  />
                }
              />
              <Select
                size="small"
                disabled={!selectedMigrationOptions?.osFamily}
                value={params?.osFamily || OS_TYPES.AUTO_DETECT}
                onChange={(e) => {
                  onChange("osFamily")(e.target.value)
                }}
              >
                {OS_TYPES_OPTIONS.map((item) => (
                  <MenuItem key={item.value} value={item.value}>
                    {item.label}
                  </MenuItem>
                ))}
              </Select>
            </Fields>

            {/* Pre and Post Web Hooks */}
            {/* {PrePostWebHooksList.map((hook) => (
              <Fields key={`${hook.label}-${hook.identifier}`}>
                <PrePostWebHooks
                  label={hook.label}
                  identifier={hook.identifier}
                  params={params}
                  selectedMigrationOptions={selectedMigrationOptions}
                  updateSelectedMigrationOptions={updateSelectedMigrationOptions}
                  onChange={onChange}
                />
              </Fields>
            ))} */}
          </FieldsContainer>
        </AccordionDetails>
      </Accordion>
    </LocalizationProvider>
  )
}

const TimePicker = ({
  identifier,
  params,
  onChange,
  errors,
  getErrorsUpdater,
  helperText = "",
  ...restProps
}) => {
  const value = params?.[identifier] ? dayjs(params?.[identifier]) : null

  const handleTimeChange = useCallback(
    (newValue: dayjs.Dayjs | null, identifier) => {
      const formattedTime = newValue?.toISOString()
      onChange(identifier)(String(formattedTime))
    },
    [onChange]
  )

  return (
    <DateTimePicker
      ampm={false}
      value={value}
      onChange={(newValue: dayjs.Dayjs | null) =>
        handleTimeChange(newValue, identifier)
      }
      onError={(error) => {
        getErrorsUpdater(identifier)(error)
      }}
      slots={{
        textField: (props) => (
          <TextField
            {...props}
            size="small"
            required={restProps?.required}
            error={!!errors[identifier] && !restProps?.disabled} // Show error if validation fails
            helperText={
              !!errors[identifier] && !restProps?.disabled ? helperText : ""
            }
          />
        ),
      }}
      {...restProps}
    />
  )
}

// const PrePostWebHooks = ({
//   label,
//   identifier,
//   params,
//   onChange,
//   selectedMigrationOptions,
//   updateSelectedMigrationOptions,
// }) => {
//   return (
//     <>
//       <FormControlLabel
//         label={label}
//         control={
//           <Checkbox
//             checked={selectedMigrationOptions?.[identifier]}
//             onChange={(e) => {
//               updateSelectedMigrationOptions(identifier)(e.target.checked)
//             }}
//           />
//         }
//       />
//       <CustomTextField
//         value={params?.[identifier] || ""}
//         onChange={(e) => onChange(identifier)(String(e.target.value))}
//       />
//     </>
//   )
// }
