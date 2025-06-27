// Constants

export enum CUTOVER_TYPES {
  "IMMEDIATE" = "0",
  "ADMIN_INITIATED" = "1",
  "TIME_WINDOW" = "2",
}

export enum OS_TYPES {
  "AUTO_DETECT" = "default",
  "WINDOWS" = "windowsGuest",
  "LINUX" = "linuxGuest",
}

export const DATA_COPY_OPTIONS = [
  { value: "hot", label: "Copy live VMs, then power off" },
  { value: "cold", label: "Power off live VMs, then copy" },
]

export const OS_TYPES_OPTIONS = [
  { value: OS_TYPES.AUTO_DETECT, label: "Auto-detect" },
  { value: OS_TYPES.WINDOWS, label: "Windows" },
  { value: OS_TYPES.LINUX, label: "Linux" },
]

export const VM_CUTOVER_OPTIONS = [
  {
    value: CUTOVER_TYPES.IMMEDIATE,
    label: "Cutover immediately after data copy",
  },
  { value: CUTOVER_TYPES.ADMIN_INITIATED, label: "Admin initiated cutover" },
  { value: CUTOVER_TYPES.TIME_WINDOW, label: "Cutover during time window" },
]
