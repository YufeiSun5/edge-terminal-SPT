# Spindle Edge Terminal Desktop

Electron + React shell for the edge-side Go backend.

## Commands

```powershell
npm install
npm run backend:build
npm run dev
npm run build
npm run package
```

The Electron main process starts `../backend/dist/edge-backend.exe` and passes `EDGE_CONFIG` to the bundled config file. The renderer talks to `http://127.0.0.1:18080` through typed API clients under `src/shared/api`.
