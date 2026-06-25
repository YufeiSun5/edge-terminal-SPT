import ExcelJS from 'exceljs'

export type SpreadsheetMountOptions = {
  containerId: string
  data?: unknown
  readonly?: boolean
  toolbar?: boolean
  sheetbar?: boolean
  deferCreate?: boolean
  onCellSelect?: (selection: SpreadsheetCellSelection) => void
}

export type SpreadsheetCellSelection = {
  sheetName: string
  address: string
  row: number
  column: number
  value: string
  mergedAddress?: string
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

type LuckysheetCell = {
  r: number
  c: number
  v?: Record<string, unknown>
}

type LuckysheetSheet = {
  name?: string
  status?: number
  images?: unknown
  celldata?: LuckysheetCell[]
  config?: {
    merge?: Record<string, { r: number; c: number; rs: number; cs: number }>
  }
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
  const sheets = normalizeSheets(data) as LuckysheetSheet[]
  const images = normalizeImages(data)

  if (images && sheets[0] && typeof sheets[0] === 'object' && !('images' in sheets[0])) {
    sheets[0] = { ...(sheets[0] as Record<string, unknown>), images }
  }

  return { sheets, images }
}

function columnNumberToName(columnIndex: number) {
  let column = columnIndex + 1
  let name = ''
  while (column > 0) {
    const remainder = (column - 1) % 26
    name = String.fromCharCode(65 + remainder) + name
    column = Math.floor((column - 1) / 26)
  }
  return name
}

function cellAddress(rowIndex: number, columnIndex: number) {
  return `${columnNumberToName(columnIndex)}${rowIndex + 1}`
}

function luckysheetDisplayValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value !== 'object') return String(value)
  const cell = value as Record<string, unknown>
  const display = cell.m ?? cell.v
  if (display === null || display === undefined) return ''
  return typeof display === 'object' ? String(display) : String(display)
}

function activeLuckysheetSheet(sheets: LuckysheetSheet[]) {
  return sheets.find((sheet) => sheet.status === 1) ?? sheets[0]
}

function resolveMergedCell(sheet: LuckysheetSheet, rowIndex: number, columnIndex: number) {
  const merge = sheet.config?.merge ?? {}
  const direct = merge[`${rowIndex}_${columnIndex}`]
  if (direct) return direct
  return Object.values(merge).find(
    (item) => rowIndex >= item.r && rowIndex < item.r + item.rs && columnIndex >= item.c && columnIndex < item.c + item.cs,
  )
}

function resolveSelection(sheets: LuckysheetSheet[], rowIndex: number, columnIndex: number): SpreadsheetCellSelection | undefined {
  const sheet = activeLuckysheetSheet(sheets)
  if (!sheet) return undefined
  const merge = resolveMergedCell(sheet, rowIndex, columnIndex)
  const sourceRow = merge?.r ?? rowIndex
  const sourceColumn = merge?.c ?? columnIndex
  const cell = sheet.celldata?.find((item) => item.r === sourceRow && item.c === sourceColumn)
  return {
    sheetName: sheet.name ?? '',
    address: cellAddress(sourceRow, sourceColumn),
    row: sourceRow + 1,
    column: sourceColumn + 1,
    value: luckysheetDisplayValue(cell?.v),
    mergedAddress: merge ? cellAddress(rowIndex, columnIndex) : undefined,
  }
}

function cellDisplayValue(value: ExcelJS.CellValue) {
  if (value === null || value === undefined) return ''
  if (value instanceof Date) return value.toISOString()
  if (typeof value !== 'object') return value
  if ('formula' in value) return (value.result ?? `=${value.formula}`) as string | number | boolean
  if ('richText' in value) return value.richText.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
  if ('text' in value) return value.text
  if ('hyperlink' in value) return value.hyperlink
  return String(value)
}

function excelColorToCss(color: Partial<ExcelJS.Color> | undefined, fallback?: string) {
  const argb = color?.argb
  if (argb) return `#${argb.slice(-6)}`
  return fallback
}

function excelFillToColor(fill: ExcelJS.Fill | undefined) {
  if (!fill || fill.type !== 'pattern') return undefined
  return excelColorToCss(fill.fgColor, excelColorToCss(fill.bgColor))
}

function excelFontColor(font: Partial<ExcelJS.Font> | undefined) {
  return excelColorToCss(font?.color)
}

const borderStyleMap: Record<ExcelJS.BorderStyle, number> = {
  thin: 1,
  hair: 2,
  dotted: 3,
  dashed: 4,
  dashDot: 5,
  dashDotDot: 6,
  double: 7,
  medium: 8,
  mediumDashed: 9,
  mediumDashDot: 10,
  mediumDashDotDot: 11,
  slantDashDot: 12,
  thick: 13,
}

function excelBorderSideToLuckysheet(side: Partial<ExcelJS.Border> | undefined) {
  if (!side?.style) return undefined
  return {
    style: borderStyleMap[side.style] ?? 1,
    color: excelColorToCss(side.color, '#000000'),
  }
}

function cellBorderInfo(cell: ExcelJS.Cell, rowIndex: number, columnIndex: number) {
  const border = cell.border
  if (!border) return undefined
  const value: Record<string, unknown> = {
    row_index: rowIndex,
    col_index: columnIndex,
  }
  const left = excelBorderSideToLuckysheet(border.left)
  const right = excelBorderSideToLuckysheet(border.right)
  const top = excelBorderSideToLuckysheet(border.top)
  const bottom = excelBorderSideToLuckysheet(border.bottom)
  if (left) value.l = left
  if (right) value.r = right
  if (top) value.t = top
  if (bottom) value.b = bottom
  if (!left && !right && !top && !bottom) return undefined
  return {
    rangeType: 'cell',
    value,
  }
}

function excelHorizontalToLuckysheet(horizontal: ExcelJS.Alignment['horizontal'] | undefined) {
  switch (horizontal) {
    case 'center':
    case 'centerContinuous':
    case 'distributed':
    case 'justify':
      return 0
    case 'right':
      return 2
    case 'left':
    case 'fill':
      return 1
    default:
      return undefined
  }
}

function excelVerticalToLuckysheet(vertical: ExcelJS.Alignment['vertical'] | undefined) {
  switch (vertical) {
    case 'middle':
    case 'distributed':
    case 'justify':
      return 0
    case 'top':
      return 1
    case 'bottom':
      return 2
    default:
      return undefined
  }
}

function excelTextRotationToLuckysheet(textRotation: ExcelJS.Alignment['textRotation'] | undefined) {
  if (textRotation === 'vertical') return 3
  if (typeof textRotation !== 'number') return undefined
  if (textRotation === 0) return 0
  if (textRotation === 45) return 1
  if (textRotation === -45) return 2
  if (textRotation === 90) return 4
  if (textRotation === -90) return 5
  return undefined
}

function bufferToBase64(buffer: ExcelJS.Image['buffer']) {
  if (!buffer) return undefined
  const bytes = buffer instanceof ArrayBuffer ? new Uint8Array(buffer) : new Uint8Array(buffer as ArrayBufferLike)
  let binary = ''
  const chunkSize = 0x8000
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.slice(index, index + chunkSize))
  }
  return btoa(binary)
}

function imageSource(workbook: ExcelJS.Workbook, imageId: string | number) {
  const image = workbook.getImage(Number(imageId))
  if (!image) return undefined
  const base64 = image.base64 || bufferToBase64(image.buffer)
  if (!base64) return undefined
  const extension = image.extension === 'jpeg' ? 'jpeg' : image.extension || 'png'
  return base64.startsWith('data:') ? base64 : `data:image/${extension};base64,${base64}`
}

const DEFAULT_COLUMN_WIDTH = 73
const DEFAULT_ROW_HEIGHT = 19

function excelColumnWidthToPixels(width: number | undefined) {
  return width ? Math.round(width * 8) : DEFAULT_COLUMN_WIDTH
}

function excelRowHeightToPixels(height: number | undefined) {
  return height ? Math.round(height * 1.35) : DEFAULT_ROW_HEIGHT
}

function sheetColumnPixelWidth(sheet: ExcelJS.Worksheet, columnIndex: number) {
  return excelColumnWidthToPixels(sheet.getColumn(columnIndex + 1).width)
}

function sheetRowPixelHeight(sheet: ExcelJS.Worksheet, rowIndex: number) {
  return excelRowHeightToPixels(sheet.getRow(rowIndex + 1).height)
}

function anchorPositionToPixels(sheet: ExcelJS.Worksheet, column: number, row: number) {
  const wholeColumn = Math.floor(column)
  const wholeRow = Math.floor(row)
  const columnFraction = column - wholeColumn
  const rowFraction = row - wholeRow
  let left = 0
  let top = 0

  for (let index = 0; index < wholeColumn; index += 1) {
    left += sheetColumnPixelWidth(sheet, index)
  }
  for (let index = 0; index < wholeRow; index += 1) {
    top += sheetRowPixelHeight(sheet, index)
  }

  left += columnFraction * sheetColumnPixelWidth(sheet, wholeColumn)
  top += rowFraction * sheetRowPixelHeight(sheet, wholeRow)

  return {
    left: Math.max(0, Math.round(left)),
    top: Math.max(0, Math.round(top)),
  }
}

function sheetImages(workbook: ExcelJS.Workbook, sheet: ExcelJS.Worksheet) {
  const images: Record<string, Record<string, unknown>> = {}
  sheet.getImages().forEach((item, index) => {
    const src = imageSource(workbook, item.imageId)
    if (!src) return
    const range = item.range as {
      tl?: { col?: number; row?: number }
      br?: { col?: number; row?: number }
      ext?: { width?: number; height?: number }
    }
    const tl = range.tl
    if (typeof tl?.col !== 'number' || typeof tl?.row !== 'number') return
    const start = anchorPositionToPixels(sheet, tl.col, tl.row)
    const br = range.br
    const left = start.left
    const top = start.top
    let width = Math.max(120, Math.round(range.ext?.width ?? 0))
    let height = Math.max(80, Math.round(range.ext?.height ?? 0))
    if (typeof br?.col === 'number' && typeof br?.row === 'number') {
      const end = anchorPositionToPixels(sheet, br.col, br.row)
      width = Math.max(120, end.left - start.left)
      height = Math.max(80, end.top - start.top)
    }
    images[`image_${index}`] = {
      type: '2',
      src,
      default: {
        width,
        height,
        left,
        top,
      },
      crop: {
        width,
        height,
        offsetLeft: 0,
        offsetTop: 0,
      },
      isFixedPos: false,
      fixedLeft: left,
      fixedTop: top,
      border: {
        width: 0,
        radius: 0,
        style: 'solid',
        color: '#000',
      },
    }
  })
  return Object.keys(images).length ? images : undefined
}

function columnNameToNumber(columnName: string) {
  return columnName.split('').reduce((total, char) => total * 26 + char.toUpperCase().charCodeAt(0) - 64, 0)
}

function decodeCellAddress(address: string) {
  const match = /^([A-Z]+)(\d+)$/i.exec(address)
  if (!match) return undefined
  return {
    column: columnNameToNumber(match[1]),
    row: Number(match[2]),
  }
}

function sheetMergeConfig(sheet: ExcelJS.Worksheet) {
  const merges = ((sheet.model as ExcelJS.WorksheetModel & { merges?: string[] }).merges ?? []) as string[]
  const merge: Record<string, { r: number; c: number; rs: number; cs: number }> = {}
  const mergedCells = new Set<string>()
  const masterCells = new Map<string, { r: number; c: number; rs: number; cs: number }>()

  merges.forEach((range) => {
    const [start, end] = range.split(':')
    const startCell = decodeCellAddress(start)
    const endCell = decodeCellAddress(end)
    if (!startCell || !endCell) return
    const r = startCell.row - 1
    const c = startCell.column - 1
    const rs = endCell.row - startCell.row + 1
    const cs = endCell.column - startCell.column + 1
    const mergeInfo = { r, c, rs, cs }
    merge[`${r}_${c}`] = mergeInfo
    masterCells.set(`${r}_${c}`, mergeInfo)
    for (let row = r; row < r + rs; row += 1) {
      for (let column = c; column < c + cs; column += 1) {
        if (row !== r || column !== c) mergedCells.add(`${row}_${column}`)
      }
    }
  })

  return { merge, mergedCells, masterCells }
}

async function xlsxFileToLuckysheetData(file: File) {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.load(await file.arrayBuffer())

  const visibleWorksheets = workbook.worksheets.filter((sheet) => sheet.state !== 'hidden' && sheet.state !== 'veryHidden')
  const sourceSheets = visibleWorksheets.length > 0 ? visibleWorksheets : workbook.worksheets

  const sheets = sourceSheets.map((sheet, sheetIndex) => {
    const celldata: Array<{ r: number; c: number; v: Record<string, unknown> }> = []
    const borderInfo: Array<Record<string, unknown>> = []
    const columnlen: Record<number, number> = {}
    const rowlen: Record<number, number> = {}
    const { merge, mergedCells, masterCells } = sheetMergeConfig(sheet)

    sheet.columns.forEach((column, index) => {
      if (column.width) columnlen[index] = excelColumnWidthToPixels(column.width)
    })

    sheet.eachRow({ includeEmpty: true }, (row, rowNumber) => {
      if (row.height) rowlen[rowNumber - 1] = excelRowHeightToPixels(row.height)
      row.eachCell({ includeEmpty: true }, (cell, colNumber) => {
        const cellKey = `${rowNumber - 1}_${colNumber - 1}`
        const border = cellBorderInfo(cell, rowNumber - 1, colNumber - 1)
        if (border) borderInfo.push(border)
        if (mergedCells.has(cellKey)) return
        const value = cellDisplayValue(cell.value)
        const style: Record<string, unknown> = {
          v: value,
          m: value === undefined || value === null ? '' : String(value),
          ct: { fa: cell.numFmt || 'General', t: typeof value === 'number' ? 'n' : 'g' },
        }
        const font = cell.font
        if (font?.bold) style.bl = 1
        if (font?.italic) style.it = 1
        if (font?.underline) style.un = 1
        if (font?.strike) style.cl = 1
        if (font?.size) style.fs = font.size
        if (font?.name) style.ff = font.name
        const fontColor = excelFontColor(font)
        if (fontColor) style.fc = fontColor
        const background = excelFillToColor(cell.fill)
        if (background) style.bg = background
        const horizontal = excelHorizontalToLuckysheet(cell.alignment?.horizontal)
        const vertical = excelVerticalToLuckysheet(cell.alignment?.vertical)
        const textRotation = excelTextRotationToLuckysheet(cell.alignment?.textRotation)
        if (horizontal !== undefined) style.ht = horizontal
        if (vertical !== undefined) style.vt = vertical
        if (cell.alignment?.wrapText) style.tb = 2
        if (textRotation !== undefined) style.tr = textRotation
        const mergeInfo = masterCells.get(cellKey)
        if (mergeInfo) style.mc = mergeInfo
        celldata.push({ r: rowNumber - 1, c: colNumber - 1, v: style })
      })
    })

    return {
      name: sheet.name,
      index: String(sheetIndex),
      status: sheetIndex === 0 ? 1 : 0,
      order: sheetIndex,
      images: sheetImages(workbook, sheet),
      celldata,
      config: {
        columnlen,
        rowlen,
        merge,
        borderInfo,
      },
    }
  })
  return { sheets }
}

export function createLuckysheetAdapter(): SpreadsheetAdapter {
  let iframe: HTMLIFrameElement | undefined
  let luckyWindow: LuckysheetWindow | undefined
  let mountedContainer: HTMLElement | undefined
  let currentWorkbook: { sheets: LuckysheetSheet[]; images?: unknown } | undefined
  let cellSelectHandler: ((selection: SpreadsheetCellSelection) => void) | undefined

  function createOptions(options: SpreadsheetMountOptions, workbook: { sheets: LuckysheetSheet[]; images?: unknown }, readonly: boolean) {
    currentWorkbook = workbook
    cellSelectHandler = options.onCellSelect
    return {
      container: 'luckysheet-root',
      lang: 'zh',
      data: workbook.sheets,
      images: workbook.images,
      showinfobar: false,
      showtoolbar: options.toolbar ?? !readonly,
      showsheetbar: options.sheetbar ?? true,
      allowEdit: !readonly,
      enableAddRow: !readonly,
      enableAddBackTop: false,
      hook: {
        cellMousedown: (_cell: unknown, position: { r?: number; c?: number; start_r?: number; start_c?: number }) => {
          const rowIndex = typeof position?.r === 'number' ? position.r : position?.start_r
          const columnIndex = typeof position?.c === 'number' ? position.c : position?.start_c
          if (typeof rowIndex !== 'number' || typeof columnIndex !== 'number') return
          const liveWorkbook = normalizeWorkbook(luckyWindow?.luckysheet?.getLuckysheetfile?.() ?? currentWorkbook)
          const selection = resolveSelection(liveWorkbook.sheets, rowIndex, columnIndex)
          if (selection) cellSelectHandler?.(selection)
        },
      },
    }
  }

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
      cellSelectHandler = options.onCellSelect
      await prepareFrame(container)

      const luckysheet = luckyWindow?.luckysheet
      if (!luckysheet?.create) {
        throw new Error('Luckysheet vendor assets are not ready.')
      }

      if (!options.deferCreate) {
        const workbook = normalizeWorkbook(options.data)

        luckysheet.destroy?.()
        luckysheet.create(createOptions(options, workbook, options.readonly ?? false))
      }
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
      currentWorkbook = undefined
      cellSelectHandler = undefined
    },
    async importFile(file) {
      const container = mountedContainer
      if (!container) throw new Error('Luckysheet has not been mounted.')
      const data = await xlsxFileToLuckysheetData(file)
      const workbook = normalizeWorkbook(data)
      const luckysheet = luckyWindow?.luckysheet
      if (!luckysheet?.create) throw new Error('Luckysheet vendor assets are not ready.')
      luckysheet.destroy?.()
      luckysheet.create(
        createOptions(
          {
            containerId: 'luckysheet-root',
            readonly: true,
            toolbar: false,
            sheetbar: true,
            onCellSelect: cellSelectHandler,
          },
          workbook,
          true,
        ),
      )
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
