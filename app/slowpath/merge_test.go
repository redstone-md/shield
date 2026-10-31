package slowpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldEscalateForceLLM(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{ForceLLM: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationForceLLM, reason)
}

func TestShouldEscalateUserReport(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{UserReport: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationUserReport, reason)
}

func TestShouldEscalateHighRiskPolicy(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{HighRiskPolicy: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationHighRiskPolicy, reason)
}

func TestShouldEscalateImageContent(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{HasImages: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationImageContent, reason)
}

func TestShouldEscalateAmbiguous(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{AmbiguousScore: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationAmbiguousFast, reason)
}

func TestShouldEscalateNone(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{})
	assert.False(t, ok)
	assert.Empty(t, reason)
}
