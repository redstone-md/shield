package spamcheck

import (
	"encoding/json"
	"testing"
)

func TestResponse_ScoringFields(t *testing.T) {
	r := Response{
		Name:           "stopword",
		Spam:           true,
		Details:        "matched: casino",
		Score:          1.0,
		Weight:         1.5,
		RuleID:         "stopword",
		NormalizedText: "casino",
	}

	if r.Score != 1.0 {
		t.Errorf("Score = %v, want 1.0", r.Score)
	}
	if r.Weight != 1.5 {
		t.Errorf("Weight = %v, want 1.5", r.Weight)
	}
	if r.RuleID != "stopword" {
		t.Errorf("RuleID = %q, want %q", r.RuleID, "stopword")
	}
	if r.NormalizedText != "casino" {
		t.Errorf("NormalizedText = %q, want %q", r.NormalizedText, "casino")
	}
}

func TestResponse_ZeroValueBackwardCompat(t *testing.T) {
	r := Response{Name: "test", Spam: true, Details: "some detail"}
	if r.Score != 0 {
		t.Errorf("default Score = %v, want 0", r.Score)
	}
	if r.Weight != 0 {
		t.Errorf("default Weight = %v, want 0", r.Weight)
	}
	if r.RuleID != "" {
		t.Errorf("default RuleID = %q, want empty", r.RuleID)
	}
	if r.NormalizedText != "" {
		t.Errorf("default NormalizedText = %q, want empty", r.NormalizedText)
	}
}

func TestResponse_JSONSerialization(t *testing.T) {
	r := Response{
		Name:           "similarity",
		Spam:           true,
		Details:        "0.95/0.80",
		Score:          0.95,
		Weight:         1.0,
		RuleID:         "similarity",
		NormalizedText: "free money now click here",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Score != r.Score {
		t.Errorf("Score roundtrip = %v, want %v", got.Score, r.Score)
	}
	if got.Weight != r.Weight {
		t.Errorf("Weight roundtrip = %v, want %v", got.Weight, r.Weight)
	}
	if got.RuleID != r.RuleID {
		t.Errorf("RuleID roundtrip = %q, want %q", got.RuleID, r.RuleID)
	}
	if got.NormalizedText != r.NormalizedText {
		t.Errorf("NormalizedText roundtrip = %q, want %q", got.NormalizedText, r.NormalizedText)
	}
}

func TestResponse_JSONOmitsZeroScoringFields(t *testing.T) {
	r := Response{Name: "test", Spam: false, Details: "ok"}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if want := `"score"`; contains(s, want) {
		t.Errorf("JSON should omit zero score, got: %s", s)
	}
	if want := `"weight"`; contains(s, want) {
		t.Errorf("JSON should omit zero weight, got: %s", s)
	}
	if want := `"rule_id"`; contains(s, want) {
		t.Errorf("JSON should omit empty rule_id, got: %s", s)
	}
	if want := `"normalized_text"`; contains(s, want) {
		t.Errorf("JSON should omit empty normalized_text, got: %s", s)
	}
}

func TestRiskScore_Struct(t *testing.T) {
	rs := RiskScore{
		Total:    2.5,
		Signals:  []Response{{Name: "a", Spam: true, Score: 1.0, Weight: 1.5}, {Name: "b", Spam: true, Score: 1.0, Weight: 1.0}},
		Decision: true,
		Reason:   "threshold exceeded",
	}
	if !rs.Decision {
		t.Error("Decision = false, want true")
	}
	if len(rs.Signals) != 2 {
		t.Errorf("len(Signals) = %d, want 2", len(rs.Signals))
	}
	if rs.Total != 2.5 {
		t.Errorf("Total = %v, want 2.5", rs.Total)
	}
}

func TestRiskScore_JSONRoundtrip(t *testing.T) {
	rs := RiskScore{
		Total:    1.8,
		Signals:  []Response{{Name: "stopword", Spam: true, Score: 1.0, Weight: 1.0, RuleID: "stopword"}},
		Decision: true,
		Reason:   "score 1.80 >= threshold 1.00",
	}
	data, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got RiskScore
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Total != rs.Total {
		t.Errorf("Total = %v, want %v", got.Total, rs.Total)
	}
	if got.Decision != rs.Decision {
		t.Errorf("Decision = %v, want %v", got.Decision, rs.Decision)
	}
	if len(got.Signals) != 1 {
		t.Errorf("len(Signals) = %d, want 1", len(got.Signals))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || sub == "" ||
		(s != "" && sub != "" && jsonContains(s, sub)))
}

func jsonContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
