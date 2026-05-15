package storage

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestAuditTenantIDInQueries(t *testing.T) {
	mustHaveTenantID := []struct {
		name  string
		qmap  *engine.QueryMap
		table string
	}{
		{"approved_users", approvedUsersQueries, "approved_users"},
		{"detected_spam", detectedSpamQueries, "detected_spam"},
		{"dictionary", dictionaryQueries, "dictionary"},
		{"incoming_events", incomingEventsQueries, "incoming_events"},
		{"locator", locatorQueries, "messages"},
		{"moderation_actions", moderationActionsQueries, "moderation_actions"},
		{"reports", reportsQueries, "reports"},
		{"rule_sets", ruleSetQueries, "rule_sets"},
		{"samples", samplesQueries, "samples"},
		{"tenants", tenantsQueries, "tenants"},
		{"tenant_limits", tenantLimitsQueries, "tenant_limits"},
		{"workspaces", workspacesQueries, "workspaces"},
	}

	whereRe := regexp.MustCompile(`(?i)\bWHERE\b`)
	skipPatterns := []string{"CREATE TABLE", "CREATE INDEX", "ALTER TABLE", "IF NOT EXISTS"}

	for _, tc := range mustHaveTenantID {
		t.Run(tc.name, func(t *testing.T) {
			tc.qmap.Iterate(func(cmd engine.DBCmd, q engine.Query) {
				for dbType, query := range map[string]string{"sqlite": q.Sqlite, "postgres": q.Postgres} {
					trimmed := strings.TrimSpace(query)
					skip := false
					for _, pat := range skipPatterns {
						if strings.HasPrefix(trimmed, pat) {
							skip = true
							break
						}
					}
					if skip {
						continue
					}

					if !whereRe.MatchString(trimmed) {
						continue
					}

					assert.Contains(t, trimmed, "tenant_id",
						"%s cmd=%d %s: WHERE clause missing tenant_id\nquery: %s", tc.name, cmd, dbType, trimmed)
				}
			})
		})
	}
}
