package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage/engine"
)

func (s *StorageTestSuite) TestTenantIsolation() {
	ctx := context.Background()

	s.Run("sqlite tenant isolation", func() {
		tmpFile1 := filepath.Join(os.TempDir(), "test_tenant_iso1.sqlite")
		tmpFile2 := filepath.Join(os.TempDir(), "test_tenant_iso2.sqlite")
		defer os.Remove(tmpFile1)
		defer os.Remove(tmpFile2)

		db1, err := engine.NewSqlite(tmpFile1, "tenant-a")
		s.Require().NoError(err)
		defer db1.Close()

		db2, err := engine.NewSqlite(tmpFile2, "tenant-b")
		s.Require().NoError(err)
		defer db2.Close()

		s.Run("incoming events", func() {
			ie1, err := NewIncomingEvents(ctx, db1)
			s.Require().NoError(err)
			ie2, err := NewIncomingEvents(ctx, db2)
			s.Require().NoError(err)

			ok, err := ie1.Record(ctx, moderation.IncomingEvent{
				EventID: "evt1", Source: "tg", ChatID: 1, IdempotencyKey: "key1",
				Subject: moderation.Subject{ID: 100}, ReceivedAt: time.Now(),
			})
			s.Require().NoError(err)
			s.True(ok)

			ok, err = ie2.Record(ctx, moderation.IncomingEvent{
				EventID: "evt2", Source: "tg", ChatID: 2, IdempotencyKey: "key2",
				Subject: moderation.Subject{ID: 200}, ReceivedAt: time.Now(),
			})
			s.Require().NoError(err)
			s.True(ok)

			rec1, err := ie1.ByIdempotencyKey(ctx, "key1")
			s.Require().NoError(err)
			s.Equal("evt1", rec1.EventID)

			rec2, err := ie2.ByIdempotencyKey(ctx, "key2")
			s.Require().NoError(err)
			s.Equal("evt2", rec2.EventID)

			_, err = ie1.ByIdempotencyKey(ctx, "key2")
			s.Error(err)

			_, err = ie2.ByIdempotencyKey(ctx, "key1")
			s.Error(err)
		})

		s.Run("moderation actions", func() {
			ma1, err := NewModerationActions(ctx, db1)
			s.Require().NoError(err)
			ma2, err := NewModerationActions(ctx, db2)
			s.Require().NoError(err)

			err = ma1.Add(ctx, ModerationActionEntry{
				EventID: "e1", Command: "ban", Status: "done",
				ChatID: 1, SubjectID: 100, IdempotencyKey: "ik1",
			})
			s.Require().NoError(err)

			err = ma2.Add(ctx, ModerationActionEntry{
				EventID: "e2", Command: "ban", Status: "done",
				ChatID: 2, SubjectID: 200, IdempotencyKey: "ik2",
			})
			s.Require().NoError(err)

			last1, err := ma1.Last(ctx, ModerationActionLookup{
				IdempotencyKey: "ik1", Command: "ban", ChatID: 1, SubjectID: 100,
			})
			s.Require().NoError(err)
			s.True(last1.Found)

			last2, err := ma2.Last(ctx, ModerationActionLookup{
				IdempotencyKey: "ik2", Command: "ban", ChatID: 2, SubjectID: 200,
			})
			s.Require().NoError(err)
			s.True(last2.Found)

			cross, err := ma1.Last(ctx, ModerationActionLookup{
				IdempotencyKey: "ik2", Command: "ban", ChatID: 2, SubjectID: 200,
			})
			s.Require().NoError(err)
			s.False(cross.Found)
		})

		s.Run("reports", func() {
			r1, err := NewReports(ctx, db1)
			s.Require().NoError(err)
			r2, err := NewReports(ctx, db2)
			s.Require().NoError(err)

			err = r1.Add(ctx, Report{
				MsgID: 1, ChatID: 100, ReporterUserID: 10, ReporterUserName: "r1",
				ReportedUserID: 20, ReportedUserName: "b1", MsgText: "spam1", ReportTime: time.Now(),
			})
			s.Require().NoError(err)

			err = r2.Add(ctx, Report{
				MsgID: 2, ChatID: 200, ReporterUserID: 30, ReporterUserName: "r2",
				ReportedUserID: 40, ReportedUserName: "b2", MsgText: "spam2", ReportTime: time.Now(),
			})
			s.Require().NoError(err)

			reps1, err := r1.GetByMessage(ctx, 1, 100)
			s.Require().NoError(err)
			s.Len(reps1, 1)
			s.Equal("spam1", reps1[0].MsgText)

			reps2, err := r2.GetByMessage(ctx, 2, 200)
			s.Require().NoError(err)
			s.Len(reps2, 1)
			s.Equal("spam2", reps2[0].MsgText)

			cross, err := r1.GetByMessage(ctx, 2, 200)
			s.Require().NoError(err)
			s.Len(cross, 0)
		})

		s.Run("rule sets", func() {
			rs1, err := NewRuleSets(ctx, db1)
			s.Require().NoError(err)
			rs2, err := NewRuleSets(ctx, db2)
			s.Require().NoError(err)

			rules1 := rules.RuleSet{WorkspaceID: "tenant-a", Source: "test", Meta: rules.MetaRules{LinksLimit: 5}}
			rules2 := rules.RuleSet{WorkspaceID: "tenant-b", Source: "test", Meta: rules.MetaRules{LinksLimit: 10}}

			_, err = rs1.EnsureBootstrap(ctx, rules1)
			s.Require().NoError(err)
			_, err = rs2.EnsureBootstrap(ctx, rules2)
			s.Require().NoError(err)

			active1, err := rs1.Active(ctx, "tenant-a")
			s.Require().NoError(err)
			s.Equal(5, active1.Meta.LinksLimit)

			active2, err := rs2.Active(ctx, "tenant-b")
			s.Require().NoError(err)
			s.Equal(10, active2.Meta.LinksLimit)
		})

		s.Run("workspaces", func() {
			ws1, err := NewWorkspaces(ctx, db1)
			s.Require().NoError(err)
			ws2, err := NewWorkspaces(ctx, db2)
			s.Require().NoError(err)

			_, err = ws1.Add(ctx, WorkspaceRecord{Name: "ws-alpha"})
			s.Require().NoError(err)
			_, err = ws2.Add(ctx, WorkspaceRecord{Name: "ws-beta"})
			s.Require().NoError(err)

			list1, err := ws1.List(ctx)
			s.Require().NoError(err)
			s.Len(list1, 1)
			s.Equal("ws-alpha", list1[0].Name)

			list2, err := ws2.List(ctx)
			s.Require().NoError(err)
			s.Len(list2, 1)
			s.Equal("ws-beta", list2[0].Name)
		})

		s.Run("tenants", func() {
			t1, err := NewTenants(ctx, db1)
			s.Require().NoError(err)
			t2, err := NewTenants(ctx, db2)
			s.Require().NoError(err)

			err = t1.BootstrapDefault(ctx, "tenant-a", "Tenant A", "owner1")
			s.Require().NoError(err)
			err = t2.BootstrapDefault(ctx, "tenant-b", "Tenant B", "owner2")
			s.Require().NoError(err)

			rec1, err := t1.Get(ctx, "tenant-a")
			s.Require().NoError(err)
			s.Equal("Tenant A", rec1.Name)

			rec2, err := t2.Get(ctx, "tenant-b")
			s.Require().NoError(err)
			s.Equal("Tenant B", rec2.Name)

			_, err = t1.Get(ctx, "tenant-b")
			s.Error(err)

			_, err = t2.Get(ctx, "tenant-a")
			s.Error(err)
		})
	})

	s.Run("postgres tenant isolation", func() {
		if testing.Short() {
			s.T().Skip("skipping postgres test in short mode")
		}

		var pgDB *engine.SQL
		for _, dbt := range s.getTestDB() {
			if dbt.DB.Type() == engine.Postgres {
				pgDB = dbt.DB
				break
			}
		}
		if pgDB == nil {
			s.T().Skip("postgres is not available")
		}

		pgConnStr := s.pgContainer.ConnectionString()

		db1, err := engine.NewPostgres(ctx, pgConnStr, "tenant-a")
		s.Require().NoError(err)
		defer db1.Close()

		db2, err := engine.NewPostgres(ctx, pgConnStr, "tenant-b")
		s.Require().NoError(err)
		defer db2.Close()

		s.Run("incoming events", func() {
			ie1, err := NewIncomingEvents(ctx, db1)
			s.Require().NoError(err)
			ie2, err := NewIncomingEvents(ctx, db2)
			s.Require().NoError(err)

			ok, err := ie1.Record(ctx, moderation.IncomingEvent{
				EventID: "evt1", Source: "tg", ChatID: 1, IdempotencyKey: "key1",
				Subject: moderation.Subject{ID: 100}, ReceivedAt: time.Now(),
			})
			s.Require().NoError(err)
			s.True(ok)

			ok, err = ie2.Record(ctx, moderation.IncomingEvent{
				EventID: "evt2", Source: "tg", ChatID: 2, IdempotencyKey: "key2",
				Subject: moderation.Subject{ID: 200}, ReceivedAt: time.Now(),
			})
			s.Require().NoError(err)
			s.True(ok)

			_, err = ie1.ByIdempotencyKey(ctx, "key2")
			s.Error(err, "tenant-a should not see tenant-b events")
		})

		s.Run("moderation actions", func() {
			ma1, err := NewModerationActions(ctx, db1)
			s.Require().NoError(err)
			ma2, err := NewModerationActions(ctx, db2)
			s.Require().NoError(err)

			err = ma1.Add(ctx, ModerationActionEntry{
				EventID: "e1", Command: "ban", Status: "done",
				ChatID: 1, SubjectID: 100, IdempotencyKey: "ik1",
			})
			s.Require().NoError(err)

			err = ma2.Add(ctx, ModerationActionEntry{
				EventID: "e2", Command: "ban", Status: "done",
				ChatID: 2, SubjectID: 200, IdempotencyKey: "ik2",
			})
			s.Require().NoError(err)

			last1, err := ma1.Last(ctx, ModerationActionLookup{
				IdempotencyKey: "ik1", Command: "ban", ChatID: 1, SubjectID: 100,
			})
			s.Require().NoError(err)
			s.True(last1.Found)

			cross, err := ma1.Last(ctx, ModerationActionLookup{
				IdempotencyKey: "ik2", Command: "ban", ChatID: 2, SubjectID: 200,
			})
			s.Require().NoError(err)
			s.False(cross.Found)
		})

		s.Run("reports", func() {
			r1, err := NewReports(ctx, db1)
			s.Require().NoError(err)
			r2, err := NewReports(ctx, db2)
			s.Require().NoError(err)

			err = r1.Add(ctx, Report{
				MsgID: 101, ChatID: 1001, ReporterUserID: 10, ReporterUserName: "r1",
				ReportedUserID: 20, ReportedUserName: "b1", MsgText: "spam-pg1", ReportTime: time.Now(),
			})
			s.Require().NoError(err)

			err = r2.Add(ctx, Report{
				MsgID: 102, ChatID: 1002, ReporterUserID: 30, ReporterUserName: "r2",
				ReportedUserID: 40, ReportedUserName: "b2", MsgText: "spam-pg2", ReportTime: time.Now(),
			})
			s.Require().NoError(err)

			reps1, err := r1.GetByMessage(ctx, 101, 1001)
			s.Require().NoError(err)
			s.Len(reps1, 1)

			cross, err := r1.GetByMessage(ctx, 102, 1002)
			s.Require().NoError(err)
			s.Len(cross, 0)
		})

		s.Run("rule sets", func() {
			rs1, err := NewRuleSets(ctx, db1)
			s.Require().NoError(err)
			rs2, err := NewRuleSets(ctx, db2)
			s.Require().NoError(err)

			rules1 := rules.RuleSet{WorkspaceID: "tenant-a", Source: "test", Meta: rules.MetaRules{LinksLimit: 5}}
			rules2 := rules.RuleSet{WorkspaceID: "tenant-b", Source: "test", Meta: rules.MetaRules{LinksLimit: 10}}

			_, err = rs1.EnsureBootstrap(ctx, rules1)
			s.Require().NoError(err)
			_, err = rs2.EnsureBootstrap(ctx, rules2)
			s.Require().NoError(err)

			active1, err := rs1.Active(ctx, "tenant-a")
			s.Require().NoError(err)
			s.Equal(5, active1.Meta.LinksLimit)

			active2, err := rs2.Active(ctx, "tenant-b")
			s.Require().NoError(err)
			s.Equal(10, active2.Meta.LinksLimit)
		})

		s.Run("workspaces", func() {
			s.T().Skip("Workspaces.Add uses LastInsertId, unsupported by pgx")
		})

		s.Run("tenants", func() {
			t1, err := NewTenants(ctx, db1)
			s.Require().NoError(err)
			t2, err := NewTenants(ctx, db2)
			s.Require().NoError(err)

			err = t1.BootstrapDefault(ctx, "tenant-a", "Tenant A PG", "owner1")
			s.Require().NoError(err)
			err = t2.BootstrapDefault(ctx, "tenant-b", "Tenant B PG", "owner2")
			s.Require().NoError(err)

			rec1, err := t1.Get(ctx, "tenant-a")
			s.Require().NoError(err)
			s.Equal("Tenant A PG", rec1.Name)

			_, err = t1.Get(ctx, "tenant-b")
			s.Error(err)
		})
	})
}
