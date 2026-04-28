package policy

import (
	"testing"
	"time"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestEngine_Decide_AllowNoSpam(t *testing.T) {
	e := NewEngine(BalancedProfile())
	result := e.Decide(PolicyInput{Signals: []spamcheck.Response{{Name: "test", Spam: false}}})
	if result.Action != moderation.ActionAllow {
		t.Errorf("expected allow, got %s", result.Action)
	}
}

func TestEngine_Decide_AllowEmptySignals(t *testing.T) {
	e := NewEngine(BalancedProfile())
	result := e.Decide(PolicyInput{})
	if result.Action != moderation.ActionAllow {
		t.Errorf("expected allow, got %s", result.Action)
	}
}

func TestEngine_Decide_SuperUserExempt(t *testing.T) {
	e := NewEngine(StrictProfile())
	result := e.Decide(PolicyInput{
		IsSuperUser: true,
		Signals:     []spamcheck.Response{{Name: "stop-word", Spam: true}},
	})
	if result.Action != moderation.ActionAllow {
		t.Errorf("expected allow for superuser, got %s", result.Action)
	}
}

func TestEngine_Decide_SameSignalsDifferentProfiles(t *testing.T) {
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	perm := NewEngine(PermissiveProfile()).Decide(PolicyInput{Signals: sig})
	bal := NewEngine(BalancedProfile()).Decide(PolicyInput{Signals: sig})
	str := NewEngine(StrictProfile()).Decide(PolicyInput{Signals: sig})

	if perm.Explanation.BaseAction >= bal.Explanation.BaseAction {
		t.Errorf("permissive base %d should be less than balanced base %d", perm.Explanation.BaseAction, bal.Explanation.BaseAction)
	}
	if bal.Explanation.BaseAction >= str.Explanation.BaseAction {
		t.Errorf("balanced base %d should be less than strict base %d", bal.Explanation.BaseAction, str.Explanation.BaseAction)
	}

	if str.Action != moderation.ActionBan {
		t.Errorf("strict profile spam should ban, got %s", str.Action)
	}
}

func TestEngine_Decide_RiskTypeClassification(t *testing.T) {
	e := NewEngine(BalancedProfile())

	result := e.Decide(PolicyInput{
		Signals: []spamcheck.Response{{Name: "duplicate", Spam: true, RuleID: "duplicate"}},
	})
	if result.Explanation.RiskType != RiskRaid {
		t.Errorf("expected raid risk, got %s", result.Explanation.RiskType)
	}

	result = e.Decide(PolicyInput{
		Signals: []spamcheck.Response{{Name: "mentions", Spam: true, RuleID: "mentions"}},
	})
	if result.Explanation.RiskType != RiskAbuse {
		t.Errorf("expected abuse risk, got %s", result.Explanation.RiskType)
	}
}

func TestEngine_Decide_SoftBanMode(t *testing.T) {
	e := NewEngine(StrictProfile())
	result := e.Decide(PolicyInput{
		Signals:     []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}},
		SoftBanMode: true,
	})
	if result.Action != moderation.ActionRestrict {
		t.Errorf("soft ban should restrict, got %s", result.Action)
	}
}

func TestEngine_Decide_Explainability(t *testing.T) {
	e := NewEngine(BalancedProfile())
	result := e.Decide(PolicyInput{
		Signals: []spamcheck.Response{
			{Name: "stop-word", Spam: true, RuleID: "stop-word"},
		},
	})
	if result.Explanation.ProfileName != "balanced" {
		t.Errorf("expected balanced profile, got %s", result.Explanation.ProfileName)
	}
	if result.Explanation.PolicyVersion != 1 {
		t.Errorf("expected version 1, got %d", result.Explanation.PolicyVersion)
	}
	if len(result.Explanation.MatchedRules) != 1 || result.Explanation.MatchedRules[0] != "stop-word" {
		t.Errorf("expected matched rules [stop-word], got %v", result.Explanation.MatchedRules)
	}
}

func TestEngine_Decide_EscalationBumpsAction(t *testing.T) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	r0 := e.Decide(PolicyInput{Signals: sig, StrikeCount: 0})
	r3 := e.Decide(PolicyInput{Signals: sig, StrikeCount: 3})

	if r3.Explanation.FinalAction <= r0.Explanation.FinalAction {
		t.Errorf("escalation should increase action: base=%d, escalated=%d", r0.Explanation.FinalAction, r3.Explanation.FinalAction)
	}
}

func TestActionLevelToAction_Defaults(t *testing.T) {
	a, d, r := actionLevelToAction(LevelWarn, 0, 0)
	if a != moderation.ActionDelete || d != 0 || r != false {
		t.Errorf("warn: expected delete/0/false, got %s/%v/%v", a, d, r)
	}

	a, d, r = actionLevelToAction(LevelMute, 0, 0)
	if a != moderation.ActionRestrict || !r {
		t.Errorf("mute: expected restrict/.../true, got %s/%v/%v", a, d, r)
	}
	if d != 30*time.Minute {
		t.Errorf("mute default duration: expected 30m, got %v", d)
	}

	a, d, r = actionLevelToAction(LevelBan, 0, 0)
	if a != moderation.ActionBan || r {
		t.Errorf("ban: expected ban/.../false, got %s/%v/%v", a, d, r)
	}
}
