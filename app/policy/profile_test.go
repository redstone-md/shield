package policy

import (
	"testing"
	"time"
)

func TestPermissiveProfile(t *testing.T) {
	p := PermissiveProfile()
	if p.Name != "permissive" {
		t.Fatalf("name: got %q, want %q", p.Name, "permissive")
	}
	if p.Version != 1 {
		t.Fatalf("version: got %d, want 1", p.Version)
	}
	if len(p.Matrix) != 6 {
		t.Fatalf("matrix entries: got %d, want 6", len(p.Matrix))
	}
	if p.Matrix[RiskSpam] != LevelMute {
		t.Fatalf("spam action: got %d, want LevelMute", p.Matrix[RiskSpam])
	}
	if p.Matrix[RiskAbuse] != LevelWarn {
		t.Fatalf("abuse action: got %d, want LevelWarn", p.Matrix[RiskAbuse])
	}
	if !p.Escalate.Enabled {
		t.Fatal("escalation should be enabled")
	}
}

func TestBalancedProfile(t *testing.T) {
	p := BalancedProfile()
	if p.Name != "balanced" {
		t.Fatalf("name: got %q, want %q", p.Name, "balanced")
	}
	if p.Matrix[RiskSpam] != LevelDeleteAndMute {
		t.Fatalf("spam action: got %d, want LevelDeleteAndMute", p.Matrix[RiskSpam])
	}
}

func TestStrictProfile(t *testing.T) {
	p := StrictProfile()
	if p.Name != "strict" {
		t.Fatalf("name: got %q, want %q", p.Name, "strict")
	}
	if p.Matrix[RiskSpam] != LevelBan {
		t.Fatalf("spam action: got %d, want LevelBan", p.Matrix[RiskSpam])
	}
	if p.Matrix[RiskNSFW] != LevelDeleteAndMute {
		t.Fatalf("nsfw action: got %d, want LevelDeleteAndMute", p.Matrix[RiskNSFW])
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		checkName string
		want      RiskType
	}{
		{"stop-word", RiskSpam},
		{"similarity", RiskSpam},
		{"duplicate", RiskRaid},
		{"mentions", RiskAbuse},
		{"giveaway", RiskScam},
		{"video-only", RiskNSFW},
		{"unknown-check", RiskUnknown},
		{"", RiskUnknown},
		{"STO-PWORD", RiskUnknown},
		{"meta-links", RiskSpam},
	}
	for _, tc := range tests {
		got := ClassifyRisk(tc.checkName)
		if got != tc.want {
			t.Errorf("ClassifyRisk(%q) = %q, want %q", tc.checkName, got, tc.want)
		}
	}
}

func TestClassifyRiskSubstring(t *testing.T) {
	got := ClassifyRisk("custom-stop-word-v2")
	if got != RiskSpam {
		t.Errorf("ClassifyRisk substring match: got %q, want %q", got, RiskSpam)
	}
}

func TestResolveProfile(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"permissive", "permissive"},
		{"PERMISSIVE", "permissive"},
		{"strict", "strict"},
		{"balanced", "balanced"},
		{"", "balanced"},
		{"unknown", "balanced"},
	}
	for _, tc := range tests {
		p := ResolveProfile(tc.name)
		if p.Name != tc.want {
			t.Errorf("ResolveProfile(%q) = %q, want %q", tc.name, p.Name, tc.want)
		}
	}
}

func TestProfileCreatedAt(t *testing.T) {
	p := PermissiveProfile()
	if p.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
	if time.Since(p.CreatedAt) > 5*time.Second {
		t.Fatal("CreatedAt should be recent")
	}
}
