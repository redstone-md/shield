package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
)

type ruleSetProviderSpy struct {
	get    func(ctx context.Context, workspaceID string) (rules.RuleSet, error)
	update func(ctx context.Context, workspaceID string, source string, rs rules.RuleSet) (rules.RuleSet, error)
}

func (s ruleSetProviderSpy) Get(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
	return s.get(ctx, workspaceID)
}

func (s ruleSetProviderSpy) Update(ctx context.Context, workspaceID string, source string, rs rules.RuleSet) (rules.RuleSet, error) {
	return s.update(ctx, workspaceID, source, rs)
}

func TestServer_getRuleSetHandler(t *testing.T) {
	server := NewServer(Config{
		Settings: Settings{InstanceID: "gr1"},
		RuleSetProvider: ruleSetProviderSpy{
			get: func(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
				assert.Equal(t, "gr1", workspaceID)
				return rules.RuleSet{
					WorkspaceID: "gr1",
					Version:     4,
					Meta:        rules.MetaRules{LinksLimit: 3},
				}, nil
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/rules", http.NoBody)
	rr := httptest.NewRecorder()

	server.getRuleSetHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var got rules.RuleSet
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "gr1", got.WorkspaceID)
	assert.Equal(t, 4, got.Version)
	assert.Equal(t, 3, got.Meta.LinksLimit)
}

func TestServer_updateRuleSetHandler(t *testing.T) {
	var captured rules.RuleSet
	server := NewServer(Config{
		Settings: Settings{InstanceID: "gr1"},
		RuleSetProvider: ruleSetProviderSpy{
			update: func(ctx context.Context, workspaceID string, source string, rs rules.RuleSet) (rules.RuleSet, error) {
				assert.Equal(t, "gr1", workspaceID)
				assert.Equal(t, "api", source)
				captured = rs
				rs.WorkspaceID = workspaceID
				rs.Source = source
				rs.Version = 2
				return rs, nil
			},
		},
	})

	reqBody := rules.RuleSet{
		Meta: rules.MetaRules{LinksLimit: 8},
		Moderation: rules.ModerationRules{
			FirstStrike:  10 * time.Minute,
			SecondStrike: time.Hour,
			DryRun:       true,
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/rules", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	server.updateRuleSetHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 8, captured.Meta.LinksLimit)
	assert.True(t, captured.Moderation.DryRun)

	var got rules.RuleSet
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "gr1", got.WorkspaceID)
	assert.Equal(t, "api", got.Source)
	assert.Equal(t, 2, got.Version)
}

func TestServer_updateRuleSetHandlerRejectsBadJSON(t *testing.T) {
	server := NewServer(Config{
		Settings:        Settings{InstanceID: "gr1"},
		RuleSetProvider: ruleSetProviderSpy{},
	})

	req := httptest.NewRequest(http.MethodPut, "/rules", bytes.NewBufferString("{bad"))
	rr := httptest.NewRecorder()

	server.updateRuleSetHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
