package policy

import "testing"

func TestApplyEscalation_Disabled(t *testing.T) {
	cfg := EscalationConfig{Enabled: false, Levels: []ActionLevel{LevelWarn, LevelMute, LevelBan}}
	got := ApplyEscalation(LevelMute, 3, cfg)
	if got != LevelMute {
		t.Errorf("expected LevelMute, got %d", got)
	}
}

func TestApplyEscalation_ZeroStrikes(t *testing.T) {
	cfg := EscalationConfig{Enabled: true, Levels: []ActionLevel{LevelWarn, LevelMute, LevelBan}}
	got := ApplyEscalation(LevelMute, 0, cfg)
	if got != LevelMute {
		t.Errorf("expected LevelMute (no escalation), got %d", got)
	}
}

func TestApplyEscalation_EscalatesUp(t *testing.T) {
	cfg := EscalationConfig{Enabled: true, Levels: []ActionLevel{LevelWarn, LevelDeleteAndMute, LevelBan}}
	got := ApplyEscalation(LevelWarn, 3, cfg)
	if got != LevelBan {
		t.Errorf("strike 3 from Warn: expected LevelBan, got %d", got)
	}
}

func TestApplyEscalation_EscalationLowerThanBase(t *testing.T) {
	cfg := EscalationConfig{Enabled: true, Levels: []ActionLevel{LevelWarn, LevelMute}}
	got := ApplyEscalation(LevelBan, 2, cfg)
	if got != LevelBan {
		t.Errorf("escalation below base: expected LevelBan, got %d", got)
	}
}

func TestApplyEscalation_EmptyLevels(t *testing.T) {
	cfg := EscalationConfig{Enabled: true, Levels: nil}
	got := ApplyEscalation(LevelMute, 3, cfg)
	if got != LevelMute {
		t.Errorf("empty levels: expected LevelMute, got %d", got)
	}
}

func TestApplyEscalation_FullChain(t *testing.T) {
	cfg := EscalationConfig{Enabled: true, Levels: []ActionLevel{LevelMute, LevelDeleteAndMute, LevelBan, LevelBan}}
	cases := []struct {
		strikes int
		want    ActionLevel
	}{
		{0, LevelMute},
		{1, LevelMute},
		{2, LevelDeleteAndMute},
		{3, LevelBan},
		{4, LevelBan},
		{10, LevelBan},
	}
	for _, tc := range cases {
		got := ApplyEscalation(LevelMute, tc.strikes, cfg)
		if got != tc.want {
			t.Errorf("strikes=%d: expected %d, got %d", tc.strikes, tc.want, got)
		}
	}
}

func TestApplyEscalation_PermissiveProfile(t *testing.T) {
	profile := PermissiveProfile()
	got := ApplyEscalation(LevelMute, 3, profile.Escalate)
	if got != LevelDeleteAndMute {
		t.Errorf("permissive strike 3 from Mute: expected LevelDeleteAndMute, got %d", got)
	}
}

func TestApplyEscalation_StrictProfile(t *testing.T) {
	profile := StrictProfile()
	got := ApplyEscalation(LevelBan, 2, profile.Escalate)
	if got != LevelBan {
		t.Errorf("strict strike 2 from Ban: expected LevelBan, got %d", got)
	}
}
