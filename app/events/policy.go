package events

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/policy"
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
	WarnStrikes   int
}

type PolicyOutcome struct {
	Decision moderation.PolicyDecision
	Duration time.Duration
	Restrict bool
}

type defaultPolicyEngine struct {
	eng *policy.Engine
}

func newProfilePolicyEngine(profileName string) defaultPolicyEngine {
	if profileName == "" {
		return defaultPolicyEngine{}
	}
	return defaultPolicyEngine{eng: policy.NewEngine(policy.ResolveProfile(profileName))}
}

func (e defaultPolicyEngine) Decide(_ context.Context, req PolicyRequest) (PolicyOutcome, error) {
	if e.eng != nil {
		return e.decideWithEngine(req)
	}
	return e.decideLegacy(req)
}

func (e defaultPolicyEngine) decideWithEngine(req PolicyRequest) (PolicyOutcome, error) {
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

	result := e.eng.Decide(policy.PolicyInput{
		Signals:      req.Response.CheckResults,
		StrikeCount:  req.StrikeCount,
		IsSuperUser:  req.IsSuperUser,
		SoftBanMode:  req.SoftBanMode,
		FirstStrike:  req.Moderation.FirstStrike,
		SecondStrike: req.Moderation.SecondStrike,
		WarnStrikes:  req.Moderation.WarnStrikes,
	})

	outcome.Decision.Score = spamScore(req.Response)
	outcome.Decision.Action = result.Action
	outcome.Decision.Reason = result.Reason
	outcome.Decision.PolicyVersion = result.Explanation.PolicyVersion
	outcome.Decision.ProfileName = result.Explanation.ProfileName
	outcome.Duration = result.Duration
	outcome.Restrict = result.Restrict

	if result.Shadow != nil {
		log.Printf("[INFO] shadow decision: action=%s reason=%q profile=%s",
			result.Shadow.Action, result.Shadow.Reason, result.Shadow.Explanation.ProfileName)
	}

	return outcome, nil
}

func (e defaultPolicyEngine) decideLegacy(req PolicyRequest) (PolicyOutcome, error) {
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
	var warn bool
	if req.UseEscalation {
		duration, restrict, warn = spamPenalty(req.StrikeCount, req.SoftBanMode, req.Moderation)
	}
	if warn {
		outcome.Decision.Action = moderation.ActionWarn
		outcome.Decision.Reason = "warning strike"
		return outcome, nil
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
