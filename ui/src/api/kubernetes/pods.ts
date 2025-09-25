import { Pod, PodListResponse } from "./model"
import { KUBERNETES_API_BASE_PATH } from "../constants"
import axios from "../axios"

/**
 * Fetch pods from a namespace with optional label selector
 * @param namespace - The namespace to fetch pods from
 * @param labelSelector - Optional label selector to filter pods
 */
export const fetchPods = async (
  namespace: string,
  labelSelector?: string
): Promise<Pod[]> => {
  const endpoint = `${KUBERNETES_API_BASE_PATH}/namespaces/${namespace}/pods`
  
  const config = labelSelector
    ? {
        params: {
          labelSelector,
        },
      }
    : undefined

  const response = await axios.get<PodListResponse>({
    endpoint,
    config,
  })
  
  return response.items
}

/**
 * Stream logs from a specific pod
 * @param namespace - The namespace of the pod
 * @param podName - The name of the pod to stream logs from
 * @param options - Optional streaming configuration
 */
export const streamPodLogs = async (
  namespace: string,
  podName: string,
  options: {
    follow?: boolean
    tailLines?: string
    signal?: AbortSignal
  } = {}
): Promise<Response> => {
  const { follow = true, tailLines = "100", signal } = options
  
  const endpoint = `${KUBERNETES_API_BASE_PATH}/namespaces/${namespace}/pods/${podName}/log`
  
  const params = new URLSearchParams({
    follow: follow.toString(),
    tailLines,
  })
  
  const url = `${endpoint}?${params.toString()}`

  // For streaming endpoints, we need to use raw fetch instead of axios
  const authToken = import.meta.env.VITE_API_TOKEN
  const headers = {
    "Content-Type": "application/json;charset=UTF-8",
    ...(authToken && { Authorization: `Bearer ${authToken}` }),
  }
  
  const baseUrl = import.meta.env.MODE === "development" ? "/dev-api" : ""
  const fullUrl = `${baseUrl}${url}`

  const response = await fetch(fullUrl, {
    headers,
    signal,
  })

  if (!response.ok) {
    throw new Error(`Failed to stream logs from pod ${podName}: HTTP ${response.status}: ${response.statusText}`)
  }

  return response
}
