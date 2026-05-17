package events

import (
	"context"
	"fmt"
	"log"
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

// IncidentCreator creates an incident record from a detected spam event and returns the new incident ID.
// implementations must be idempotent on idempotencyKey.
type IncidentCreator interface {
	CreateIncident(
		ctx context.Context,
		idempotencyKey string,
		chatID int64,
		ruleSetVersion int,
		spamUserID int64,
		spamUserName string,
		messageText string,
		checks []spamcheck.Response,
		slowPath *slowpath.SlowPathInvocation,
	) (int64, error)
}

type defaultAuditWriter struct {
	spamLogger      SpamLogger
	locator         Locator
	incidentCreator IncidentCreator
}

func NewDefaultAuditWriter(spamLogger SpamLogger, locator Locator, incidentCreator IncidentCreator) AuditWriter {
	return defaultAuditWriter{spamLogger: spamLogger, locator: locator, incidentCreator: incidentCreator}
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
	if w.incidentCreator != nil {
		userName := ""
		if record.Message != nil && record.Message.From.ID != 0 {
			userName = record.Message.From.DisplayName
		}
		msgText := ""
		if record.Message != nil {
			msgText = record.Message.Text
		}
		if _, err := w.incidentCreator.CreateIncident(ctx,
			record.Event.IdempotencyKey, record.ChatID, record.RuleSetVersion,
			record.SpamUserID, userName, msgText,
			record.Response.CheckResults, record.SlowPath,
		); err != nil {
			log.Printf("[WARN] incident creation failed: %v", err)
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
