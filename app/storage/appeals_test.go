package storage

import (
	"encoding/json"
	"fmt"

	"github.com/umputun/tg-spam/app/audit"
)

func (s *StorageTestSuite) TestAppeals_CreateAndGet() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			incStore, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			apStore, err := NewAppealStorage(ctx, db)
			s.Require().NoError(err)

			inc, err := incStore.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityMedium,
				IdempotencyKey: fmt.Sprintf("appeal-inc-%s", db.Type()),
				ReasonCode:     audit.ReasonRegexMatch,
				SpamUserID:     123,
			})
			s.Require().NoError(err)

			ap := audit.Appeal{
				IncidentID:      inc.ID,
				AppellantUserID: 123,
				AppellantName:   "testuser",
				Status:          audit.AppealNew,
				AppealText:      "I was wrongly banned",
			}
			created, err := apStore.Create(ctx, ap)
			s.Require().NoError(err)
			s.NotZero(created.ID)
			s.NotZero(created.CreatedAt)
			s.Equal(inc.ID, created.IncidentID)
			s.Equal("testuser", created.AppellantName)

			got, err := apStore.Get(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(created.ID, got.ID)
			s.Equal("I was wrongly banned", got.AppealText)
		})
	}
}

func (s *StorageTestSuite) TestAppeals_GetByIncident() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			incStore, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			apStore, err := NewAppealStorage(ctx, db)
			s.Require().NoError(err)

			inc, err := incStore.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("appeal-get-inc-%s", db.Type()),
				ReasonCode:     audit.ReasonStopWord,
				SpamUserID:     456,
			})
			s.Require().NoError(err)

			_, err = apStore.Create(ctx, audit.Appeal{
				IncidentID:      inc.ID,
				AppellantUserID: 456,
				AppellantName:   "user2",
				Status:          audit.AppealNew,
				AppealText:      "not spam",
			})
			s.Require().NoError(err)

			got, err := apStore.GetByIncident(ctx, inc.ID)
			s.Require().NoError(err)
			s.Equal("not spam", got.AppealText)
			s.Equal(inc.ID, got.IncidentID)
		})
	}
}

func (s *StorageTestSuite) TestAppeals_UpdateStatus() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			incStore, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			apStore, err := NewAppealStorage(ctx, db)
			s.Require().NoError(err)

			inc, err := incStore.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("appeal-status-inc-%s", db.Type()),
				ReasonCode:     audit.ReasonSimilarity,
			})
			s.Require().NoError(err)

			ap, err := apStore.Create(ctx, audit.Appeal{
				IncidentID:      inc.ID,
				AppellantUserID: 789,
				AppellantName:   "user3",
				Status:          audit.AppealNew,
				AppealText:      "mistake",
			})
			s.Require().NoError(err)

			err = apStore.UpdateStatus(ctx, ap.ID, audit.AppealAccepted, "admin1", "confirmed false positive")
			s.Require().NoError(err)

			got, err := apStore.Get(ctx, ap.ID)
			s.Require().NoError(err)
			s.Equal(audit.AppealAccepted, got.Status)
			s.Equal("admin1", got.ResolvedBy)
			s.Equal("confirmed false positive", got.ResolutionText)
			s.NotNil(got.ResolvedAt)
		})
	}
}

func (s *StorageTestSuite) TestAppeals_UpdateReplayResult() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			incStore, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			apStore, err := NewAppealStorage(ctx, db)
			s.Require().NoError(err)

			inc, err := incStore.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("appeal-replay-inc-%s", db.Type()),
				ReasonCode:     audit.ReasonCAS,
			})
			s.Require().NoError(err)

			ap, err := apStore.Create(ctx, audit.Appeal{
				IncidentID:      inc.ID,
				AppellantUserID: 999,
				AppellantName:   "user4",
				Status:          audit.AppealNew,
				AppealText:      "replay me",
			})
			s.Require().NoError(err)

			result := audit.ReplayResult{
				DetectionSpam:   false,
				DetectionScore:  0.1,
				PolicyAction:    "allow",
				PolicyReason:    "no spam detected",
				PolicyScore:     0.05,
				ReplayTimestamp: "2026-04-28T12:00:00Z",
			}
			err = apStore.UpdateReplayResult(ctx, ap.ID, result)
			s.Require().NoError(err)

			got, err := apStore.Get(ctx, ap.ID)
			s.Require().NoError(err)
			s.NotEmpty(got.ReplayResult)

			var parsed audit.ReplayResult
			s.Require().NoError(json.Unmarshal([]byte(got.ReplayResult), &parsed))
			s.False(parsed.DetectionSpam)
			s.Equal("allow", parsed.PolicyAction)
		})
	}
}

func (s *StorageTestSuite) TestAppeals_List() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			incStore, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)
			apStore, err := NewAppealStorage(ctx, db)
			s.Require().NoError(err)

			for i := range 3 {
				inc, ierr := incStore.Create(ctx, audit.Incident{
					Source:         audit.SourceAutoMod,
					Status:         audit.IncidentStatusOpen,
					Severity:       audit.SeverityLow,
					IdempotencyKey: fmt.Sprintf("appeal-list-inc-%s-%d", db.Type(), i),
					ReasonCode:     audit.ReasonRegexMatch,
					SpamUserID:     int64(1000 + i),
				})
				s.Require().NoError(ierr)

				status := audit.AppealNew
				if i == 1 {
					status = audit.AppealAccepted
				}
				_, err = apStore.Create(ctx, audit.Appeal{
					IncidentID:      inc.ID,
					AppellantUserID: int64(1000 + i),
					AppellantName:   fmt.Sprintf("listuser%d", i),
					Status:          status,
					AppealText:      fmt.Sprintf("appeal %d", i),
				})
				s.Require().NoError(err)
			}

			all, err := apStore.List(ctx, audit.AppealFilter{Limit: 10})
			s.Require().NoError(err)
			s.GreaterOrEqual(len(all), 3)

			newOnly, err := apStore.List(ctx, audit.AppealFilter{Status: audit.AppealNew, Limit: 10})
			s.Require().NoError(err)
			s.GreaterOrEqual(len(newOnly), 2)
		})
	}
}
