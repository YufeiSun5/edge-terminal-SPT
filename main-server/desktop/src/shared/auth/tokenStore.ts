let accessToken: string | null = null
const unauthorizedListeners = new Set<() => void>()

export function getAccessToken() {
  return accessToken
}

export function hasAccessToken() {
  return Boolean(accessToken)
}

export function setAccessToken(token: string | null) {
  accessToken = token
}

export function clearAccessToken() {
  accessToken = null
}

export function onUnauthorized(listener: () => void) {
  unauthorizedListeners.add(listener)
  return () => unauthorizedListeners.delete(listener)
}

export function notifyUnauthorized() {
  clearAccessToken()
  unauthorizedListeners.forEach((listener) => listener())
}
