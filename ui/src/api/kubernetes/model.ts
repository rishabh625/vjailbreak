export interface Pod {
  metadata: {
    name: string
    namespace: string
  }
}

export interface PodListResponse {
  apiVersion: string
  kind: string
  metadata: {
    continue?: string
    resourceVersion: string
  }
  items: Pod[]
}
