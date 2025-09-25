import axios from "../axios"

import {
  VJAILBREAK_API_BASE_PATH,
  VJAILBREAK_DEFAULT_NAMESPACE,
} from "../constants"
import { RdmDisk, RdmDiskList } from "./model"

export const getRdmDisksList = async (
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/rdmdisks`
  const response = await axios.get<RdmDiskList>({
    endpoint,
  })
  return response?.items || []
}

export const getRdmDisk = async (
  name: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/rdmdisks/${name}`
  const response = await axios.get<RdmDisk>({
    endpoint,
  })
  return response
}

export const patchRdmDisk = async (
  name: string,
  data: Partial<RdmDisk>,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/rdmdisks/${name}`
  const response = await axios.patch<RdmDisk>({
    endpoint,
    data,
    config: {
      headers: {
        "Content-Type": "application/merge-patch+json",
      },
    },
  })
  return response
}