package rbac

func HasPermission(granted []string, required string) bool {
	for _, code := range granted {
		if code == required {
			return true
		}
	}
	return false
}
