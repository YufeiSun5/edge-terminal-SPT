export type SpreadsheetMountOptions = {
  containerId: string
  data?: unknown
  readonly?: boolean
}

export type SpreadsheetAdapter = {
  mount: (options: SpreadsheetMountOptions) => Promise<void>
  unmount: () => void
  importFile: (file: File) => Promise<void>
  exportFile: () => Promise<Blob>
}

export function createLuckysheetAdapter(): SpreadsheetAdapter {
  return {
    async mount() {
      throw new Error('Luckysheet is a legacy dependency and must be mounted only after its vendor assets are isolated.')
    },
    unmount() {},
    async importFile() {
      throw new Error('Luckysheet import is not wired yet.')
    },
    async exportFile() {
      throw new Error('Luckysheet export is not wired yet.')
    },
  }
}
