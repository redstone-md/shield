package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type PolicyInput struct {
	Signals      []spamcheck.Response
	StrikeCount  int
	IsSuperUser  bool
	SoftBanMode  bool
	FirstStrike  time.Duration
	SecondStrike time.Duration
	WarnStrikes  int
	DryRun       bool
}

type PolicyResult struct {
	Action      moderation.Action
	Duration    time.Duration
	Restrict    bool
	DryRun      bool
	Reason      string
	Explanation DecisionExplanation
	Shadow      *PolicyResult
}

type DecisionExplanation struct {
	ProfileName   string
	RiskType      RiskType
	BaseAction    ActionLevel
	FinalAction   ActionLevel
	StrikeCount   int
	MatchedRules  []string
	PolicyVersion int
}

type Engine struct {
	Profile       PolicyProfile
	ShadowProfile *PolicyProfile
}

func NewEngine(profile PolicyProfile) *Engine {
	return &Engine{Profile: profile}
}

func NewEngineWithShadow(profile, shadow PolicyProfile) *Engine {
	return &Engine{Profile: profile, ShadowProfile: &shadow}
}

func (e *Engine) Decide(input PolicyInput) PolicyResult {
	if input.IsSuperUser {
		return PolicyResult{
			Action: moderation.ActionAllow,
			Reason: "superuser exempt from automated sanctions",
			Explanation: DecisionExplanation{
				ProfileName:   e.Profile.Name,
				PolicyVersion: e.Profile.Version,
			},
		}
	}

	spamSignals := filterSpam(input.Signals)
	if len(spamSignals) == 0 {
		return PolicyResult{
			Action: moderation.ActionAllow,
			Reason: "message allowed",
			Explanation: DecisionExplanation{
				ProfileName:   e.Profile.Name,
				PolicyVersion: e.Profile.Version,
			},
		}
	}

	riskType := classifyRisk(spamSignals)
	matchedRules := extractRuleIDs(spamSignals)

	baseLevel := e.Profile.Matrix[riskType]
	if baseLevel == LevelNone {
		baseLevel = e.Profile.Matrix[RiskUnknown]
	}

	finalLevel := ApplyEscalation(baseLevel, input.StrikeCount, e.Profile.Escalate)

	if input.WarnStrikes > 0 && input.StrikeCount < input.WarnStrikes && finalLevel >= LevelMute {
		finalLevel = LevelWarn
	}

	if input.SoftBanMode && finalLevel >= LevelBan {
		finalLevel = LevelDeleteAndMute
	}

	action, duration, restrict := actionLevelToAction(finalLevel, input.FirstStrike, input.SecondStrike)

	reason := fmt.Sprintf("%s detected", riskType)
	if len(matchedRules) > 0 {
		reason = fmt.Sprintf("%s: %s", riskType, strings.Join(matchedRules, ", "))
	}

	result := PolicyResult{
		Action:   action,
		Duration: duration,
		Restrict: restrict,
		Reason:   reason,
		DryRun:   input.DryRun,
		Explanation: DecisionExplanation{
			ProfileName:   e.Profile.Name,
			RiskType:      riskType,
			BaseAction:    baseLevel,
			FinalAction:   finalLevel,
			StrikeCount:   input.StrikeCount,
			MatchedRules:  matchedRules,
			PolicyVersion: e.Profile.Version,
		},
	}

	if e.ShadowProfile != nil {
		shadowEngine := &Engine{Profile: *e.ShadowProfile}
		shadowResult := shadowEngine.Decide(PolicyInput{
			Signals:      input.Signals,
			StrikeCount:  input.StrikeCount,
			IsSuperUser:  input.IsSuperUser,
			SoftBanMode:  input.SoftBanMode,
			FirstStrike:  input.FirstStrike,
			SecondStrike: input.SecondStrike,
			WarnStrikes:  input.WarnStrikes,
		})
		result.Shadow = &shadowResult
	}

	return result
}

func filterSpam(results []spamcheck.Response) []spamcheck.Response {
	out := make([]spamcheck.Response, 0, len(results))
	for _, r := range results {
		if r.Spam {
			out = append(out, r)
		}
	}
	return out
}

func classifyRisk(signals []spamcheck.Response) RiskType {
	for _, s := range signals {
		if id := s.RuleID; id != "" {
			if rt := ClassifyRisk(id); rt != RiskUnknown {
				return rt
			}
		}
		if rt := ClassifyRisk(s.Name); rt != RiskUnknown {
			return rt
		}
	}
	return RiskUnknown
}

func extractRuleIDs(signals []spamcheck.Response) []string {
	ids := make([]string, 0, len(signals))
	for _, s := range signals {
		id := s.RuleID
		if id == "" {
			id = s.Name
		}
		ids = append(ids, id)
	}
	return ids
}

func actionLevelToAction(level ActionLevel, firstStrike, secondStrike time.Duration) (moderation.Action, time.Duration, bool) {
	if firstStrike <= 0 {
		firstStrike = 30 * time.Minute
	}
	if secondStrike <= 0 {
		secondStrike = 6 * time.Hour
	}

	switch level {
	case LevelNone:
		return moderation.ActionAllow, 0, false
	case LevelWarn:
		return moderation.ActionWarn, 0, false
	case LevelMute:
		return moderation.ActionRestrict, firstStrike, true
	case LevelDeleteAndMute:
		return moderation.ActionRestrict, secondStrike, true
	case LevelBan:
		return moderation.ActionBan, permanentBanDuration, false
	default:
		return moderation.ActionBan, permanentBanDuration, false
	}
}

var permanentBanDuration = time.Hour * 24 * 400
