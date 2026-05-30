import { create } from 'zustand'
import { clearAccessToken, setAccessToken } from '@/shared/auth/tokenStore'
import type { AuthMeResponse, LoginResponse } from '@/shared/api/types'

type AuthUser = {
  id?: number
  username: string
  role: string
  display_name?: string
  permissions_version?: number
}

type AuthState = {
  user: AuthUser | null
  permissions: string[]
  authenticated: boolean
  setSession: (session: LoginResponse) => void
  setPrincipal: (principal: AuthMeResponse) => void
  clearSession: () => void
  hasPermission: (permission: string) => boolean
  hasAnyPermission: (permissions: string[]) => boolean
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  permissions: [],
  authenticated: false,
  setSession: (session) => {
    setAccessToken(session.access_token)
    set({ user: session.user, permissions: session.permissions ?? [], authenticated: true })
  },
  setPrincipal: (principal) => {
    set({ user: principal.user, permissions: principal.permissions, authenticated: true })
  },
  clearSession: () => {
    clearAccessToken()
    set({ user: null, permissions: [], authenticated: false })
  },
  hasPermission: (permission) => get().permissions.includes(permission),
  hasAnyPermission: (permissions) => permissions.some((permission) => get().permissions.includes(permission)),
}))
