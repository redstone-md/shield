package events

import (
	"context"
	"strings"
)

func (a *admin) spamInfoForCallback(ctx context.Context, userID int64, messageText string) string {
	spamInfo := []string{}
	if a.locator != nil {
		info, found := a.locator.Spam(ctx, userID)
		if found {
			for _, check := range info.Checks {
				spamInfo = append(spamInfo, "- "+escapeMarkDownV1Text(check.String()))
			}
		}
	}
	if len(spamInfo) > 0 {
		return strings.Join(spamInfo, "\n")
	}
	if existing := existingSpamInfoFromMessage(messageText); existing != "" {
		return escapeMarkDownV1Text(existing)
	}
	return "**can't get spam info**"
}

func existingSpamInfoFromMessage(messageText string) string {
	lines := strings.Split(messageText, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "spam detection results") ||
			strings.HasPrefix(trimmed, "**spam detection results**") {
			start = i + 1
			break
		}
	}
	if start < 0 || start >= len(lines) {
		return ""
	}

	result := make([]string, 0, len(lines)-start)
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(result) > 0 {
				break
			}
			continue
		}
		if strings.Contains(trimmed, "can't get spam info") || strings.Contains(trimmed, "не удалось получить диагностику") {
			continue
		}
		if isCallbackStatusLine(trimmed) {
			break
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func isCallbackStatusLine(line string) bool {
	return strings.HasPrefix(line, "разбанено администратором") ||
		strings.HasPrefix(line, "забанено администратором") ||
		strings.HasPrefix(line, "пользователь забанен") ||
		strings.HasPrefix(line, "_unbanned by moderator")
}
