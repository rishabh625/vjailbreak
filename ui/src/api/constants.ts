// Add this prefix so that the requests are proxied to the backend server
// Proxy config is in vite.config.ts
export const VJAILBREAK_API_BASE_URL =
  import.meta.env.MODE === "development" ? "/dev-api" : ""
export const VJAILBREAK_API_BASE_PATH = "/apis/vjailbreak.k8s.pf9.io/v1alpha1"
export const KUBERNETES_API_BASE_PATH = '/api/v1'
export const VJAILBREAK_DEFAULT_NAMESPACE = "migration-system"
