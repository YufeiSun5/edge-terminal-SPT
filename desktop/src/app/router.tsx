import { Navigate, createHashRouter } from 'react-router'
import { ShellLayout } from './ShellLayout'
import { LoginPage } from '@/features/auth/LoginPage'
import { ProtectedRoute } from '@/features/auth/ProtectedRoute'
import { SsoHandoffPage } from '@/features/auth/SsoHandoffPage'
import { EdgeStatusPage } from '@/features/edge-status/EdgeStatusPage'
import { StationOperationPage } from '@/features/station-operation/StationOperationPage'
import { HistoryQueryPage } from '@/features/history-query/HistoryQueryPage'
import { TaskDetailPage } from '@/features/history-query/TaskDetailPage'
import { LatestHistoryRedirectPage } from '@/features/history-query/LatestHistoryRedirectPage'
import { DetectionPlansPage } from '@/features/history-query/DetectionPlansPage'
import { ReportTemplateManagementPage } from '@/features/reports/ReportTemplateManagementPage'
import { ReportPlanImportPage } from '@/features/reports/ReportPlanImportPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { DetectionConfigPage } from '@/features/detection-config/DetectionConfigPage'
import { VariableConfigPage } from '@/features/variable-config/VariableConfigPage'
import { TaskFlowsPage } from '@/features/task-flows/TaskFlowsPage'
import { ModelCockpitPage } from '@/features/model-cockpit/ModelCockpitPage'
import { ModelStageDebugPage } from '@/features/model-cockpit/ModelStageDebugPage'
import { NotificationCenterPage } from '@/features/notifications/NotificationCenterPage'
import { AlarmCenterPage } from '@/features/alarms/AlarmCenterPage'

export const router = createHashRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/sso/edge',
    element: <SsoHandoffPage />,
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
              {
                path: 'model-cockpit',
                element: <ModelStageDebugPage />,
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
              {
                path: 'list',
                element: <LatestHistoryRedirectPage />,
              },
              {
                path: 'plans',
                element: <DetectionPlansPage />,
              },
              {
                path: 'runs/:taskId',
                element: <TaskDetailPage />,
              },
            ],
          },
          {
            path: 'report-settings',
            element: <ProtectedRoute permissions={['system_settings']} />,
            children: [
              {
                index: true,
                element: <ReportTemplateManagementPage />,
              },
              {
                path: 'templates',
                element: <ReportTemplateManagementPage />,
              },
              {
                path: 'plan-imports',
                element: <ReportPlanImportPage />,
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
            path: 'variables',
            element: <ProtectedRoute permissions={['manage_variables']} />,
            children: [
              {
                index: true,
                element: <VariableConfigPage />,
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
              {
                path: 'debug',
                element: <Navigate to="/debug/model-cockpit" replace />,
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
