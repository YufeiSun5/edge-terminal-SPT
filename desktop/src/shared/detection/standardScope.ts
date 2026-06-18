import type { DetectionStandard, Project } from '@/shared/api/types'

type Translator = (key: string, options?: Record<string, unknown>) => string

type StandardScopeInput = Pick<DetectionStandard, 'project_id' | 'project_code' | 'project_group'>
type ProjectScopeInput = Pick<Project, 'id' | 'project_code' | 'project_group'>

export function detectionStandardScopeLabel(
  standard: StandardScopeInput,
  t: Translator,
  currentProject?: ProjectScopeInput,
) {
  if (standard.project_id) {
    if (currentProject?.id === standard.project_id) {
      return t('settings.standards.scopeCurrentProject')
    }
    return t('settings.standards.scopeProject', {
      project: standard.project_code || standard.project_id,
    })
  }
  if (standard.project_group) {
    return t('settings.standards.scopeProjectGroup', {
      group: standard.project_group,
    })
  }
  return t('settings.standards.scopeGlobal')
}

export function detectionStandardScopeColor(standard: StandardScopeInput) {
  if (standard.project_id) return 'blue'
  if (standard.project_group) return 'purple'
  return 'default'
}
