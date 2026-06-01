import { createHashRouter } from 'react-router'
import { ShellLayout } from './ShellLayout'
import { LoginPage } from '@/features/auth/LoginPage'
import { ProtectedRoute } from '@/features/auth/ProtectedRoute'
import { EdgeStatusPage } from '@/features/edge-status/EdgeStatusPage'
import { StationOperationPage } from '@/features/station-operation/StationOperationPage'
import { HistoryQueryPage } from '@/features/history-query/HistoryQueryPage'
import { ReportsPage } from '@/features/reports/ReportsPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { DetectionConfigPage } from '@/features/detection-config/DetectionConfigPage'
import { TaskFlowsPage } from '@/features/task-flows/TaskFlowsPage'
import { ModelCockpitPage } from '@/features/model-cockpit/ModelCockpitPage'
import { NotificationCenterPage } from '@/features/notifications/NotificationCenterPage'
import { AlarmCenterPage } from '@/features/alarms/AlarmCenterPage'

export const router = createHashRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: <ProtectedRoute />,
    children: [
      {
        element: <ShellLayout />,
        children: [
          {
            index: true,
            element: <StationOperationPage />,
          },
          {
            path: 'station',
            element: <StationOperationPage />,
          },
          {
            path: 'debug',
            element: <ProtectedRoute permissions={['system_settings']} />,
            children: [
              {
                index: true,
                element: <EdgeStatusPage />,
              },
            ],
          },
          {
            path: 'history',
            element: <ProtectedRoute permissions={['view_history']} />,
            children: [
              {
                index: true,
                element: <HistoryQueryPage />,
              },
            ],
          },
          {
            path: 'reports',
            element: <ProtectedRoute permissions={['view_history']} />,
            children: [
              {
                index: true,
                element: <ReportsPage />,
              },
            ],
          },
          {
            path: 'notifications',
            element: <NotificationCenterPage />,
          },
          {
            path: 'alarms',
            element: <ProtectedRoute permissions={['view_realtime']} />,
            children: [
              {
                index: true,
                element: <AlarmCenterPage />,
              },
            ],
          },
          {
            path: 'detection-config',
            element: <ProtectedRoute permissions={['manage_variables']} />,
            children: [
              {
                index: true,
                element: <DetectionConfigPage />,
              },
            ],
          },
          {
            path: 'model-cockpit',
            element: <ProtectedRoute permissions={['view_realtime']} />,
            children: [
              {
                index: true,
                element: <ModelCockpitPage />,
              },
            ],
          },
          {
            path: 'tasks',
            element: <ProtectedRoute permissions={['system_settings']} />,
            children: [
              {
                index: true,
                element: <TaskFlowsPage />,
              },
            ],
          },
          {
            path: 'settings',
            element: <ProtectedRoute permissions={['manage_variables', 'manage_gateways', 'system_settings', 'manage_users']} />,
            children: [
              {
                index: true,
                element: <SettingsPage />,
              },
            ],
          },
        ],
      },
    ],
  },
])
