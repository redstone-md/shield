package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type AuditWriter interface {
	Write(ctx context.Context, record AuditRecord) error
}

type AuditRecord struct {
	Event          moderation.IncomingEvent
	Message        *bot.Message
	Response       bot.Response
	Decision       moderation.PolicyDecision
	ActionResult   moderation.ModerationActionResult
	RuleSetVersion int
	ChatID         int64
	SpamUserID     int64
	SlowPath       *slowpath.SlowPathInvocation
}

type enrichedAuditLogger interface {
	SaveAudit(ctx context.Context, record AuditRecord) error
}

type defaultAuditWriter struct {
	spamLogger SpamLogger
	locator    Locator
}

func (w defaultAuditWriter) Write(ctx context.Context, record AuditRecord) error {
	if !record.Response.Send || record.Response.BanInterval <= 0 {
		return nil
	}

	if w.spamLogger != nil {
		if logger, ok := w.spamLogger.(enrichedAuditLogger); ok {
			if err := logger.SaveAudit(ctx, record); err != nil {
				return fmt.Errorf("save enriched audit: %w", err)
			}
		} else {
			w.spamLogger.Save(record.Message, &record.Response)
		}
	}
	if w.locator != nil {
		if err := w.locator.AddSpam(ctx, record.SpamUserID, record.Response.CheckResults); err != nil {
			return fmt.Errorf("add spam to locator: %w", err)
		}
	}
	return nil
}

// MatchedRules returns the rule IDs of spam checks that matched.
// Prefers RuleID (structured identifier) over Name when available.
func MatchedRules(results []spamcheck.Response) []string {
	res := make([]string, 0, len(results))
	for _, result := range results {
		if result.Spam {
			id := result.RuleID
			if id == "" {
				id = result.Name
			}
			res = append(res, id)
		}
	}
	return res
}

// SignalSource returns the primary spam signal source for an audit record.
func SignalSource(results []spamcheck.Response) string {
	rules := MatchedRules(results)
	if len(rules) == 0 {
		return "policy"
	}
	return strings.TrimSpace(rules[0])
}
