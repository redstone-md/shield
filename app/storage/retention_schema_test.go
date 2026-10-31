package storage

import (
	"fmt"
	"time"

	"github.com/redstone-md/shield/app/moderation"
)

// runs every cleaner against the schemas the stores create, so a wrong table or column name fails the test.
func (s *StorageTestSuite) TestRetentionService_CleanersMatchRealSchema() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			_, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			_, err = NewAppealStorage(ctx, db)
			s.Require().NoError(err)
			_, err = NewDetectedSpam(ctx, db)
			s.Require().NoError(err)
			_, err = NewLabelStorage(ctx, db)
			s.Require().NoError(err)
			_, err = NewCandidateStorage(ctx, db)
			s.Require().NoError(err)
			_, err = NewKnowledgeSnapshotStorage(ctx, db)
			s.Require().NoError(err)
			_, err = NewIncomingEvents(ctx, db)
			s.Require().NoError(err)
			_, err = NewModerationActions(ctx, db)
			s.Require().NoError(err)
			_, err = NewReports(ctx, db)
			s.Require().NoError(err)
			_, err = NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			const longTTL = 100 * 365 * 24 * time.Hour
			svc := NewRetentionService(db, RetentionConfig{
				IncidentsTTL: longTTL, AppealsTTL: longTTL, DetectedSpamTTL: longTTL, IncomingEventsTTL: longTTL,
				ModerationActionsTTL: longTTL, LabelsTTL: longTTL, CandidatesTTL: longTTL, UsageCountersTTL: longTTL,
			})
			_, err = svc.CleanNow(ctx)
			s.Require().NoError(err)
		})
	}
}

func (s *StorageTestSuite) TestRetentionService_DeletesExpiredRows() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			events, err := NewIncomingEvents(ctx, db)
			s.Require().NoError(err)
			actions, err := NewModerationActions(ctx, db)
			s.Require().NoError(err)
			reports, err := NewReports(ctx, db)
			s.Require().NoError(err)

			now := time.Now().UTC()
			ts := now.UnixNano()
			old := now.Add(-48 * time.Hour)

			event := func(id string, at time.Time) moderation.IncomingEvent {
				return moderation.IncomingEvent{
					EventID: id, CorrelationID: id, TenantID: "tg-spam", Source: "telegram.update",
					IdempotencyKey: id, ReceivedAt: at,
				}
			}
			oldEvent, freshEvent := fmt.Sprintf("old-%d", ts), fmt.Sprintf("fresh-%d", ts)
			_, err = events.Reserve(ctx, event(oldEvent, old))
			s.Require().NoError(err)
			_, err = events.Reserve(ctx, event(freshEvent, now))
			s.Require().NoError(err)

			oldAction, freshAction := fmt.Sprintf("action-old-%d", ts), fmt.Sprintf("action-fresh-%d", ts)
			s.Require().NoError(actions.Add(ctx, ModerationActionEntry{EventID: oldAction, Command: "ban_user", Status: "completed", CreatedAt: old}))
			s.Require().NoError(actions.Add(ctx, ModerationActionEntry{EventID: freshAction, Command: "ban_user", Status: "completed", CreatedAt: now}))

			oldMsg, freshMsg := int(ts%1_000_000), int(ts%1_000_000)+1
			s.Require().NoError(reports.Add(ctx, Report{MsgID: oldMsg, ChatID: 1, ReporterUserID: 1, ReportedUserID: 2, MsgText: "x"}))
			s.Require().NoError(reports.Add(ctx, Report{MsgID: freshMsg, ChatID: 1, ReporterUserID: 1, ReportedUserID: 2, MsgText: "x"}))
			_, err = db.ExecContext(ctx, db.Adopt("UPDATE reports SET report_time = ? WHERE msg_id = ?"), old, oldMsg)
			s.Require().NoError(err)

			svc := NewRetentionService(db, RetentionConfig{IncomingEventsTTL: 24 * time.Hour, ModerationActionsTTL: 24 * time.Hour})
			_, err = svc.CleanNow(ctx)
			s.Require().NoError(err)

			_, err = events.ByIdempotencyKey(ctx, oldEvent)
			s.Require().Error(err, "expired event is deleted")
			_, err = events.ByIdempotencyKey(ctx, freshEvent)
			s.Require().NoError(err)

			got, err := actions.ByEventID(ctx, oldAction)
			s.Require().NoError(err)
			s.Empty(got, "expired action is deleted")
			got, err = actions.ByEventID(ctx, freshAction)
			s.Require().NoError(err)
			s.Len(got, 1)

			rows, err := reports.GetByMessage(ctx, oldMsg, 1)
			s.Require().NoError(err)
			s.Empty(rows, "expired report is deleted")
			rows, err = reports.GetByMessage(ctx, freshMsg, 1)
			s.Require().NoError(err)
			s.Len(rows, 1)
		})
	}
}
