package auth

import "strings"

const (
	RoleGuest     = "guest"
	RoleAdmin     = "admin"
	RoleDeveloper = "developer"
)

const (
	PermViewRealtime    = "view_realtime"
	PermViewHistory     = "view_history"
	PermStartDetection  = "start_detection"
	PermStopDetection   = "stop_detection"
	PermManageVariables = "manage_variables"
	PermManageGateways  = "manage_gateways"
	PermKIOWrite        = "kio_write"
	PermManageUsers     = "manage_users"
	PermSystemSettings  = "system_settings"
	PermSSOHandoff      = "sso_handoff"
)

const (
	ScopeServiceRealtimeRead = "service_realtime_read"
	ScopeServiceControlCall  = "service_control_call"
	ScopeServiceSSOVerify    = "service_sso_verify"
)

var rolePermissions = map[string][]string{
	RoleGuest: {
		PermViewRealtime,
		PermViewHistory,
	},
	RoleAdmin: {
		PermViewRealtime,
		PermViewHistory,
		PermStartDetection,
		PermStopDetection,
		PermManageVariables,
		PermManageGateways,
		PermKIOWrite,
		PermManageUsers,
		PermSystemSettings,
		PermSSOHandoff,
	},
	RoleDeveloper: {
		PermViewRealtime,
		PermViewHistory,
		PermManageVariables,
		PermManageGateways,
		PermSystemSettings,
		PermSSOHandoff,
	},
}

func ValidRole(role string) bool {
	_, ok := rolePermissions[role]
	return ok
}

func PermissionsForRole(role string) []string {
	permissions := rolePermissions[role]
	result := make([]string, len(permissions))
	copy(result, permissions)
	return result
}

func RoleHasPermission(role string, permission string) bool {
	for _, item := range rolePermissions[role] {
		if item == permission {
			return true
		}
	}
	return false
}

func NormalizeScopes(scopes []string) string {
	seen := make(map[string]bool, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		normalized = append(normalized, scope)
	}
	return strings.Join(normalized, ",")
}

func ParseScopes(raw string) []string {
	parts := strings.Split(raw, ",")
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			scopes = append(scopes, part)
		}
	}
	return scopes
}

func HasScope(raw string, required string) bool {
	for _, scope := range ParseScopes(raw) {
		if scope == required {
			return true
		}
	}
	return false
}
