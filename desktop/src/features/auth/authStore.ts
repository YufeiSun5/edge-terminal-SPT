import { create } from 'zustand'
import { clearAccessToken, setAccessToken } from '@/shared/auth/tokenStore'
import { refreshAuthSession } from './api'
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
  tokenIssuedAt: number | null
  tokenExpiresInSec: number | null
  setSession: (session: LoginResponse) => void
  setPrincipal: (principal: AuthMeResponse) => void
  clearSession: () => void
  refreshSession: () => Promise<void>
  hasPermission: (permission: string) => boolean
  hasAnyPermission: (permissions: string[]) => boolean
}

let refreshTimer: number | undefined
let refreshInFlight: Promise<void> | undefined
const refreshLeadMs = 5 * 60 * 1000

function clearRefreshTimer() {
  if (refreshTimer === undefined || typeof window === 'undefined') return
  window.clearTimeout(refreshTimer)
  refreshTimer = undefined
}

function scheduleRefresh(expiresInSec: number | undefined, refresh: () => Promise<void>) {
  clearRefreshTimer()
  if (!expiresInSec || typeof window === 'undefined') return
  const ttlMs = expiresInSec * 1000
  const delayMs = Math.max(1000, ttlMs > refreshLeadMs ? ttlMs - refreshLeadMs : Math.floor(ttlMs * 0.8))
  refreshTimer = window.setTimeout(() => {
    void refresh()
  }, delayMs)
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  permissions: [],
  authenticated: false,
  tokenIssuedAt: null,
  tokenExpiresInSec: null,
  setSession: (session) => {
    setAccessToken(session.access_token)
    set({
      user: session.user,
      permissions: session.permissions ?? [],
      authenticated: true,
      tokenIssuedAt: Date.now(),
      tokenExpiresInSec: session.expires_in ?? null,
    })
    scheduleRefresh(session.expires_in, get().refreshSession)
  },
  setPrincipal: (principal) => {
    set({ user: principal.user, permissions: principal.permissions, authenticated: true })
  },
  clearSession: () => {
    clearRefreshTimer()
    clearAccessToken()
    set({ user: null, permissions: [], authenticated: false, tokenIssuedAt: null, tokenExpiresInSec: null })
  },
  refreshSession: async () => {
    if (refreshInFlight) return refreshInFlight
    refreshInFlight = refreshAuthSession()
      .then((session) => {
        get().setSession(session)
      })
      .catch((error) => {
        get().clearSession()
        throw error
      })
      .finally(() => {
        refreshInFlight = undefined
      })
    return refreshInFlight
  },
  hasPermission: (permission) => get().permissions.includes(permission),
  hasAnyPermission: (permissions) => permissions.some((permission) => get().permissions.includes(permission)),
}))
