# Spindle Main Server Desktop

Deprecated transitional copy of the desktop frontend.

New frontend work must be implemented in `../../desktop/src` and run with
`VITE_APP_ROLE=main_server`. This directory must not grow independent business
pages, API clients, mock report flows, local station-view state, or direct gateway
maintenance behavior. If it is kept temporarily, it should only receive mechanical
syncs from the mainline frontend.

## Commands

```powershell
npm install
npm run dev
npm run build
npm run lint
```

Business validation for the main server must use `../../desktop`:

```powershell
cd ..\..\desktop
npm run dev:main-server
npm run smoke:main-server
```
