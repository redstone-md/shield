package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
)

type PolicyEngine interface {
	Decide(ctx context.Context, req PolicyRequest) (PolicyOutcome, error)
}

type PolicyRequest struct {
	Event         moderation.IncomingEvent
	Response      bot.Response
	Message       *bot.Message
	SpamUserID    int64
	StrikeCount   int
	UseEscalation bool
	SoftBanMode   bool
	Moderation    ModerationConfig
	IsSuperUser   bool
}

type PolicyOutcome struct {
	Decision moderation.PolicyDecision
	Duration time.Duration
	Restrict bool
}

type defaultPolicyEngine struct{}

func (e defaultPolicyEngine) Decide(_ context.Context, req PolicyRequest) (PolicyOutcome, error) {
	outcome := PolicyOutcome{
		Decision: moderation.PolicyDecision{
			EventID:       req.Event.EventID,
			CorrelationID: req.Event.CorrelationID,
			Action:        moderation.ActionAllow,
			Reason:        "message allowed",
			DecidedAt:     time.Now().UTC(),
		},
	}

	if !req.Response.Send {
		return outcome, nil
	}

	outcome.Decision.Score = spamScore(req.Response)
	outcome.Decision.Reason = policyReason(req.Response)

	if req.IsSuperUser {
		outcome.Decision.Action = moderation.ActionAllow
		outcome.Decision.Reason = "superuser exempt from automated sanctions"
		return outcome, nil
	}

	if req.Response.BanInterval <= 0 {
		outcome.Decision.Action = moderation.ActionDelete
		if outcome.Decision.Reason == "" {
			outcome.Decision.Reason = "spam detected"
		}
		return outcome, nil
	}

	duration, restrict := req.Response.BanInterval, req.SoftBanMode
	if req.UseEscalation {
		duration, restrict = spamPenalty(req.StrikeCount, req.SoftBanMode, req.Moderation)
	}
	outcome.Duration = duration
	outcome.Restrict = restrict
	if restrict {
		outcome.Decision.Action = moderation.ActionRestrict
		outcome.Decision.Reason = fmt.Sprintf("restrict for %s", duration)
		return outcome, nil
	}

	outcome.Decision.Action = moderation.ActionBan
	outcome.Decision.Reason = "permanent ban"
	return outcome, nil
}

func spamScore(resp bot.Response) float64 {
	if len(resp.CheckResults) == 0 {
		return 1
	}
	total := 0.0
	hasWeighted := false
	for _, result := range resp.CheckResults {
		if !result.Spam {
			continue
		}
		w := result.Weight
		s := result.Score
		if w > 0 || s > 0 {
			hasWeighted = true
			if w == 0 {
				w = 1.0
			}
			if s == 0 {
				s = 1.0
			}
			total += w * s
		} else {
			total++
		}
	}
	if total == 0 {
		return 1
	}
	if hasWeighted {
		return total
	}
	return total
}

func policyReason(resp bot.Response) string {
	for _, result := range resp.CheckResults {
		if result.Spam && strings.TrimSpace(result.Details) != "" {
			return result.Details
		}
	}
	return "spam detected"
}
