import { z } from 'zod'
import { deleteJson, getJson, patchJson, postJson } from '@/shared/api/http'
import type {
  AuthMeResponse,
  LoginRequest,
  LoginResponse,
  LogoutResponse,
  SsoTicketResponse,
  SystemUser,
  UserCreatePayload,
  UserPatchPayload,
  UserResetPasswordPayload,
} from '@/shared/api/types'

export const loginRequestSchema = z.object({
  username: z.string().min(1),
  password: z.string().min(1),
})

export async function login(payload: LoginRequest) {
  const body = loginRequestSchema.parse(payload)
  return postJson<LoginResponse, LoginRequest>('/api/v1/auth/login', body)
}

export function getCurrentUser() {
  return getJson<AuthMeResponse>('/api/v1/auth/me')
}

export function logout() {
  return postJson<LogoutResponse, Record<string, never>>('/api/v1/auth/logout', {})
}

export function createSsoTicket() {
  return postJson<SsoTicketResponse, Record<string, never>>('/api/v1/auth/sso-ticket', {})
}

export function getUsers() {
  return getJson<SystemUser[]>('/api/v1/users')
}

export function createUser(payload: UserCreatePayload) {
  return postJson<SystemUser, UserCreatePayload>('/api/v1/users', payload)
}

export function updateUser(userId: number, payload: UserPatchPayload) {
  return patchJson<SystemUser, UserPatchPayload>(`/api/v1/users/${userId}`, payload)
}

export function resetUserPassword(userId: number, payload: UserResetPasswordPayload) {
  return postJson<SystemUser, UserResetPasswordPayload>(`/api/v1/users/${userId}/reset-password`, payload)
}

export function deleteUser(userId: number) {
  return deleteJson<{ status: string }>(`/api/v1/users/${userId}`)
}
