export type SpreadsheetMountOptions = {
  containerId: string
  data?: unknown
  readonly?: boolean
  toolbar?: boolean
  sheetbar?: boolean
}

export type SpreadsheetAdapter = {
  mount: (options: SpreadsheetMountOptions) => Promise<void>
  unmount: () => void
  importFile: (file: File) => Promise<void>
  exportFile: () => Promise<Blob>
  getWorkbook: () => unknown[]
}

type LuckysheetApi = {
  create: (options: Record<string, unknown>) => void
  destroy?: () => void
  getLuckysheetfile?: () => unknown
}

type LuckysheetWindow = Window &
  typeof globalThis & {
    luckysheet?: LuckysheetApi
  }

const VENDOR_BASE = '/luckysheet/vendor'
const STYLE_URL = `${VENDOR_BASE}/css/luckysheet.css`
const SCRIPT_URLS = [
  `${VENDOR_BASE}/jquery.js`,
  `${VENDOR_BASE}/jquery.mousewheel.js`,
  `${VENDOR_BASE}/luckysheet.umd.js`,
]

function loadStylesheet(documentRef: Document, href: string) {
  return new Promise<void>((resolve, reject) => {
    const link = documentRef.createElement('link')
    link.rel = 'stylesheet'
    link.href = href
    link.onload = () => resolve()
    link.onerror = () => reject(new Error(`Failed to load stylesheet: ${href}`))
    documentRef.head.appendChild(link)
  })
}

function loadScript(documentRef: Document, src: string) {
  return new Promise<void>((resolve, reject) => {
    const script = documentRef.createElement('script')
    script.src = src
    script.onload = () => resolve()
    script.onerror = () => reject(new Error(`Failed to load script: ${src}`))
    documentRef.body.appendChild(script)
  })
}

function normalizeSheets(data: unknown) {
  if (Array.isArray(data)) return data
  if (data && typeof data === 'object' && 'sheets' in data && Array.isArray((data as { sheets?: unknown }).sheets)) {
    return (data as { sheets: unknown[] }).sheets
  }
  return [{ name: 'Sheet1', index: '0', status: 1, order: 0 }]
}

function normalizeImages(data: unknown) {
  if (data && typeof data === 'object' && 'images' in data) {
    return (data as { images?: unknown }).images
  }
  return undefined
}

function normalizeWorkbook(data: unknown) {
  const sheets = normalizeSheets(data)
  const images = normalizeImages(data)

  if (images && sheets[0] && typeof sheets[0] === 'object' && !('images' in sheets[0])) {
    sheets[0] = { ...(sheets[0] as Record<string, unknown>), images }
  }

  return { sheets, images }
}

export function createLuckysheetAdapter(): SpreadsheetAdapter {
  let iframe: HTMLIFrameElement | undefined
  let luckyWindow: LuckysheetWindow | undefined
  let mountedContainer: HTMLElement | undefined

  async function prepareFrame(container: HTMLElement) {
    iframe = document.createElement('iframe')
    iframe.className = 'luckysheet-isolated-frame'
    iframe.title = 'Luckysheet report editor'
    iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-downloads')
    container.replaceChildren(iframe)

    const frameDocument = iframe.contentDocument
    if (!frameDocument || !iframe.contentWindow) {
      throw new Error('Luckysheet iframe is not available.')
    }

    frameDocument.open()
    frameDocument.write(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <style>
      html, body, #luckysheet-root {
        width: 100%;
        height: 100%;
        margin: 0;
        overflow: hidden;
        background: transparent;
        font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", sans-serif;
      }
      .luckysheet_info_detail { display: none !important; }
    </style>
  </head>
  <body>
    <div id="luckysheet-root"></div>
  </body>
</html>`)
    frameDocument.close()

    await loadStylesheet(frameDocument, STYLE_URL)
    for (const scriptUrl of SCRIPT_URLS) {
      await loadScript(frameDocument, scriptUrl)
    }
    luckyWindow = iframe.contentWindow as LuckysheetWindow
  }

  return {
    async mount(options) {
      const container = document.getElementById(options.containerId)
      if (!container) throw new Error(`Luckysheet container not found: ${options.containerId}`)

      mountedContainer = container
      await prepareFrame(container)

      const luckysheet = luckyWindow?.luckysheet
      if (!luckysheet?.create) {
        throw new Error('Luckysheet vendor assets are not ready.')
      }

      const workbook = normalizeWorkbook(options.data)

      luckysheet.destroy?.()
      luckysheet.create({
        container: 'luckysheet-root',
        lang: 'zh',
        data: workbook.sheets,
        images: workbook.images,
        showinfobar: false,
        showtoolbar: options.toolbar ?? !options.readonly,
        showsheetbar: options.sheetbar ?? true,
        allowEdit: !options.readonly,
        enableAddRow: !options.readonly,
        enableAddBackTop: false,
      })
    },
    unmount() {
      try {
        luckyWindow?.luckysheet?.destroy?.()
      } catch {
        // Luckysheet destroy can throw when the iframe is already detached.
      }
      mountedContainer?.replaceChildren()
      iframe = undefined
      luckyWindow = undefined
      mountedContainer = undefined
    },
    async importFile() {
      throw new Error('Luckysheet file import needs the future report-template API and is not enabled in the static screen.')
    },
    async exportFile() {
      const file = luckyWindow?.luckysheet?.getLuckysheetfile?.() ?? []
      return new Blob([JSON.stringify(file, null, 2)], { type: 'application/json' })
    },
    getWorkbook() {
      const file = luckyWindow?.luckysheet?.getLuckysheetfile?.()
      return Array.isArray(file) ? file : []
    },
  }
}
