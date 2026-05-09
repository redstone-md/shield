package tgspam

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

type llmResponse struct {
	IsSpam     bool   `json:"spam"`
	Reason     string `json:"reason"`
	Confidence int    `json:"confidence"`
}

type llmContext struct {
	RequestContext     string
	RecentChatMessages []spamcheck.Request
}

const maxLLMProviderRetries = 20

var (
	trailingCommaRegex = regexp.MustCompile(`,\s*([}\]])`)
	spamFieldRegex     = regexp.MustCompile(`(?is)"?spam"?\s*:\s*("?(?:true|false|1|0)"?)`)
	reasonFieldRegex   = regexp.MustCompile(`(?is)"?reason"?\s*:\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
	confFieldRegex     = regexp.MustCompile(`(?is)"?confidence"?\s*:\s*"?([0-9]{1,3})"?`)
)

type llmCheckParams struct {
	Name        string
	ErrorPrefix string
	RetryCount  int
	Msg         string
	History     llmContext
	Send        func(context.Context, string) (llmResponse, error)
}

func runLLMProviderCheck(ctx context.Context, p llmCheckParams) (spam bool, cr spamcheck.Response) {
	retryCount := p.RetryCount
	if retryCount < 1 {
		retryCount = 1
	}
	if retryCount > maxLLMProviderRetries {
		retryCount = maxLLMProviderRetries
	}

	msg := appendHistoryToLLMMessage(p.Msg, p.History)

	var resp llmResponse
	var err error
	for i := 0; i < retryCount; i++ {
		if resp, err = p.Send(ctx, msg); err == nil {
			break
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
	}
	if err != nil {
		return false, spamcheck.Response{
			Spam: false, Name: p.Name, Details: fmt.Sprintf("%s error: %v", p.ErrorPrefix, err), Error: err,
		}
	}

	return resp.IsSpam, spamcheck.Response{
		Spam: resp.IsSpam, Name: p.Name,
		Details: strings.TrimSuffix(resp.Reason, ".") + ", confidence: " + fmt.Sprintf("%d%%", resp.Confidence),
	}
}

func appendHistoryToLLMMessage(msg string, history llmContext) string {
	if history.RequestContext == "" && len(history.RecentChatMessages) == 0 {
		return msg
	}

	var sb strings.Builder
	if history.RequestContext != "" {
		sb.WriteString("Moderation context:\n")
		sb.WriteString(history.RequestContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Current checked user message:\n")
	sb.WriteString(msg)

	if len(history.RecentChatMessages) > 0 {
		sb.WriteString("\n\nRecent chat messages:\n")
		for i, h := range history.RecentChatMessages {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(fmt.Sprintf("%q: %q", h.UserName, llmHistoryMessage(h)))
		}
	}

	sb.WriteByte('\n')
	return sb.String()
}

func llmHistoryMessage(req spamcheck.Request) string {
	if req.HistoryMsg != "" {
		return req.HistoryMsg
	}
	return req.Msg
}

func parseLLMResponse(content string) (llmResponse, error) {
	clean := strings.TrimSpace(stripThoughtTags(content))
	if clean == "" {
		return llmResponse{}, fmt.Errorf("empty response")
	}

	candidates := []string{clean}
	if extracted := extractFirstJSONObject(clean); extracted != "" && extracted != clean {
		candidates = append(candidates, extracted)
	}

	for _, candidate := range candidates {
		if resp, err := unmarshalLLMResponse(candidate); err == nil {
			return resp, nil
		}
		if sanitized := sanitizeBrokenJSON(candidate); sanitized != candidate {
			if resp, err := unmarshalLLMResponse(sanitized); err == nil {
				return resp, nil
			}
		}
	}

	for _, candidate := range candidates {
		if resp, ok := parseLLMResponseFallback(candidate); ok {
			return resp, nil
		}
	}

	return llmResponse{}, fmt.Errorf("can't unmarshal response: %s", clean)
}

func unmarshalLLMResponse(content string) (llmResponse, error) {
	var response llmResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return llmResponse{}, fmt.Errorf("can't unmarshal response: %s - %w", content, err)
	}
	return response, nil
}

func sanitizeBrokenJSON(content string) string {
	return trailingCommaRegex.ReplaceAllString(content, "$1")
}

func extractFirstJSONObject(content string) string {
	start := strings.IndexByte(content, '{')
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return ""
}

func parseLLMResponseFallback(content string) (llmResponse, bool) {
	spamMatch := spamFieldRegex.FindStringSubmatch(content)
	reasonMatch := reasonFieldRegex.FindStringSubmatch(content)
	confMatch := confFieldRegex.FindStringSubmatch(content)
	if len(spamMatch) < 2 || len(reasonMatch) < 2 || len(confMatch) < 2 {
		return llmResponse{}, false
	}

	isSpam, ok := parseFallbackBool(spamMatch[1])
	if !ok {
		return llmResponse{}, false
	}

	reason, err := strconv.Unquote(normalizeQuotedString(reasonMatch[1]))
	if err != nil {
		return llmResponse{}, false
	}

	confidence, err := strconv.Atoi(confMatch[1])
	if err != nil {
		return llmResponse{}, false
	}

	return llmResponse{IsSpam: isSpam, Reason: reason, Confidence: confidence}, true
}

func parseFallbackBool(raw string) (bool, bool) {
	val := strings.Trim(strings.ToLower(raw), `" `)
	switch val {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func normalizeQuotedString(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return `"` + strings.ReplaceAll(raw[1:len(raw)-1], `"`, `\"`) + `"`
	}
	return raw
}
