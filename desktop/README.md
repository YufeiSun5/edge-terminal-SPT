# Spindle Desktop Frontend

Electron + React shell for the edge terminal and main-server web/desktop frontend.

`desktop/src` is the single delivery frontend. Run it as the edge terminal with
`VITE_APP_ROLE=edge` or as the main-server frontend with `VITE_APP_ROLE=main_server`.
Do not add new business pages under `main-server/desktop`; that directory is a
transitional copy only.

## Commands

```powershell
npm install
npm run backend:build
npm run dev
npm run dev:main-server
npm run build
npm run package
```

Edge mode starts `../backend/dist/edge-backend.exe` and passes `EDGE_CONFIG` to
the bundled config file. The renderer talks to `http://127.0.0.1:18080` through
typed API clients under `src/shared/api`.

Main-server mode does not start the edge sidecar. It checks
`http://127.0.0.1:19080`, serves the renderer on `http://127.0.0.1:5273`, and
uses the same route, DTO, and WS semantics as edge mode:

```powershell
npm run dev:main-server
npm run smoke:main-server
```
