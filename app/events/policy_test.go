package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestDefaultPolicyEngine(t *testing.T) {
	engine := defaultPolicyEngine{}
	baseReq := PolicyRequest{
		Event: moderation.IncomingEvent{EventID: "evt-1", CorrelationID: "corr-1"},
		Response: bot.Response{
			Send:        true,
			BanInterval: time.Minute,
			CheckResults: []spamcheck.Response{
				{Name: "rule", Spam: true, Details: "matched spam rule"},
			},
		},
		Message:    &bot.Message{},
		SpamUserID: 42,
	}

	t.Run("allow when not spam", func(t *testing.T) {
		req := baseReq
		req.Response = bot.Response{}
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionAllow, outcome.Decision.Action)
	})

	t.Run("allow for superuser", func(t *testing.T) {
		req := baseReq
		req.IsSuperUser = true
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionAllow, outcome.Decision.Action)
	})

	t.Run("restrict on soft ban", func(t *testing.T) {
		req := baseReq
		req.SoftBanMode = true
		req.UseEscalation = true
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionRestrict, outcome.Decision.Action)
		assert.True(t, outcome.Restrict)
	})

	t.Run("ban after escalation", func(t *testing.T) {
		req := baseReq
		req.StrikeCount = 3
		req.UseEscalation = true
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionBan, outcome.Decision.Action)
		assert.False(t, outcome.Restrict)
	})

	t.Run("ban without escalation source", func(t *testing.T) {
		req := baseReq
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionBan, outcome.Decision.Action)
		assert.False(t, outcome.Restrict)
		assert.Equal(t, time.Minute, outcome.Duration)
	})

	t.Run("delete when spam without ban interval", func(t *testing.T) {
		req := baseReq
		req.Response.BanInterval = 0
		outcome, err := engine.Decide(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionDelete, outcome.Decision.Action)
	})
}
