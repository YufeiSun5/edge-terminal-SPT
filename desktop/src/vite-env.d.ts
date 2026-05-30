/// <reference types="vite/client" />

import type { EdgeDesktopApi } from './shared/desktop/desktopBridge'

declare global {
  interface Window {
    edgeDesktop?: EdgeDesktopApi
  }
}
