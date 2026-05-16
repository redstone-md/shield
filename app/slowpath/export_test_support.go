package slowpath

// ExportResolvePrompt exposes resolvePrompt for tests in other packages.
func ExportResolvePrompt(e *Engine, provider string) (system string, customs []string, version string, err error) {
	return e.resolvePrompt(provider, "")
}

// ExportVisionPrompt exposes the configured vision prompt for tests in other packages.
func ExportVisionPrompt(e *Engine) string { return e.visionPrompt }
