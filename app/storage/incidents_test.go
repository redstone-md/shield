package storage

import (
	"fmt"
	"time"

	"github.com/redstone-md/shield/app/audit"
)

func (s *StorageTestSuite) TestIncidents_CreateAndGet() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			inc := audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityMedium,
				IdempotencyKey: fmt.Sprintf("create-get-%d", time.Now().UnixNano()),
				ReasonCode:     audit.ReasonRegexMatch,
				ReasonText:     "regex matched spam pattern",
				SpamUserID:     12345,
				SpamUserName:   "spammer",
				ChatID:         999,
				MessageText:    "buy cheap stuff now",
			}
			created, err := store.Create(ctx, inc)
			s.Require().NoError(err)
			s.NotZero(created.ID)
			s.Equal(inc.Source, created.Source)
			s.Equal(inc.IdempotencyKey, created.IdempotencyKey)
			s.NotZero(created.CreatedAt)

			got, err := store.Get(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(created.ID, got.ID)
			s.Equal("spammer", got.SpamUserName)
			s.Equal(inc.MessageText, got.MessageText)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_GetByIdempotencyKey() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			key := fmt.Sprintf("idem-key-%d", time.Now().UnixNano())
			inc := audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityHigh,
				IdempotencyKey: key,
				ReasonCode:     audit.ReasonStopWord,
				SpamUserID:     42,
			}
			created, err := store.Create(ctx, inc)
			s.Require().NoError(err)

			got, err := store.GetByIdempotencyKey(ctx, db.TenantID(), key)
			s.Require().NoError(err)
			s.Equal(created.ID, got.ID)
			s.Equal(audit.SeverityHigh, got.Severity)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_UpdateStatus() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			created, err := store.Create(ctx, audit.Incident{
				Source:         audit.SourceUserReport,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("status-test-%d", time.Now().UnixNano()),
				ReasonCode:     audit.ReasonUserReport,
			})
			s.Require().NoError(err)

			err = store.UpdateStatus(ctx, created.ID, audit.IncidentStatusReviewing, "admin1")
			s.Require().NoError(err)

			got, err := store.Get(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(audit.IncidentStatusReviewing, got.Status)
			s.Equal("admin1", got.ResolvedBy)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_UpdateSeverity() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			created, err := store.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("sev-test-%d", time.Now().UnixNano()),
				ReasonCode:     audit.ReasonSimilarity,
			})
			s.Require().NoError(err)

			err = store.UpdateSeverity(ctx, created.ID, audit.SeverityCritical)
			s.Require().NoError(err)

			got, err := store.Get(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(audit.SeverityCritical, got.Severity)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_ListWithFilters() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			prefix := fmt.Sprintf("filter-%d-", time.Now().UnixNano())
			sources := []audit.IncidentSource{audit.SourceAutoMod, audit.SourceUserReport, audit.SourceAdminAction}
			for i, src := range sources {
				_, errCreate := store.Create(ctx, audit.Incident{
					Source:         src,
					Status:         audit.IncidentStatusOpen,
					Severity:       audit.SeverityMedium,
					IdempotencyKey: fmt.Sprintf("%s%d", prefix, i),
					ReasonCode:     audit.ReasonRegexMatch,
				})
				s.Require().NoError(errCreate)
			}

			autoOnly, err := store.List(ctx, audit.IncidentFilter{Source: audit.SourceAutoMod, Limit: 10})
			s.Require().NoError(err)
			s.Require().NotEmpty(autoOnly)
			s.Equal(audit.SourceAutoMod, autoOnly[0].Source)

			userReports, err := store.List(ctx, audit.IncidentFilter{Source: audit.SourceUserReport, Limit: 10})
			s.Require().NoError(err)
			s.Require().NotEmpty(userReports)
			s.Equal(audit.SourceUserReport, userReports[0].Source)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_Pagination() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			prefix := fmt.Sprintf("page-%d-", time.Now().UnixNano())
			for i := range 5 {
				_, errCreate := store.Create(ctx, audit.Incident{
					Source:         audit.SourceAutoMod,
					Status:         audit.IncidentStatusOpen,
					Severity:       audit.SeverityLow,
					IdempotencyKey: fmt.Sprintf("%s%d", prefix, i),
					ReasonCode:     audit.ReasonRegexMatch,
				})
				s.Require().NoError(errCreate)
			}

			page1, err := store.List(ctx, audit.IncidentFilter{Limit: 2, Offset: 0})
			s.Require().NoError(err)
			s.Len(page1, 2)

			page2, err := store.List(ctx, audit.IncidentFilter{Limit: 2, Offset: 2})
			s.Require().NoError(err)
			s.Len(page2, 2)

			s.NotEqual(page1[0].ID, page2[0].ID)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_Comments() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			created, err := store.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityMedium,
				IdempotencyKey: fmt.Sprintf("comment-%d", time.Now().UnixNano()),
				ReasonCode:     audit.ReasonRegexMatch,
			})
			s.Require().NoError(err)

			c1, err := store.AddComment(ctx, audit.IncidentComment{
				IncidentID: created.ID,
				AuthorType: "system",
				AuthorID:   "pipeline",
				Action:     "created",
				Payload:    `{"source":"fast_path"}`,
			})
			s.Require().NoError(err)
			s.NotZero(c1.ID)

			time.Sleep(10 * time.Millisecond)

			c2, err := store.AddComment(ctx, audit.IncidentComment{
				IncidentID: created.ID,
				AuthorType: "admin",
				AuthorID:   "admin1",
				Action:     "reviewed",
				Payload:    "confirmed spam",
			})
			s.Require().NoError(err)
			s.NotZero(c2.ID)

			comments, err := store.ListComments(ctx, created.ID)
			s.Require().NoError(err)
			s.Len(comments, 2)
			s.Equal("created", comments[0].Action)
			s.Equal("reviewed", comments[1].Action)

			s.NotZero(comments[0].CreatedAt)
			s.NotZero(comments[1].CreatedAt)
		})
	}
}

func (s *StorageTestSuite) TestIncidents_DateFilters() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewIncidentStorage(ctx, db)
			s.Require().NoError(err)

			_, err = store.Create(ctx, audit.Incident{
				Source:         audit.SourceAutoMod,
				Status:         audit.IncidentStatusOpen,
				Severity:       audit.SeverityLow,
				IdempotencyKey: fmt.Sprintf("date-%d", time.Now().UnixNano()),
				ReasonCode:     audit.ReasonRegexMatch,
			})
			s.Require().NoError(err)

			future := time.Now().Add(24 * time.Hour)
			past := time.Now().Add(-24 * time.Hour)

			fromFuture, err := store.List(ctx, audit.IncidentFilter{From: future, Limit: 10})
			s.Require().NoError(err)
			s.Empty(fromFuture)

			toPast, err := store.List(ctx, audit.IncidentFilter{To: past, Limit: 10})
			s.Require().NoError(err)
			s.Empty(toPast)

			allRange, err := store.List(ctx, audit.IncidentFilter{From: past, To: future, Limit: 10})
			s.Require().NoError(err)
			s.NotEmpty(allRange)
		})
	}
}
