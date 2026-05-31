package events

import (
	"strings"

	"github.com/redstone-md/shield/lib/spamcheck"
)

func slowpathReason(checks []spamcheck.Response) string {
	for _, check := range checks {
		if check.Spam && check.Name == "slowpath" {
			return strings.TrimSpace(check.Details)
		}
	}
	for _, check := range checks {
		if !check.Spam || strings.TrimSpace(check.Details) == "" {
			continue
		}
		if isLLMProviderCheck(check.Name) || strings.Contains(check.Details, "confidence:") {
			return strings.TrimSpace(check.Details)
		}
	}
	return ""
}

func isLLMProviderCheck(name string) bool {
	switch strings.TrimSpace(name) {
	case "openai", "gemini", "vision":
		return true
	default:
		return false
	}
}
