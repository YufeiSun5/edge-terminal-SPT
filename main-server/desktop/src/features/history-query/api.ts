import { getJson } from '@/shared/api/http'
import type { HistoryDataParams, HistoryDataResponse } from '@/shared/api/types'

export function getHistoryData(params: HistoryDataParams = {}) {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      search.set(key, String(value))
    }
  }
  const query = search.toString()
  return getJson<HistoryDataResponse>(`/api/v1/history/data${query ? `?${query}` : ''}`)
}
