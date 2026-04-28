package slowpath

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type llmResponse struct {
	IsSpam     bool   `json:"spam"`
	Reason     string `json:"reason"`
	Confidence int    `json:"confidence"`
}

var (
	trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
	spamFieldRe     = regexp.MustCompile(`(?is)"?spam"?\s*:\s*("?(?:true|false|1|0)"?)`)
	reasonFieldRe   = regexp.MustCompile(`(?is)"?reason"?\s*:\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
	confFieldRe     = regexp.MustCompile(`(?is)"?confidence"?\s*:\s*"?([0-9]{1,3})"?`)
	thoughtRe       = regexp.MustCompile(`<thought>(?s).*?</thought>`)
)

func parseLLMOutput(content string) (llmResponse, error) {
	clean := strings.TrimSpace(stripThoughtTags(content))
	if clean == "" {
		return llmResponse{}, fmt.Errorf("empty response")
	}

	candidates := []string{clean}
	if extracted := extractFirstJSON(clean); extracted != "" && extracted != clean {
		candidates = append(candidates, extracted)
	}

	for _, c := range candidates {
		if resp, err := unmarshalLLM(c); err == nil {
			return resp, nil
		}
		if fixed := fixTrailingComma(c); fixed != c {
			if resp, err := unmarshalLLM(fixed); err == nil {
				return resp, nil
			}
		}
	}

	for _, c := range candidates {
		if resp, ok := parseFallback(c); ok {
			return resp, nil
		}
	}

	return llmResponse{}, fmt.Errorf("can't parse LLM response: %s", clean)
}

func stripThoughtTags(content string) string {
	return thoughtRe.ReplaceAllString(content, "")
}

func unmarshalLLM(content string) (llmResponse, error) {
	var r llmResponse
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		return llmResponse{}, fmt.Errorf("unmarshal: %w", err)
	}
	return r, nil
}

func fixTrailingComma(content string) string {
	return trailingCommaRe.ReplaceAllString(content, "$1")
}

func extractFirstJSON(content string) string {
	start := strings.IndexByte(content, '{')
	if start < 0 {
		return ""
	}

	depth, inStr, escaped := 0, false, false
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inStr {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
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

func parseFallback(content string) (llmResponse, bool) {
	sm := spamFieldRe.FindStringSubmatch(content)
	rm := reasonFieldRe.FindStringSubmatch(content)
	cm := confFieldRe.FindStringSubmatch(content)
	if len(sm) < 2 || len(rm) < 2 || len(cm) < 2 {
		return llmResponse{}, false
	}

	isSpam, ok := parseBool(sm[1])
	if !ok {
		return llmResponse{}, false
	}

	reason, err := strconv.Unquote(normalizeQuote(rm[1]))
	if err != nil {
		return llmResponse{}, false
	}

	conf, err := strconv.Atoi(cm[1])
	if err != nil {
		return llmResponse{}, false
	}

	return llmResponse{IsSpam: isSpam, Reason: reason, Confidence: conf}, true
}

func parseBool(raw string) (bool, bool) {
	v := strings.Trim(strings.ToLower(raw), `" `)
	switch v {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func normalizeQuote(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return `"` + strings.ReplaceAll(raw[1:len(raw)-1], `"`, `\"`) + `"`
	}
	return raw
}
