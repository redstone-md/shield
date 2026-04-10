package tgspam

import (
	"context"
	"fmt"
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
	RecentUserMessages []spamcheck.Request
}

func runLLMProviderCheck(ctx context.Context, name, errorPrefix string, retryCount int, msg string, history llmContext,
	send func(context.Context, string) (llmResponse, error),
) (spam bool, cr spamcheck.Response) {
	if retryCount < 1 {
		retryCount = 1
	}

	msg = appendHistoryToLLMMessage(msg, history)

	var resp llmResponse
	var err error
	for i := 0; i < retryCount; i++ {
		if resp, err = send(ctx, msg); err == nil {
			break
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
	}
	if err != nil {
		return false, spamcheck.Response{
			Spam: false, Name: name, Details: fmt.Sprintf("%s error: %v", errorPrefix, err), Error: err,
		}
	}

	return resp.IsSpam, spamcheck.Response{
		Spam: resp.IsSpam, Name: name,
		Details: strings.TrimSuffix(resp.Reason, ".") + ", confidence: " + fmt.Sprintf("%d%%", resp.Confidence),
	}
}

func appendHistoryToLLMMessage(msg string, history llmContext) string {
	if history.RequestContext == "" && len(history.RecentChatMessages) == 0 && len(history.RecentUserMessages) == 0 {
		return msg
	}

	var sb strings.Builder
	if history.RequestContext != "" {
		sb.WriteString("Moderation context:\n")
		sb.WriteString(history.RequestContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString("User message:\n")
	sb.WriteString(msg)

	if len(history.RecentChatMessages) > 0 {
		sb.WriteString("\n\nRecent chat messages:\n")
		for i, h := range history.RecentChatMessages {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(fmt.Sprintf("%q: %q", h.UserName, h.Msg))
		}
	}

	if len(history.RecentUserMessages) > 0 {
		sb.WriteString("\n\nRecent messages from the same user:\n")
		for i, h := range history.RecentUserMessages {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(fmt.Sprintf("%q: %q", h.UserName, h.Msg))
		}
	}

	sb.WriteByte('\n')
	return sb.String()
}
