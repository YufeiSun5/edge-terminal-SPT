import { env } from '@/shared/config/env'

export type SidecarState =
  | 'unavailable'
  | 'missing'
  | 'stopped'
  | 'starting'
  | 'online'
  | 'offline'
  | 'unhealthy'
  | 'failed'

export type SidecarStatus = {
  state: SidecarState
  pid: number | null
  error: string | null
  health: unknown
  backendUrl: string
  logFile: string | null
  restartAttempts?: number
  watchdogEnabled?: boolean
}

export type BackendRuntimeStatus = SidecarStatus

export type AppInfo = {
  isPackaged: boolean
  version: string
  userDataPath: string
  backendUrl: string
}

export type DesktopStatus = {
  autostart: {
    openAtLogin: boolean
    openAsHidden?: boolean
  }
  minimizeToTray: boolean
  trayAvailable: boolean
  watchdogEnabled: boolean
  restartAttempts: number
}

export type RuntimeLogTail = {
  logFile: string
  size: number
  updatedAt: string | null
  content: string
}

export type EdgeDesktopApi = {
  getAppInfo: () => Promise<AppInfo>
  getDesktopStatus: () => Promise<DesktopStatus>
  setAutostart: (enabled: boolean) => Promise<DesktopStatus>
  setMinimizeToTray: (enabled: boolean) => Promise<DesktopStatus>
  getSidecarStatus: () => Promise<SidecarStatus>
  restartSidecar: () => Promise<SidecarStatus>
  openLogs: () => Promise<{ logFile: string }>
  readLogs: (options?: { maxBytes?: number }) => Promise<RuntimeLogTail>
  openExternal: (url: string) => Promise<{ opened: boolean }>
}

export type DesktopBridgeApi = EdgeDesktopApi

export async function getAppInfo(): Promise<AppInfo> {
  if (!window.edgeDesktop) {
    return {
      isPackaged: false,
      version: 'renderer-only',
      userDataPath: '',
      backendUrl: env.apiBaseUrl,
    }
  }
  return window.edgeDesktop.getAppInfo()
}

export async function getDesktopStatus(): Promise<DesktopStatus> {
  if (!window.edgeDesktop) {
    return {
      autostart: { openAtLogin: false, openAsHidden: false },
      minimizeToTray: false,
      trayAvailable: false,
      watchdogEnabled: false,
      restartAttempts: 0,
    }
  }
  return window.edgeDesktop.getDesktopStatus()
}

export async function setAutostart(enabled: boolean): Promise<DesktopStatus> {
  if (!window.edgeDesktop) {
    return getDesktopStatus()
  }
  return window.edgeDesktop.setAutostart(enabled)
}

export async function setMinimizeToTray(enabled: boolean): Promise<DesktopStatus> {
  if (!window.edgeDesktop) {
    return getDesktopStatus()
  }
  return window.edgeDesktop.setMinimizeToTray(enabled)
}

export async function getSidecarStatus(): Promise<SidecarStatus> {
  if (!window.edgeDesktop) {
    return {
      state: 'unavailable',
      pid: null,
      error: 'Electron preload bridge is unavailable',
      health: null,
      backendUrl: env.apiBaseUrl,
      logFile: null,
    }
  }
  return window.edgeDesktop.getSidecarStatus()
}

export async function getBackendRuntimeStatus(): Promise<BackendRuntimeStatus> {
  return getSidecarStatus()
}

export async function restartSidecar(): Promise<SidecarStatus> {
  if (!window.edgeDesktop) {
    return getSidecarStatus()
  }
  return window.edgeDesktop.restartSidecar()
}

export async function restartBackend(): Promise<BackendRuntimeStatus> {
  return restartSidecar()
}

export async function openLogs() {
  if (!window.edgeDesktop) {
    return { logFile: '' }
  }
  return window.edgeDesktop.openLogs()
}

export async function readLogs(options?: { maxBytes?: number }): Promise<RuntimeLogTail> {
  if (!window.edgeDesktop) {
    return {
      logFile: '',
      size: 0,
      updatedAt: null,
      content: '',
    }
  }
  return window.edgeDesktop.readLogs(options)
}

export async function openExternal(url: string) {
  if (!window.edgeDesktop) {
    window.open(url, '_blank', 'noopener,noreferrer')
    return { opened: true }
  }
  return window.edgeDesktop.openExternal(url)
}
