import { App as AntApp, ConfigProvider } from 'antd'
import enUS from 'antd/locale/en_US'
import jaJP from 'antd/locale/ja_JP'
import zhCN from 'antd/locale/zh_CN'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { useTranslation } from 'react-i18next'
import { languageCode } from '@/shared/i18n/language'
import { queryClient } from './queryClient'
import { router } from './router'

const antdLocales = {
  zh: zhCN,
  en: enUS,
  ja: jaJP,
}

export function AppProviders() {
  const { i18n } = useTranslation()
  const language = languageCode(i18n.resolvedLanguage)

  return (
    <ConfigProvider
      locale={antdLocales[language]}
      theme={{
        token: {
          colorPrimary: '#1f5f8b',
          borderRadius: 6,
          fontFamily:
            'Inter, "Segoe UI", "Microsoft YaHei", "Hiragino Sans GB", Arial, sans-serif',
        },
      }}
    >
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  )
}
