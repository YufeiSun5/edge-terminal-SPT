const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('edgeDesktop', {
  getAppInfo: () => ipcRenderer.invoke('edge:get-app-info'),
  getDesktopStatus: () => ipcRenderer.invoke('edge:get-desktop-status'),
  setAutostart: (enabled) => ipcRenderer.invoke('edge:set-autostart', enabled),
  setMinimizeToTray: (enabled) => ipcRenderer.invoke('edge:set-minimize-to-tray', enabled),
  getSidecarStatus: () => ipcRenderer.invoke('edge:get-sidecar-status'),
  restartSidecar: () => ipcRenderer.invoke('edge:restart-sidecar'),
  openLogs: () => ipcRenderer.invoke('edge:open-logs'),
  readLogs: (options) => ipcRenderer.invoke('edge:read-logs', options),
  openExternal: (url) => ipcRenderer.invoke('edge:open-external', url),
});
