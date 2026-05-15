package policy

import (
	"testing"
	"time"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestIntegration_SameSignalsDifferentProfiles(t *testing.T) {
	sig := []spamcheck.Response{
		{Name: "stop-word", Spam: true, RuleID: "stop-word", Score: 1.0, Weight: 1.0},
	}

	perm := NewEngine(PermissiveProfile()).Decide(PolicyInput{Signals: sig})
	bal := NewEngine(BalancedProfile()).Decide(PolicyInput{Signals: sig})
	str := NewEngine(StrictProfile()).Decide(PolicyInput{Signals: sig})

	if perm.Action == moderation.ActionBan {
		t.Errorf("permissive should not ban for first spam, got %s", perm.Action)
	}
	if str.Action != moderation.ActionBan {
		t.Errorf("strict should ban for spam, got %s", str.Action)
	}
	if bal.Action == moderation.ActionAllow {
		t.Errorf("balanced should not allow spam, got %s", bal.Action)
	}

	t.Logf("permissive: %s (base=%d final=%d)", perm.Action, perm.Explanation.BaseAction, perm.Explanation.FinalAction)
	t.Logf("balanced:   %s (base=%d final=%d)", bal.Action, bal.Explanation.BaseAction, bal.Explanation.FinalAction)
	t.Logf("strict:     %s (base=%d final=%d)", str.Action, str.Explanation.BaseAction, str.Explanation.FinalAction)
}

func TestIntegration_EscalationProgression(t *testing.T) {
	e := NewEngine(PermissiveProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	r0 := e.Decide(PolicyInput{Signals: sig, StrikeCount: 0})
	r3 := e.Decide(PolicyInput{Signals: sig, StrikeCount: 3})
	r5 := e.Decide(PolicyInput{Signals: sig, StrikeCount: 5})

	if r5.Explanation.FinalAction <= r0.Explanation.FinalAction {
		t.Errorf("high strikes should escalate past base: r0=%d, r5=%d", r0.Explanation.FinalAction, r5.Explanation.FinalAction)
	}
	if r3.Explanation.FinalAction <= r0.Explanation.FinalAction {
		t.Errorf("strike 3 should escalate past base: r0=%d, r3=%d", r0.Explanation.FinalAction, r3.Explanation.FinalAction)
	}
	if r5.Explanation.FinalAction < r3.Explanation.FinalAction {
		t.Errorf("more strikes should not decrease action: r3=%d, r5=%d", r3.Explanation.FinalAction, r5.Explanation.FinalAction)
	}

	t.Logf("strike 0: %s (final=%d)", r0.Action, r0.Explanation.FinalAction)
	t.Logf("strike 3: %s (final=%d)", r3.Action, r3.Explanation.FinalAction)
	t.Logf("strike 5: %s (final=%d)", r5.Action, r5.Explanation.FinalAction)
}

func TestIntegration_ShadowMode(t *testing.T) {
	actual := NewEngine(PermissiveProfile())
	shadow := StrictProfile()
	engine := NewEngineWithShadow(actual.Profile, shadow)

	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}
	result := engine.Decide(PolicyInput{Signals: sig})

	if result.Shadow == nil {
		t.Fatal("expected shadow result")
	}
	if result.Shadow.Explanation.ProfileName != "strict" {
		t.Errorf("shadow profile should be strict, got %s", result.Shadow.Explanation.ProfileName)
	}
	if result.Explanation.ProfileName != "permissive" {
		t.Errorf("actual profile should be permissive, got %s", result.Explanation.ProfileName)
	}
	if result.Shadow.Action != result.Action {
		if result.Shadow.Action != moderation.ActionBan {
			t.Errorf("shadow strict should ban, got %s", result.Shadow.Action)
		}
	}

	t.Logf("actual: %s (%s)", result.Action, result.Explanation.ProfileName)
	t.Logf("shadow: %s (%s)", result.Shadow.Action, result.Shadow.Explanation.ProfileName)
}

func TestIntegration_SuperUserOverride(t *testing.T) {
	e := NewEngine(StrictProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	result := e.Decide(PolicyInput{Signals: sig, IsSuperUser: true})
	if result.Action != moderation.ActionAllow {
		t.Errorf("superuser should always be allowed, got %s", result.Action)
	}
}

func TestIntegration_DryRunFlag(t *testing.T) {
	e := NewEngine(StrictProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	result := e.Decide(PolicyInput{Signals: sig, DryRun: true})
	if !result.DryRun {
		t.Error("dry run flag should be set in result")
	}
	if result.Action == moderation.ActionAllow {
		t.Error("dry run should still compute action, not allow")
	}
}

func TestIntegration_AllRiskTypes(t *testing.T) {
	e := NewEngine(BalancedProfile())
	cases := map[string]RiskType{
		"stop-word":     RiskSpam,
		"duplicate":     RiskRaid,
		"mentions":      RiskAbuse,
		"giveaway":      RiskScam,
		"video-only":    RiskNSFW,
		"unknown-check": RiskUnknown,
	}

	for name, expected := range cases {
		sig := []spamcheck.Response{{Name: name, Spam: true, RuleID: name}}
		result := e.Decide(PolicyInput{Signals: sig})
		if result.Explanation.RiskType != expected {
			t.Errorf("check %q: expected risk %s, got %s", name, expected, result.Explanation.RiskType)
		}
		if result.Action == moderation.ActionAllow {
			t.Errorf("check %q: should not allow spam", name)
		}
	}
}

func TestIntegration_SoftBanOverride(t *testing.T) {
	e := NewEngine(StrictProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	result := e.Decide(PolicyInput{Signals: sig, SoftBanMode: true})
	if result.Action == moderation.ActionBan {
		t.Errorf("soft ban mode should not permanently ban, got %s", result.Action)
	}
	if !result.Restrict {
		t.Error("soft ban mode should set restrict=true")
	}
}

func TestIntegration_ProfileVersionInAudit(t *testing.T) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	result := e.Decide(PolicyInput{Signals: sig})
	if result.Explanation.PolicyVersion != 1 {
		t.Errorf("expected version 1, got %d", result.Explanation.PolicyVersion)
	}
	if result.Explanation.ProfileName != "balanced" {
		t.Errorf("expected balanced, got %s", result.Explanation.ProfileName)
	}
}

func TestIntegration_MultipleSignals(t *testing.T) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{
		{Name: "stop-word", Spam: true, RuleID: "stop-word"},
		{Name: "links", Spam: true, RuleID: "meta-links"},
		{Name: "similarity", Spam: true, RuleID: "similarity"},
	}

	result := e.Decide(PolicyInput{Signals: sig})
	if result.Action == moderation.ActionAllow {
		t.Error("multiple spam signals should not be allowed")
	}
	if len(result.Explanation.MatchedRules) != 3 {
		t.Errorf("expected 3 matched rules, got %d", len(result.Explanation.MatchedRules))
	}
}

func TestIntegration_EmptySignals(t *testing.T) {
	e := NewEngine(StrictProfile())
	result := e.Decide(PolicyInput{})
	if result.Action != moderation.ActionAllow {
		t.Errorf("empty signals should allow, got %s", result.Action)
	}
}

func TestIntegration_MixedSignals(t *testing.T) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{
		{Name: "length", Spam: false},
		{Name: "stop-word", Spam: true, RuleID: "stop-word"},
		{Name: "pre-approved", Spam: false},
	}

	result := e.Decide(PolicyInput{Signals: sig})
	if result.Action == moderation.ActionAllow {
		t.Error("has spam signal, should not allow")
	}
}

func TestIntegration_CustomDurations(t *testing.T) {
	e := NewEngine(PermissiveProfile())
	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	result := e.Decide(PolicyInput{
		Signals:      sig,
		StrikeCount:  0,
		FirstStrike:  5 * time.Minute,
		SecondStrike: 1 * time.Hour,
	})

	if result.Duration != 5*time.Minute {
		t.Errorf("expected mute duration 5m, got %v", result.Duration)
	}
}

func TestIntegration_PolicyVersioningAcrossUpdates(t *testing.T) {
	p1 := BalancedProfile()
	p1.Version = 1
	e1 := NewEngine(p1)

	p2 := BalancedProfile()
	p2.Version = 2
	e2 := NewEngine(p2)

	sig := []spamcheck.Response{{Name: "stop-word", Spam: true, RuleID: "stop-word"}}

	r1 := e1.Decide(PolicyInput{Signals: sig})
	r2 := e2.Decide(PolicyInput{Signals: sig})

	if r1.Explanation.PolicyVersion != 1 || r2.Explanation.PolicyVersion != 2 {
		t.Errorf("versions should differ: v1=%d, v2=%d", r1.Explanation.PolicyVersion, r2.Explanation.PolicyVersion)
	}
	if r1.Action != r2.Action {
		t.Errorf("same profile different version should produce same action: %s vs %s", r1.Action, r2.Action)
	}
}
