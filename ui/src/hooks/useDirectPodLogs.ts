import { useCallback, useEffect, useRef, useState } from "react"
import { streamPodLogs } from "../api/kubernetes/pods"

interface UseDirectPodLogsParams {
  podName: string
  namespace: string
  enabled: boolean
}

interface UseDirectPodLogsReturn {
  logs: string[]
  isLoading: boolean
  error: string | null
  reconnect: () => void
}

const MAX_LOG_LINES = 1000

export const useDirectPodLogs = ({
  podName,
  namespace,
  enabled,
}: UseDirectPodLogsParams): UseDirectPodLogsReturn => {
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
    if (!enabled || !podName || !namespace) {
      return
    }

    cleanup()
    setIsLoading(true)
    setError(null)

    try {
      const abortController = new AbortController()
      abortControllerRef.current = abortController

      const response = await streamPodLogs(namespace, podName, {
        follow: true,
        tailLines: "100",
        signal: abortController.signal,
      })

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
  }, [enabled, podName, namespace, cleanup])

  const reconnect = useCallback(() => {
    setLogs([])
    connect()
  }, [connect])

  useEffect(() => {
    if (enabled && podName && namespace) {
      connect()
    } else {
      cleanup()
      setLogs([])
      setIsLoading(false)
      setError(null)
    }

    return cleanup
  }, [enabled, podName, namespace, connect, cleanup])

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
