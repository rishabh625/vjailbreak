import { useCallback, useEffect, useRef, useState } from "react"

interface UseDeploymentLogsParams {
  deploymentName: string
  namespace: string
  labelSelector: string
  enabled: boolean
}

interface UseDeploymentLogsReturn {
  logs: string[]
  isLoading: boolean
  error: string | null
  reconnect: () => void
}

interface Pod {
  metadata: {
    name: string
    namespace: string
  }
}

interface PodListResponse {
  items: Pod[]
}

const MAX_LOG_LINES = 1000
const KUBERNETES_API_BASE_PATH = "/api/v1"

export const useDeploymentLogs = ({
  deploymentName,
  namespace,
  labelSelector,
  enabled,
}: UseDeploymentLogsParams): UseDeploymentLogsReturn => {
  const [logs, setLogs] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortControllersRef = useRef<AbortController[]>([])
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  const cleanup = useCallback(() => {
    // Abort all active connections
    abortControllersRef.current.forEach(controller => {
      controller.abort()
    })
    abortControllersRef.current = []
    
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }
  }, [])

  const fetchPods = useCallback(async (): Promise<Pod[]> => {
    const baseUrl = import.meta.env.MODE === "development" ? "/dev-api" : ""
    const endpoint = `${baseUrl}${KUBERNETES_API_BASE_PATH}/namespaces/${namespace}/pods`
    const params = new URLSearchParams({
      labelSelector: labelSelector,
    })
    const url = `${endpoint}?${params.toString()}`

    const response = await fetch(url, {
      headers: getFetchHeaders(),
      signal: new AbortController().signal,})
      
    if (!response.ok) {
      throw new Error(`Failed to fetch pods: HTTP ${response.status}: ${response.statusText}`)
    }

    const data: PodListResponse = await response.json()
    return data.items
  }, [namespace, labelSelector])

  const streamPodLogs = useCallback(async (podName: string, podNamespace: string): Promise<void> => {
    const baseUrl = import.meta.env.MODE === "development" ? "/dev-api" : ""
    const endpoint = `${baseUrl}${KUBERNETES_API_BASE_PATH}/namespaces/${podNamespace}/pods/${podName}/log`
    const params = new URLSearchParams({
      follow: "true",
      tailLines: "100",
    })
    const url = `${endpoint}?${params.toString()}`

    const abortController = new AbortController()
    abortControllersRef.current.push(abortController)

    const response = await fetch(url, {
      headers: getFetchHeaders(),
      signal: abortController.signal,
    })

    if (!response.ok) {
      throw new Error(`Failed to stream logs from pod ${podName}: HTTP ${response.status}: ${response.statusText}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error(`Response body is not readable for pod ${podName}`)
    }

    const decoder = new TextDecoder()
    let buffer = ''

    const readStream = async (): Promise<void> => {
      try {
        const { done, value } = await reader.read()
        
        if (done) {
          // Process any remaining buffer content
          if (buffer.trim()) {
            const logLine = `[${podName}] ${buffer.trim()}`
            setLogs(prevLogs => {
              const newLogs = [...prevLogs, logLine]
              return newLogs.length > MAX_LOG_LINES 
                ? newLogs.slice(-MAX_LOG_LINES) 
                : newLogs
            })
          }
          return
        }

        // Decode the chunk and add to buffer
        buffer += decoder.decode(value, { stream: true })
        
        // Split by newlines and process complete lines
        const lines = buffer.split('\n')
        buffer = lines.pop() || '' // Keep incomplete line in buffer
        
        if (lines.length > 0) {
          const prefixedLines = lines
            .filter(line => line.trim())
            .map(line => `[${podName}] ${line}`)
          
          setLogs(prevLogs => {
            const newLogs = [...prevLogs, ...prefixedLines]
            return newLogs.length > MAX_LOG_LINES 
              ? newLogs.slice(-MAX_LOG_LINES) 
              : newLogs
          })
        }

        // Continue reading
        return readStream()
      } catch (err) {
        if (err instanceof Error && err.name === 'AbortError') {
          // Stream was aborted, this is expected
          return
        }
        throw err
      }
    }

    await readStream()
  }, [])

  const connect = useCallback(async () => {
    if (!enabled || !deploymentName || !namespace || !labelSelector) {
      return
    }

    cleanup()
    setIsLoading(true)
    setError(null)

    try {
      // First, fetch the pods for this deployment
      const pods = await fetchPods()
      
      if (pods.length === 0) {
        throw new Error(`No pods found for deployment ${deploymentName} with label selector ${labelSelector}`)
      }

      setIsLoading(false)

      // Start streaming logs from all pods
      const streamPromises = pods.map(pod => 
        streamPodLogs(pod.metadata.name, pod.metadata.namespace)
      )

      // Wait for all streams to complete (they won't unless there's an error or abort)
      await Promise.all(streamPromises)
    } catch (err) {
      setIsLoading(false)
      if (err instanceof Error && err.name === 'AbortError') {
        // Aborted, don't set error
        return
      }
      
      const errorMessage = err instanceof Error ? err.message : `Failed to connect to deployment ${deploymentName} logs stream`
      setError(errorMessage)
      
      // Attempt to reconnect after 3 seconds
      reconnectTimeoutRef.current = setTimeout(() => {
        if (enabled) {
          connect()
        }
      }, 3000)
    }
  }, [enabled, deploymentName, namespace, labelSelector, cleanup, fetchPods, streamPodLogs])

  const reconnect = useCallback(() => {
    setLogs([])
    connect()
  }, [connect])

  useEffect(() => {
    if (enabled && deploymentName && namespace && labelSelector) {
      connect()
    } else {
      cleanup()
      setLogs([])
      setIsLoading(false)
      setError(null)
    }

    return cleanup
  }, [enabled, deploymentName, namespace, labelSelector, connect, cleanup])

  useEffect(() => {
    return cleanup
  }, [cleanup])

  return {
    logs,
    isLoading,
    error,
    reconnect,
  }
}
const getFetchHeaders = () => {
  const authToken = import.meta.env.VITE_API_TOKEN
  return {
    "Content-Type": "application/json;charset=UTF-8",
    ...(authToken && { Authorization: `Bearer ${authToken}` }),
  }
}