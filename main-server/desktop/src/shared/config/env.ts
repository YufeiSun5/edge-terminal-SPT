const DEFAULT_MAIN_API_BASE_URL = 'http://127.0.0.1:19080'

export const env = {
  edgeApiBaseUrl:
    import.meta.env.VITE_MAIN_API_BASE_URL ??
    import.meta.env.VITE_EDGE_API_BASE_URL ??
    DEFAULT_MAIN_API_BASE_URL,
}
