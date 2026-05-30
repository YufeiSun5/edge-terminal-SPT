const DEFAULT_EDGE_API_BASE_URL = 'http://127.0.0.1:18080'

export const env = {
  edgeApiBaseUrl: import.meta.env.VITE_EDGE_API_BASE_URL ?? DEFAULT_EDGE_API_BASE_URL,
}
