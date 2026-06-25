import {
  getMainReportNotificationUnreadCount,
  getMainReportNotifications,
  getNotificationUnreadCount,
  getNotifications,
  markAllMainReportNotificationsRead,
  markAllNotificationsRead,
} from '@/features/edge-status/api'
import type {
  MainReportNotification,
  NotificationListParams,
  NotificationListResponse,
  NotificationUnreadCount,
  UserNotification,
} from '@/shared/api/types'
import { env } from '@/shared/config/env'

type Translate = (key: string, options?: Record<string, unknown>) => string

export const alarmNotificationTypes = [
  'alarm.limit.enter',
  'alarm.limit.recover',
  'alarm.limit.level_change',
] as const

export const reportNotificationType = 'report.job'
export const reportVisibleEventTypes = ['started', 'succeeded'] as const

export const notificationTypeOptions = [
  { value: 'alarm.limit.enter', labelKey: 'notifications.types.alarmEnter' },
  { value: 'alarm.limit.recover', labelKey: 'notifications.types.alarmRecover' },
  { value: 'alarm.limit.level_change', labelKey: 'notifications.types.alarmLevelChange' },
  { value: reportNotificationType, labelKey: 'notifications.types.reportJob' },
]

export function isVisibleNotificationType(type?: string) {
  return !type || type === reportNotificationType || alarmNotificationTypes.includes(type as typeof alarmNotificationTypes[number])
}

export function canIncludeReportNotifications(filters: Pick<NotificationListParams, 'type' | 'project_id' | 'keyword' | 'from' | 'to'>) {
  return env.runtimeRole === 'main_server'
    && !filters.project_id
    && !filters.keyword
    && !filters.from
    && !filters.to
    && (!filters.type || filters.type === reportNotificationType)
}

export function isVisibleReportNotification(notification: MainReportNotification) {
  const eventType = String(notification.payload?.event_type ?? '')
  return eventType === 'started' || eventType === 'succeeded'
}

export function reportNotificationToUserNotification(notification: MainReportNotification, t: Translate): UserNotification {
  const payload = notification.payload ?? {}
  const eventType = String(payload.event_type ?? '')
  const taskId = Number(payload.task_id ?? 0)
  const projectId = Number(payload.project_id ?? 0)
  const reportName = String(payload.report_name ?? notification.title ?? '')
  const ready = eventType === 'succeeded'
  return {
    id: -notification.id,
    event_uid: `main-report-${notification.id}`,
    type: reportNotificationType,
    level: ready ? 'success' : 'info',
    target_type: 'all',
    target_id: String(notification.job_id),
    project_id: Number.isFinite(projectId) ? projectId : 0,
    project_code: typeof payload.project_code === 'string' ? payload.project_code : undefined,
    task_id: Number.isFinite(taskId) && taskId > 0 ? taskId : undefined,
    test_no: typeof payload.test_no === 'string' ? payload.test_no : undefined,
    display_name: reportName || (ready ? t('notifications.report.readyTitle') : t('notifications.report.startedTitle')),
    message: ready ? t('notifications.report.readyMessage') : t('notifications.report.startedMessage'),
    payload: { ...payload, report_notification_id: notification.id, job_id: notification.job_id },
    occurred_at: notification.created_at,
    created_at: notification.created_at,
    read_at: notification.read_at,
  }
}

function isUserNotification(item: UserNotification | null | undefined): item is UserNotification {
  return Boolean(item && typeof item === 'object')
}

function sanitizeNotificationList(response: NotificationListResponse): NotificationListResponse {
  return {
    ...response,
    items: response.items.filter(isUserNotification),
  }
}

export function sortNotifications(items: Array<UserNotification | null | undefined>) {
  return items.filter(isUserNotification).sort((left, right) => {
    const leftTime = Date.parse(left.occurred_at || left.created_at || '')
    const rightTime = Date.parse(right.occurred_at || right.created_at || '')
    return (Number.isFinite(rightTime) ? rightTime : 0) - (Number.isFinite(leftTime) ? leftTime : 0)
  })
}

export function emptyNotificationList(limit: number, offset: number): NotificationListResponse {
  return { items: [], total: 0, limit, offset }
}

export async function getVisibleBaseNotifications(params: NotificationListParams = {}) {
  const limit = params.limit ?? 20
  const offset = params.offset ?? 0
  if (params.type) {
    if (!isVisibleNotificationType(params.type) || params.type === reportNotificationType) {
      return emptyNotificationList(limit, offset)
    }
    const response = await getNotifications(params)
    return sanitizeNotificationList(response)
  }

  const fetchLimit = offset + limit
  const pages = await Promise.all(
    alarmNotificationTypes.map((type) => getNotifications({ ...params, type, limit: fetchLimit, offset: 0 })),
  )
  const items = sortNotifications(pages.flatMap((page) => page.items)).slice(offset, offset + limit)
  return {
    items,
    total: pages.reduce((sum, page) => sum + page.total, 0),
    limit,
    offset,
  }
}

export async function getVisibleBaseUnreadCount(params: NotificationListParams = {}): Promise<NotificationUnreadCount> {
  if (params.unread === false) return { unread: 0 }
  if (params.type) {
    if (!isVisibleNotificationType(params.type) || params.type === reportNotificationType) return { unread: 0 }
    return getNotificationUnreadCount(params)
  }

  const counts = await Promise.all(alarmNotificationTypes.map((type) => getNotificationUnreadCount({ ...params, type })))
  return { unread: counts.reduce((sum, item) => sum + item.unread, 0) }
}

export async function markVisibleBaseNotificationsRead(params: NotificationListParams = {}) {
  if (params.type) {
    if (!isVisibleNotificationType(params.type) || params.type === reportNotificationType) return { updated: 0 }
    return markAllNotificationsRead(params)
  }

  const results = await Promise.all(alarmNotificationTypes.map((type) => markAllNotificationsRead({ ...params, type })))
  return { updated: results.reduce((sum, item) => sum + item.updated, 0) }
}

export async function getVisibleReportNotifications(
  params: NotificationListParams & { job_id?: number } = {},
  t: Translate,
) {
  const response = await getMainReportNotifications({ ...params, dedupe: 'job_event', event_type: [...reportVisibleEventTypes] })
  return {
    ...response,
    items: response.items
      .filter(isVisibleReportNotification)
      .map((notification) => reportNotificationToUserNotification(notification, t)),
  }
}

export async function getVisibleReportUnreadCount(params: NotificationListParams & { job_id?: number } = {}) {
  if (params.unread === false) return { unread: 0 }
  return getMainReportNotificationUnreadCount({ ...params, dedupe: 'job_event', unread: true, event_type: [...reportVisibleEventTypes] })
}

export async function markVisibleReportNotificationsRead(params: NotificationListParams & { job_id?: number } = {}) {
  if (params.unread === false) return { updated: 0 }
  return markAllMainReportNotificationsRead({ ...params, unread: true, event_type: [...reportVisibleEventTypes] })
}
