// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { getDesktopStatus, getSidecarStatus } from './desktopBridge'

describe('desktopBridge browser fallback', () => {
  it('reports unavailable desktop capabilities when Electron preload is absent', async () => {
    window.edgeDesktop = undefined

    await expect(getDesktopStatus()).resolves.toMatchObject({
      autostart: { openAtLogin: false },
      minimizeToTray: false,
      trayAvailable: false,
      watchdogEnabled: false,
      restartAttempts: 0,
    })
  })

  it('reports unavailable sidecar status when Electron preload is absent', async () => {
    window.edgeDesktop = undefined

    await expect(getSidecarStatus()).resolves.toMatchObject({
      state: 'unavailable',
      pid: null,
      backendUrl: 'http://127.0.0.1:18080',
    })
  })
})
