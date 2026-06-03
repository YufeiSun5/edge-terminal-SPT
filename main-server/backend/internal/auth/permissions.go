package auth

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
