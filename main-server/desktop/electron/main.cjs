const { app, BrowserWindow, Menu, Tray, ipcMain, nativeImage, shell } = require('electron');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

const MAIN_SERVER_URL = process.env.MAIN_SERVER_URL || 'http://127.0.0.1:19080';
const isDev = !app.isPackaged;

let mainWindow = null;
let tray = null;
let minimizeToTray = true;
let isQuitting = false;
let serverStatus = {
  state: 'offline',
  pid: null,
  error: null,
  health: null,
  backendUrl: MAIN_SERVER_URL,
  logFile: null,
  restartAttempts: 0,
  watchdogEnabled: false,
};

function createTrayIcon() {
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <rect width="32" height="32" rx="8" fill="#111827"/>
      <path d="M7 9h18v4H7z" fill="#22c55e"/>
      <path d="M7 15h18v3H7z" fill="#60a5fa"/>
      <path d="M7 20h18v3H7z" fill="#f59e0b"/>
    </svg>`;
  return nativeImage.createFromDataURL(`data:image/svg+xml;base64,${Buffer.from(svg).toString('base64')}`);
}

function showMainWindow() {
  if (!mainWindow) {
    createWindow();
    return;
  }
  if (mainWindow.isMinimized()) {
    mainWindow.restore();
  }
  mainWindow.show();
  mainWindow.focus();
}

function getAutostartSettings() {
  const settings = app.getLoginItemSettings();
  return {
    openAtLogin: settings.openAtLogin,
    openAsHidden: settings.openAsHidden,
  };
}

function setAutostart(enabled) {
  app.setLoginItemSettings({
    openAtLogin: Boolean(enabled),
    openAsHidden: true,
  });
  return getAutostartSettings();
}

function updateTrayMenu() {
  if (!tray) return;
  const statusLabel = serverStatus.state === 'online' ? 'Main server online' : `Main server ${serverStatus.state}`;
  tray.setToolTip(`Spindle Main Server - ${statusLabel}`);
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: 'Show Main Server', click: showMainWindow },
    { label: statusLabel, enabled: false },
    { type: 'separator' },
    { label: 'Refresh Backend Status', click: () => void refreshServerHealth() },
    {
      label: 'Open Logs',
      click: async () => {
        const logFile = ensureLogFile();
        await shell.showItemInFolder(logFile);
      },
    },
    { type: 'separator' },
    {
      label: minimizeToTray ? 'Minimize to tray: On' : 'Minimize to tray: Off',
      type: 'checkbox',
      checked: minimizeToTray,
      click: (menuItem) => {
        minimizeToTray = menuItem.checked;
        updateTrayMenu();
      },
    },
    {
      label: 'Quit',
      click: () => {
        isQuitting = true;
        app.quit();
      },
    },
  ]));
}

function createTray() {
  if (tray) return tray;
  tray = new Tray(createTrayIcon());
  tray.on('double-click', showMainWindow);
  updateTrayMenu();
  return tray;
}

function desktopStatus() {
  return {
    autostart: getAutostartSettings(),
    minimizeToTray,
    trayAvailable: Boolean(tray),
    watchdogEnabled: false,
    restartAttempts: 0,
  };
}

function ensureLogFile() {
  const logDir = path.join(app.getPath('userData'), 'logs');
  fs.mkdirSync(logDir, { recursive: true });
  const logFile = path.join(logDir, 'main-server-desktop.log');
  serverStatus.logFile = logFile;
  return logFile;
}

async function readLogTail(maxBytes = 48000) {
  const logFile = ensureLogFile();
  try {
    const stat = await fs.promises.stat(logFile);
    const length = Math.min(Math.max(Number(maxBytes) || 48000, 1024), 256000);
    const start = Math.max(0, stat.size - length);
    const handle = await fs.promises.open(logFile, 'r');
    try {
      const buffer = Buffer.alloc(stat.size - start);
      await handle.read(buffer, 0, buffer.length, start);
      return {
        logFile,
        size: stat.size,
        updatedAt: stat.mtime.toISOString(),
        content: buffer.toString('utf8'),
      };
    } finally {
      await handle.close();
    }
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      return { logFile, size: 0, updatedAt: null, content: '' };
    }
    throw error;
  }
}

function checkServerHealth(timeoutMs = 1200) {
  return new Promise((resolve, reject) => {
    const request = http.get(`${MAIN_SERVER_URL}/health`, { timeout: timeoutMs }, (response) => {
      let body = '';
      response.setEncoding('utf8');
      response.on('data', (chunk) => {
        body += chunk;
      });
      response.on('end', () => {
        try {
          resolve({
            ok: response.statusCode >= 200 && response.statusCode < 300,
            statusCode: response.statusCode,
            body: body ? JSON.parse(body) : null,
          });
        } catch (error) {
          reject(error);
        }
      });
    });
    request.on('timeout', () => request.destroy(new Error('health check timeout')));
    request.on('error', reject);
  });
}

async function refreshServerHealth() {
  try {
    const health = await checkServerHealth();
    serverStatus = {
      ...serverStatus,
      state: health.ok ? 'online' : 'unhealthy',
      error: health.ok ? null : `health status ${health.statusCode}`,
      health: health.body,
    };
  } catch (error) {
    serverStatus = {
      ...serverStatus,
      state: 'offline',
      error: error.message,
      health: null,
    };
  }
  updateTrayMenu();
  return serverStatus;
}

function createWindow() {
  void refreshServerHealth();

  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 1180,
    minHeight: 760,
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });

  const devUrl = process.env.ELECTRON_RENDERER_URL;
  if (isDev && devUrl) {
    mainWindow.loadURL(devUrl);
    mainWindow.webContents.openDevTools({ mode: 'detach' });
  } else {
    mainWindow.loadFile(path.join(__dirname, '..', 'dist', 'index.html'));
  }

  mainWindow.on('close', (event) => {
    if (!isQuitting && minimizeToTray) {
      event.preventDefault();
      mainWindow.hide();
    }
  });
  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

function setupMenu() {
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
  ]));
}

ipcMain.handle('edge:get-app-info', () => ({
  isPackaged: app.isPackaged,
  version: app.getVersion(),
  userDataPath: app.getPath('userData'),
  backendUrl: MAIN_SERVER_URL,
}));

ipcMain.handle('edge:get-desktop-status', () => desktopStatus());
ipcMain.handle('edge:set-autostart', (_event, enabled) => ({ ...desktopStatus(), autostart: setAutostart(Boolean(enabled)) }));
ipcMain.handle('edge:set-minimize-to-tray', (_event, enabled) => {
  minimizeToTray = Boolean(enabled);
  updateTrayMenu();
  return desktopStatus();
});
ipcMain.handle('edge:get-sidecar-status', refreshServerHealth);
ipcMain.handle('edge:restart-sidecar', refreshServerHealth);
ipcMain.handle('edge:open-logs', async () => {
  const logFile = ensureLogFile();
  await shell.showItemInFolder(logFile);
  return { logFile };
});
ipcMain.handle('edge:read-logs', async (_event, options = {}) => readLogTail(options && typeof options.maxBytes === 'number' ? options.maxBytes : 48000));
ipcMain.handle('edge:open-external', async (_event, targetUrl) => {
  if (typeof targetUrl !== 'string' || !/^https?:\/\//.test(targetUrl)) {
    return { opened: false };
  }
  await shell.openExternal(targetUrl);
  return { opened: true };
});

app.whenReady().then(() => {
  setupMenu();
  createTray();
  createWindow();
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin' && !tray) {
    app.quit();
  }
});

app.on('before-quit', () => {
  isQuitting = true;
});

app.on('activate', showMainWindow);
