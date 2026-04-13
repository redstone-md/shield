package events

import (
	"context"
	"fmt"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
)

type AuditWriter interface {
	Write(ctx context.Context, record AuditRecord) error
}

type AuditRecord struct {
	Event        moderation.IncomingEvent
	Message      *bot.Message
	Response     bot.Response
	Decision     moderation.PolicyDecision
	ActionResult moderation.ModerationActionResult
	ChatID       int64
	SpamUserID   int64
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
		w.spamLogger.Save(record.Message, &record.Response)
	}
	if w.locator != nil {
		if err := w.locator.AddSpam(ctx, record.SpamUserID, record.Response.CheckResults); err != nil {
			return fmt.Errorf("add spam to locator: %w", err)
		}
	}
	return nil
}
