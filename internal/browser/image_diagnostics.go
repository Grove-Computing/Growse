package browser

const maxImageDiagnostics = 64

func appendImageDiagnostic(diagnostics []string, message string) []string {
	if message == "" || len(diagnostics) >= maxImageDiagnostics {
		return diagnostics
	}
	return append(diagnostics, message)
}

func boundedImageDiagnostics(diagnostics []string) []string {
	if len(diagnostics) > maxImageDiagnostics {
		diagnostics = diagnostics[:maxImageDiagnostics]
	}
	return append([]string(nil), diagnostics...)
}
