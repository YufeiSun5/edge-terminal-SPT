const { app, BrowserWindow, Menu, Tray, ipcMain, nativeImage, shell } = require('electron');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

const RUNTIME_ROLE = process.env.VITE_APP_ROLE === 'main_server' || process.env.APP_ROLE === 'main_server'
  ? 'main_server'
  : 'edge';
const BACKEND_URL = RUNTIME_ROLE === 'main_server'
  ? (process.env.MAIN_SERVER_URL || process.env.VITE_MAIN_API_BASE_URL || 'http://127.0.0.1:19080')
  : (process.env.EDGE_BACKEND_URL || process.env.VITE_EDGE_API_BASE_URL || 'http://127.0.0.1:18080');
const isDev = !app.isPackaged;
const hasManagedSidecar = RUNTIME_ROLE === 'edge';

let mainWindow = null;
let tray = null;
let sidecarProcess = null;
let sidecarRestartTimer = null;
let sidecarRestartAttempts = 0;
let intentionalSidecarStop = false;
let isQuitting = false;
let minimizeToTray = true;
let sidecarStatus = {
  state: 'stopped',
  pid: null,
  error: null,
  health: null,
  backendUrl: BACKEND_URL,
  logFile: null,
  restartAttempts: 0,
  watchdogEnabled: hasManagedSidecar,
};

function createTrayIcon() {
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <rect width="32" height="32" rx="8" fill="#0f172a"/>
      <path d="M8 18.5h16v3H8z" fill="#60a5fa"/>
      <path d="M8 10h16v3H8z" fill="#f59e0b"/>
      <path d="M10 15h12v2H10z" fill="#e2e8f0"/>
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
  const appLabel = RUNTIME_ROLE === 'main_server' ? 'Spindle Main Server' : 'Spindle Edge Terminal';
  const statusLabel = sidecarStatus.state === 'online' ? 'Backend online' : `Backend ${sidecarStatus.state}`;
  tray.setToolTip(`${appLabel} - ${statusLabel}`);
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: RUNTIME_ROLE === 'main_server' ? 'Show Main Server' : 'Show Edge Terminal', click: showMainWindow },
    { label: statusLabel, enabled: false },
    { type: 'separator' },
    { label: hasManagedSidecar ? 'Restart Backend' : 'Refresh Backend Status', click: () => void restartSidecar() },
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

function emitDesktopState() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('edge:desktop-state-changed');
  }
}

function desktopStatus() {
  return {
    autostart: getAutostartSettings(),
    minimizeToTray,
    trayAvailable: Boolean(tray),
    watchdogEnabled: hasManagedSidecar,
    restartAttempts: hasManagedSidecar ? sidecarRestartAttempts : 0,
  };
}

function resolveBackendPaths() {
  if (app.isPackaged) {
    const backendDir = path.join(process.resourcesPath, 'backend');
    return {
      exePath: path.join(backendDir, 'edge-backend.exe'),
      configPath: path.join(backendDir, 'configs', 'config.json'),
      backendDir,
    };
  }

  const projectRoot = path.resolve(__dirname, '..', '..');
  const distDir = path.join(projectRoot, 'backend', 'dist');
  const distConfig = path.join(distDir, 'configs', 'config.json');
  const sourceConfig = path.join(projectRoot, 'backend', 'configs', 'config.json');

  return {
    exePath: path.join(distDir, 'edge-backend.exe'),
    configPath: fs.existsSync(distConfig) ? distConfig : sourceConfig,
    backendDir: distDir,
  };
}

function ensureLogFile() {
  const logDir = path.join(app.getPath('userData'), 'logs');
  fs.mkdirSync(logDir, { recursive: true });
  const logFile = path.join(logDir, hasManagedSidecar ? 'edge-backend.log' : 'main-server-desktop.log');
  sidecarStatus.logFile = logFile;
  return logFile;
}

function appendSidecarLog(chunk) {
  const logFile = ensureLogFile();
  fs.appendFile(logFile, chunk.toString(), (error) => {
    if (error) {
      console.error('write sidecar log failed', error);
    }
  });
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
      return {
        logFile,
        size: 0,
        updatedAt: null,
        content: '',
      };
    }
    throw error;
  }
}

function checkBackendHealth(timeoutMs = 1200) {
  return new Promise((resolve, reject) => {
    const request = http.get(`${BACKEND_URL}/health`, { timeout: timeoutMs }, (response) => {
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

    request.on('timeout', () => {
      request.destroy(new Error('health check timeout'));
    });
    request.on('error', reject);
  });
}

async function refreshSidecarHealth() {
  try {
    const health = await checkBackendHealth();
    sidecarStatus = {
      ...sidecarStatus,
      state: health.ok ? 'online' : 'unhealthy',
      error: health.ok ? null : `health status ${health.statusCode}`,
      health: health.body,
    };
  } catch (error) {
    sidecarStatus = {
      ...sidecarStatus,
      state: sidecarProcess ? 'starting' : 'offline',
      error: error.message,
      health: null,
    };
  }
  updateTrayMenu();
  return sidecarStatus;
}

function startSidecar() {
  if (!hasManagedSidecar) {
    void refreshSidecarHealth();
    return sidecarStatus;
  }

  if (sidecarProcess && !sidecarProcess.killed) {
    return sidecarStatus;
  }

  const { exePath, configPath, backendDir } = resolveBackendPaths();
  ensureLogFile();

  if (!fs.existsSync(exePath)) {
    sidecarStatus = {
      ...sidecarStatus,
      state: 'missing',
      pid: null,
      error: `missing backend executable: ${exePath}`,
    };
    appendSidecarLog(`[electron] ${sidecarStatus.error}\n`);
    updateTrayMenu();
    return sidecarStatus;
  }

  intentionalSidecarStop = false;
  if (sidecarRestartTimer) {
    clearTimeout(sidecarRestartTimer);
    sidecarRestartTimer = null;
  }

  sidecarProcess = spawn(exePath, {
    cwd: backendDir,
    env: {
      ...process.env,
      EDGE_CONFIG: configPath,
    },
    windowsHide: true,
  });

  sidecarStatus = {
    ...sidecarStatus,
    state: 'starting',
    pid: sidecarProcess.pid,
    error: null,
    health: null,
    restartAttempts: sidecarRestartAttempts,
  };

  appendSidecarLog(`[electron] starting sidecar pid=${sidecarProcess.pid} exe=${exePath} config=${configPath}\n`);
  updateTrayMenu();
  emitDesktopState();
  sidecarProcess.stdout.on('data', appendSidecarLog);
  sidecarProcess.stderr.on('data', appendSidecarLog);
  sidecarProcess.on('error', (error) => {
    sidecarStatus = { ...sidecarStatus, state: 'failed', error: error.message };
    appendSidecarLog(`[electron] sidecar error: ${error.message}\n`);
    updateTrayMenu();
    emitDesktopState();
  });
  sidecarProcess.on('exit', (code, signal) => {
    appendSidecarLog(`[electron] sidecar exited code=${code} signal=${signal}\n`);
    sidecarProcess = null;
    const shouldRestart = !isQuitting && !intentionalSidecarStop && code !== 0 && sidecarRestartAttempts < 5;
    sidecarStatus = {
      ...sidecarStatus,
      state: shouldRestart ? 'starting' : 'stopped',
      pid: null,
      error: code === 0 ? null : `sidecar exited with code ${code ?? signal}`,
      health: null,
      restartAttempts: sidecarRestartAttempts,
    };
    updateTrayMenu();
    emitDesktopState();
    if (shouldRestart) {
      sidecarRestartAttempts += 1;
      const delay = Math.min(2000 * sidecarRestartAttempts, 10000);
      appendSidecarLog(`[electron] restarting sidecar attempt=${sidecarRestartAttempts} delay=${delay}ms\n`);
      sidecarRestartTimer = setTimeout(() => {
        sidecarRestartTimer = null;
        startSidecar();
      }, delay);
    }
  });

  setTimeout(refreshSidecarHealth, 1000);
  setTimeout(refreshSidecarHealth, 2500);
  return sidecarStatus;
}

async function restartSidecar() {
  if (!hasManagedSidecar) {
    return refreshSidecarHealth();
  }

  if (sidecarProcess && !sidecarProcess.killed) {
    intentionalSidecarStop = true;
    sidecarProcess.kill();
  }
  sidecarProcess = null;
  sidecarRestartAttempts = 0;
  startSidecar();
  return refreshSidecarHealth();
}

function createWindow() {
  startSidecar();

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
  const template = [
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

ipcMain.handle('edge:get-app-info', () => ({
  isPackaged: app.isPackaged,
  version: app.getVersion(),
  userDataPath: app.getPath('userData'),
  backendUrl: BACKEND_URL,
}));

ipcMain.handle('edge:get-desktop-status', () => desktopStatus());

ipcMain.handle('edge:set-autostart', (_event, enabled) => ({
  ...desktopStatus(),
  autostart: setAutostart(Boolean(enabled)),
}));

ipcMain.handle('edge:set-minimize-to-tray', (_event, enabled) => {
  minimizeToTray = Boolean(enabled);
  updateTrayMenu();
  return desktopStatus();
});

ipcMain.handle('edge:get-sidecar-status', async () => {
  await refreshSidecarHealth();
  return sidecarStatus;
});

ipcMain.handle('edge:restart-sidecar', restartSidecar);

ipcMain.handle('edge:open-logs', async () => {
  const logFile = ensureLogFile();
  await shell.showItemInFolder(logFile);
  return { logFile };
});

ipcMain.handle('edge:read-logs', async (_event, options = {}) => {
  const maxBytes = options && typeof options.maxBytes === 'number' ? options.maxBytes : 48000;
  return readLogTail(maxBytes);
});

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
  if (sidecarRestartTimer) {
    clearTimeout(sidecarRestartTimer);
    sidecarRestartTimer = null;
  }
  if (sidecarProcess && !sidecarProcess.killed) {
    intentionalSidecarStop = true;
    sidecarProcess.kill();
  }
});

app.on('activate', showMainWindow);
