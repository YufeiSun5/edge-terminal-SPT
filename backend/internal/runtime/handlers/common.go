package handlers

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func setStringUpdate(updates map[string]interface{}, column string, value *string) {
	if value != nil {
		updates[column] = *value
	}
}
