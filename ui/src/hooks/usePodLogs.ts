import { useCallback, useEffect, useRef, useState } from "react"

interface UsePodLogsParams {
  jobName: string
  namespace: string
  enabled: boolean
}

interface UsePodLogsReturn {
  logs: string[]
  isLoading: boolean
  error: string | null
  reconnect: () => void
}

const MAX_LOG_LINES = 1000
const KUBERNETES_API_BASE_PATH = "/api/v1"

const getPodNameForJob = async (jobName: string, namespace: string): Promise<string | null> => {
  const baseUrl = import.meta.env.MODE === "development" ? "/dev-api" : ""
  const podListUrl = `${baseUrl}/api/v1/namespaces/${namespace}/pods?labelSelector=job-name=${jobName}`

  const response = await fetch(podListUrl, {
    headers: getFetchHeaders(),
  })

  if (!response.ok) {
    throw new Error(`Failed to fetch pods: ${response.statusText}`)
  }

  const data = await response.json()
  const pod = data.items?.[0]
  return pod?.metadata?.name ?? null
}

export const usePodLogs = ({
  jobName,
  namespace,
  enabled,
}: UsePodLogsParams): UsePodLogsReturn => {
  const [logs, setLogs] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortControllerRef = useRef<AbortController | null>(null)
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  const cleanup = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }
  }, [])

  const connect = useCallback(async () => {
    if (!enabled || !jobName || !namespace) {
      return
    }

    cleanup()
    setIsLoading(true)
    setError(null)

    try {
      const baseUrl = import.meta.env.MODE === "development" ? "/dev-api" : ""
      const resolvedPodName = await getPodNameForJob(jobName, namespace)
      if (!resolvedPodName) {
        throw new Error(`No pods found for job: ${jobName}`)
      }
      const endpoint = `${baseUrl}${KUBERNETES_API_BASE_PATH}/namespaces/${namespace}/pods/${resolvedPodName}/log`
      const params = new URLSearchParams({
        follow: "true",
        tailLines: "100",
      })
      const url = `${endpoint}?${params.toString()}`

      const abortController = new AbortController()
      abortControllerRef.current = abortController

      const response = await fetch(url, {
        headers: getFetchHeaders(),
        signal: abortController.signal,
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      setIsLoading(false)

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('Response body is not readable')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      const readStream = async (): Promise<void> => {
        try {
          const { done, value } = await reader.read()
          
          if (done) {
            // Process any remaining buffer content
            if (buffer.trim()) {
              setLogs(prevLogs => {
                const newLogs = [...prevLogs, buffer.trim()]
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
            setLogs(prevLogs => {
              const newLogs = [...prevLogs, ...lines.filter(line => line.trim())]
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
    } catch (err) {
      setIsLoading(false)
      if (err instanceof Error && err.name === 'AbortError') {
        // Aborted, don't set error
        return
      }
      
      const errorMessage = err instanceof Error ? err.message : "Failed to connect to pod logs stream"
      setError(errorMessage)
      
      // Attempt to reconnect after 3 seconds
      reconnectTimeoutRef.current = setTimeout(() => {
        if (enabled) {
          connect()
        }
      }, 3000)
    }
  }, [enabled, jobName, namespace, cleanup])

  const reconnect = useCallback(() => {
    setLogs([])
    connect()
  }, [connect])

  useEffect(() => {
    if (enabled && jobName && namespace) {
      connect()
    } else {
      cleanup()
      setLogs([])
      setIsLoading(false)
      setError(null)
    }

    return cleanup
  }, [enabled, jobName, namespace, connect, cleanup])

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