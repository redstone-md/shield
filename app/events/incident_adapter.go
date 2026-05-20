package events

import (
	"context"

	"github.com/redstone-md/shield/app/audit"
	"github.com/redstone-md/shield/app/slowpath"
	"github.com/redstone-md/shield/lib/spamcheck"
)

type incidentAdapter struct {
	svc *audit.Service
}

func (a *incidentAdapter) CreateIncident(
	ctx context.Context,
	idempotencyKey string,
	chatID int64,
	ruleSetVersion int,
	spamUserID int64,
	spamUserName string,
	messageText string,
	checks []spamcheck.Response,
	slow *slowpath.SlowPathInvocation,
) (int64, error) {
	data := audit.AuditEventData{
		IdempotencyKey: idempotencyKey,
		ChatID:         chatID,
		RuleSetVersion: ruleSetVersion,
		SpamUserID:     spamUserID,
		SpamUserName:   spamUserName,
		MessageText:    messageText,
	}
	for _, c := range checks {
		data.CheckResults = append(data.CheckResults, audit.SpamCheckResult{
			Name:    c.Name,
			Spam:    c.Spam,
			Details: c.Details,
		})
	}
	if slow != nil {
		data.SlowPathInvoked = true
		data.SlowProvider = slow.Provider
		data.SlowPromptVer = slow.PromptVersion
	}
	return a.svc.CreateIncident(ctx, data)
}

func NewIncidentAdapter(svc *audit.Service) IncidentCreator {
	return &incidentAdapter{svc: svc}
}
