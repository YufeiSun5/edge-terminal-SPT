import { getJson } from '@/shared/api/http'
import type { HistoryDataParams, HistoryDataResponse } from '@/shared/api/types'

export function getHistoryData(params: HistoryDataParams = {}) {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      if (key === 'device_id') {
        if (!params.project_id) search.set('project_id', String(value))
      } else if (key === 'device_code') {
        if (!params.project_code) search.set('project_code', String(value))
      } else {
        search.set(key, String(value))
      }
    }
  }
  const query = search.toString()
  return getJson<HistoryDataResponse>(`/api/v1/history/data${query ? `?${query}` : ''}`)
}
