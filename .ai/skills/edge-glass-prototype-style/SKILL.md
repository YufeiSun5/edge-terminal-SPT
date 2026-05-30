---
name: edge-glass-prototype-style
description: Use when restoring or extending Edge Terminal React screens from react-design-prototype, especially the station operation and history query glassmorphism pages, dynamic background light, card-edge illumination, pill controls, special scrollbars, and prototype-matched spacing.
argument-hint: <target page or component>
---

# Edge Glass Prototype Style

## Workflow

1. Run or inspect `react-design-prototype/` before editing `desktop/`.
2. Compare source files and screenshots, not memory:
   - Station: `src/pages/InspectionScreen.jsx`, `src/components/InspectionScreen/SortableCardGrid.jsx`, `GlobalLightBackground.jsx`, `RightControlPanel.jsx`.
   - History: `src/pages/HistoryScreen.jsx`, `src/components/HistoryScreen/TrendChart.jsx`, `HistoryTable.jsx`, `GanttChartModal.jsx`.
3. Preserve component boundaries in `desktop/src/features/<feature>/components/`; avoid putting chart, table, modal, light engine, and style engine back into page files.
4. After edits, verify with Playwright screenshots against both prototype and desktop at the same viewport.

## Station Glass Rules

- The yellow moving orb is the light source. It must update card CSS variables each animation frame:
  - `--mouse-x = lightX - panelRect.left`
  - `--mouse-y = lightY - panelRect.top`
- Scope the selector to `.station-page .glass-panel` in desktop to avoid affecting other pages.
- The exact edge glow is a masked pseudo-element:
  - `radial-gradient(600px circle at var(--mouse-x) var(--mouse-y), rgba(255, 220, 150, 1) 0%, rgba(255,255,255,0.9) 10%, rgba(255,255,255,0.1) 40%, rgba(255,255,255,0) 60%)`
  - `-webkit-mask-composite: xor; mask-composite: exclude;`
- The glass body uses `rgba(255,255,255,0.25)`, `blur(24px) saturate(150%)`, `0 8px 32px rgba(0,0,0,0.08)`, and an inset `rgba(255,255,255,0.1)`.
- Card grid must keep the prototype container-query behavior:
  - outer `.grid-scroll-container` with `container-type: size`
  - inner grid rows `minmax(200px, calc((100cqh - 40px) / 3))`
  - 20px gap, so the visible viewport shows complete rows such as 6 cards at the tested desktop size.

## History Page Rules

- Use the prototype light background: base `#e6eaf0`, three blurred radial color blocks, and SVG noise at `opacity: 0.03`.
- Buttons are `.glass-btn` pills: 20px radius, `rgba(255,255,255,0.7)`, 20px blur, white border, subtle inset highlight.
- Panels use `rgba(255,255,255,0.65)`, `blur(40px) saturate(150%)`, radius 16px, and inset `rgba(255,255,255,0.8)`.
- Keep the original history split: toolbar, chart panel `flex: 0 0 42%`, table panel filling the remaining height.
- Use AntD Table virtual scrolling and the prototype table styling: fixed left timestamp, black 2px header bottom border, subtle row striping, 8px custom table scrollbar.
- Use the prototype custom portal modal for the Gantt chart rather than AntD Modal when matching visual fidelity.

## Verification

- Run `npm run lint` and `npm run build` from `desktop/`.
- Capture desktop and prototype screenshots with identical viewport, usually `1858x895`.
- Check these visual points before finishing:
  - station cards show complete adaptive rows, not a clipped fourth row;
  - card edges brighten near the moving yellow orb;
  - right table scrollbar is hidden until hover;
  - history toolbar, glass panels, pill buttons, chart, table, and Gantt modal match the prototype structure.
