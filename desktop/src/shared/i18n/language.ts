export type AppLanguage = 'zh' | 'en' | 'ja'

export function languageCode(language?: string): AppLanguage {
  const normalized = (language || '').toLowerCase()
  if (normalized.startsWith('en')) return 'en'
  if (normalized.startsWith('ja')) return 'ja'
  return 'zh'
}
