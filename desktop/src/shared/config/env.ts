const DEFAULT_EDGE_API_BASE_URL = 'http://127.0.0.1:18080'
const DEFAULT_MAIN_API_BASE_URL = 'http://127.0.0.1:19080'

export type RuntimeRole = 'edge' | 'main_server'

export type RuntimeFeatures = {
  sidecar: boolean
  gatewayManage: boolean
  kioManage: boolean
  detectionControl: boolean
  reportGeneration: boolean
  lanWeb: boolean
}

export function runtimeFeaturesFor(role: RuntimeRole): RuntimeFeatures {
  if (role === 'main_server') {
    return {
      sidecar: false,
      gatewayManage: false,
      kioManage: false,
      detectionControl: true,
      reportGeneration: true,
      lanWeb: true,
    }
  }

  return {
    sidecar: true,
    gatewayManage: true,
    kioManage: true,
    detectionControl: true,
    reportGeneration: false,
    lanWeb: false,
  }
}

export function normalizeRuntimeRole(value?: string): RuntimeRole {
  return value === 'main_server' ? 'main_server' : 'edge'
}

const runtimeRole = normalizeRuntimeRole(import.meta.env.VITE_APP_ROLE)
const apiBaseUrl =
  runtimeRole === 'main_server'
    ? import.meta.env.VITE_MAIN_API_BASE_URL ?? import.meta.env.VITE_EDGE_API_BASE_URL ?? DEFAULT_MAIN_API_BASE_URL
    : import.meta.env.VITE_EDGE_API_BASE_URL ?? DEFAULT_EDGE_API_BASE_URL

export const env = {
  runtimeRole,
  runtimeFeatures: runtimeFeaturesFor(runtimeRole),
  apiBaseUrl,
  edgeApiBaseUrl: apiBaseUrl,
}
