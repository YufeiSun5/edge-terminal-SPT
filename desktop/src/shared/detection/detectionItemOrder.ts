import type { DetectionStandard, DetectionStandardItem, DetectionStandardItemPayload } from '@/shared/api/types'

export const inletAreaDisplayName = '吸入口表面积'

export const legacyDetectionConfigOrder = [
  inletAreaDisplayName,
  '吹出口温度',
  '吹出口湿度',
  '吸入风量',
  '设备噪音',
  '震动位移',
  '吸入口温度',
  '吸入口湿度',
  '压缩机吸入管温度',
  '压缩机吐出口温度',
  '蒸发器出口温度',
  '冷凝器出口温度',
  '膨胀阀出口温度',
  '冷却水入口温度',
  '冷却水出口温度',
  '加湿器给水口温度',
  '再热器出口温度',
  '干燥过滤器入口温度',
  '干燥过滤器出口温度',
] as const

export const standardDetectionItemOrder = legacyDetectionConfigOrder.filter(
  (name) => name !== inletAreaDisplayName,
)

type DetectionItemLike = {
  display_name?: string
  var_name?: string
  sort_order?: number
}

export function normalizeDetectionItemName(value?: string) {
  return (value ?? '').replace(/\s+/g, '').toLowerCase()
}

function itemNames(item: DetectionItemLike) {
  return [item.display_name, item.var_name]
    .map(normalizeDetectionItemName)
    .filter(Boolean)
}

export function isInletAreaItem(item: DetectionItemLike) {
  return itemNames(item).some((name) => name.includes(normalizeDetectionItemName(inletAreaDisplayName)))
}

function orderIndex(item: DetectionItemLike) {
  const names = itemNames(item)
  for (const [index, orderedName] of standardDetectionItemOrder.entries()) {
    const normalizedOrderedName = normalizeDetectionItemName(orderedName)
    if (names.some((name) => name.includes(normalizedOrderedName) || normalizedOrderedName.includes(name))) {
      return index
    }
  }
  return Number.POSITIVE_INFINITY
}

function itemTitle(item: DetectionItemLike) {
  return item.display_name || item.var_name || ''
}

export function sortDetectionItems<T extends DetectionItemLike>(items: T[]) {
  return [...items].sort((left, right) => {
    const leftIndex = orderIndex(left)
    const rightIndex = orderIndex(right)
    if (leftIndex !== rightIndex) return leftIndex - rightIndex
    if (leftIndex === Number.POSITIVE_INFINITY && rightIndex === Number.POSITIVE_INFINITY) {
      const leftOrder = Number.isFinite(left.sort_order) ? Number(left.sort_order) : Number.POSITIVE_INFINITY
      const rightOrder = Number.isFinite(right.sort_order) ? Number(right.sort_order) : Number.POSITIVE_INFINITY
      if (leftOrder !== rightOrder) return leftOrder - rightOrder
    }
    return itemTitle(left).localeCompare(itemTitle(right), 'zh-CN')
  })
}

export function normalizeDetectionStandardItems(items: DetectionStandard['items'] = []): DetectionStandardItemPayload[] {
  const payloadItems: DetectionStandardItemPayload[] =
    items
      .filter((item) => !isInletAreaItem(item))
      .map((item) => ({
        var_id: item.var_id_text ?? item.var_id,
        var_name: item.var_name,
        display_name: item.display_name,
        display_name_en: item.display_name_en,
        display_name_ja: item.display_name_ja,
        check_enabled: item.check_enabled,
        alarm_enabled: item.alarm_enabled,
        store_enabled: item.store_enabled,
        check_cycle_ms: item.check_cycle_ms,
        check_on_start: item.check_on_start,
        required: item.required,
        check_method: item.check_method,
        target_value: item.target_value,
        limit_ll: item.limit_ll ?? null,
        limit_l: item.limit_l ?? null,
        limit_h: item.limit_h ?? null,
        limit_hh: item.limit_hh ?? null,
        limit_deadband: item.limit_deadband,
        violation_hold_ms: item.violation_hold_ms,
        recover_hold_ms: item.recover_hold_ms,
        quality_policy: item.quality_policy,
        unit: item.unit,
        decimal_places: item.decimal_places,
        sort_order: item.sort_order,
      }))
  return sortDetectionItems(payloadItems).map((item, index) => ({ ...item, sort_order: index + 1 }))
}

export function extractInletAreaM2(items: Array<DetectionStandardItem | DetectionStandardItemPayload> = []) {
  const areaItem = items.find((item) => isInletAreaItem(item))
  if (!areaItem) return undefined
  const value = areaItem.limit_l ?? areaItem.limit_h ?? areaItem.limit_ll ?? areaItem.limit_hh
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

export function sanitizeDetectionStandardItems(items: DetectionStandardItemPayload[]) {
  return sortDetectionItems(items.filter((item) => !isInletAreaItem(item))).map((item, index) => ({
    ...item,
    sort_order: index + 1,
  }))
}
