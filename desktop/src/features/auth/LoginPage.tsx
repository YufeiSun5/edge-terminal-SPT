import { useState } from 'react'
import { Button, Form, Input, Segmented, message } from 'antd'
import { useMutation } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Languages, LockKeyhole, LogIn, Server, UserRound } from 'lucide-react'
import { login } from './api'
import { useAuthStore } from './authStore'
import type { LoginRequest } from '@/shared/api/types'
import './login.css'

type LoginLocationState = {
  from?: {
    pathname?: string
    search?: string
  }
}

export function LoginPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const [messageApi, contextHolder] = message.useMessage()
  const setSession = useAuthStore((state) => state.setSession)
  const [form] = Form.useForm<LoginRequest>()
  const [lastUser, setLastUser] = useState('admin')

  const loginMutation = useMutation({
    mutationFn: login,
    onSuccess: (session) => {
      setSession(session)
      const state = location.state as LoginLocationState | null
      const target = state?.from?.pathname ? `${state.from.pathname}${state.from.search ?? ''}` : '/'
      navigate(target, { replace: true })
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('auth.loginFailed'))
    },
  })

  return (
    <div className="login-page">
      {contextHolder}
      <div className="login-ambient" aria-hidden="true">
        <span className="login-orb login-orb-blue" />
        <span className="login-orb login-orb-gold" />
        <span className="login-noise" />
      </div>
      <section className="login-panel">
        <div className="login-brand">
          <span className="brand-mark">
            <Server aria-hidden="true" />
          </span>
          <div>
            <span>{t('app.subtitle')}</span>
            <h1>{t('auth.title')}</h1>
          </div>
        </div>
        <p className="login-copy">{t('auth.subtitle')}</p>
        <Form
          form={form}
          layout="vertical"
          initialValues={{ username: lastUser }}
          onValuesChange={(_, values) => {
            if (values.username) setLastUser(values.username)
          }}
          onFinish={(values) => loginMutation.mutate(values)}
        >
          <Form.Item name="username" label={t('auth.username')} rules={[{ required: true, message: t('auth.usernameRequired') }]}>
            <Input prefix={<UserRound size={15} />} autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label={t('auth.password')} rules={[{ required: true, message: t('auth.passwordRequired') }]}>
            <Input.Password prefix={<LockKeyhole size={15} />} autoComplete="current-password" />
          </Form.Item>
          <Button
            block
            className="login-submit"
            htmlType="submit"
            icon={<LogIn size={16} />}
            loading={loginMutation.isPending}
            type="primary"
          >
            {t('auth.login')}
          </Button>
        </Form>
        <div className="login-footer">
          <span>{t('auth.localCredentialHint', { username: lastUser })}</span>
          <Segmented
            size="small"
            value={i18n.resolvedLanguage ?? 'zh'}
            onChange={(value) => void i18n.changeLanguage(String(value))}
            options={[
              { label: '中文', value: 'zh', icon: <Languages size={14} /> },
              { label: 'EN', value: 'en' },
              { label: '日本語', value: 'ja' },
            ]}
          />
        </div>
      </section>
    </div>
  )
}
