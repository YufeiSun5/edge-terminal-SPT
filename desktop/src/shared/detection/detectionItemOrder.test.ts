import { describe, expect, it } from 'vitest'
import type { DetectionStandardItemPayload } from '@/shared/api/types'
import {
  extractInletAreaM2,
  normalizeDetectionItemName,
  sanitizeDetectionStandardItems,
  sortDetectionItems,
  standardDetectionItemOrder,
} from './detectionItemOrder'

function item(displayName: string, sortOrder = 1): DetectionStandardItemPayload {
  return {
    var_id: displayName,
    var_name: displayName,
    display_name: displayName,
    check_enabled: true,
    alarm_enabled: true,
    store_enabled: true,
    sort_order: sortOrder,
  }
}

describe('detectionItemOrder', () => {
  it('keeps inlet area out of the default standard item order', () => {
    expect(standardDetectionItemOrder).toContain('吹出口温度')
    expect(standardDetectionItemOrder).not.toContain('吸入口表面积')
  })

  it('normalizes detection item names for matching', () => {
    expect(normalizeDetectionItemName(' 吹 出 口 温 度 ')).toBe('吹出口温度')
  })

  it('sorts known detection items by the legacy Vue order', () => {
    const sorted = sortDetectionItems([
      item('吸入口温度'),
      item('吹出口湿度'),
      item('吹出口温度'),
      item('设备噪音'),
    ])

    expect(sorted.map((entry) => entry.display_name)).toEqual([
      '吹出口温度',
      '吹出口湿度',
      '设备噪音',
      '吸入口温度',
    ])
  })

  it('places unknown items after known items using sort_order then name', () => {
    const sorted = sortDetectionItems([
      item('未知B', 30),
      item('吹出口湿度', 99),
      item('未知A', 20),
      item('吹出口温度', 99),
    ])

    expect(sorted.map((entry) => entry.display_name)).toEqual([
      '吹出口温度',
      '吹出口湿度',
      '未知A',
      '未知B',
    ])
  })

  it('filters inlet area out of detection standard payloads', () => {
    const sanitized = sanitizeDetectionStandardItems([
      { ...item('吸入口表面积'), limit_l: 1.25, limit_h: 1.25 },
      item('吹出口温度'),
    ])

    expect(sanitized).toHaveLength(1)
    expect(sanitized[0].display_name).toBe('吹出口温度')
  })

  it('extracts legacy inlet area value from old standard items', () => {
    expect(
      extractInletAreaM2([
        item('吹出口温度'),
        { ...item('吸入口表面积'), limit_l: 1.25, limit_h: 1.25 },
      ]),
    ).toBe(1.25)
  })
})
