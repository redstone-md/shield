package storage

import (
	"fmt"
	"time"

	"github.com/umputun/tg-spam/app/feedback"
)

func (s *StorageTestSuite) TestCandidates_CreateAndGet() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Create uses LastInsertId, unsupported by pgx")
			}
			store, err := NewCandidateStorage(ctx, db)
			s.Require().NoError(err)

			entry := feedback.CandidateEntry{
				Type:   feedback.CandidateStopPhrase,
				Value:  fmt.Sprintf("buy now %d", time.Now().UnixNano()),
				Source: "incident",
				Score:  1.5,
				Status: feedback.CandidatePending,
			}

			created, err := store.Create(ctx, entry)
			s.Require().NoError(err)
			s.Positive(created.ID)
			s.Equal(feedback.CandidatePending, created.Status)

			got, err := store.GetByID(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(created.Value, got.Value)
		})
	}
}

func (s *StorageTestSuite) TestCandidates_ListWithFilter() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewCandidateStorage(ctx, db)
			s.Require().NoError(err)

			val := fmt.Sprintf("token-%d", time.Now().UnixNano())
			_, err = store.Create(ctx, feedback.CandidateEntry{Type: feedback.CandidateStopPhrase, Value: val, Source: "incident", Status: feedback.CandidatePending})
			s.Require().NoError(err)
			_, err = store.Create(ctx, feedback.CandidateEntry{Type: feedback.CandidateRegex, Value: val + "-rx", Source: "detected_spam", Status: feedback.CandidatePending})
			s.Require().NoError(err)

			pending, err := store.List(ctx, feedback.CandidateFilter{Status: feedback.CandidatePending, Limit: 10})
			s.Require().NoError(err)
			s.GreaterOrEqual(len(pending), 2)

			phrases, err := store.List(ctx, feedback.CandidateFilter{Type: feedback.CandidateStopPhrase, Limit: 10})
			s.Require().NoError(err)
			s.GreaterOrEqual(len(phrases), 1)
		})
	}
}

func (s *StorageTestSuite) TestCandidates_UpdateStatus() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Create uses LastInsertId, unsupported by pgx")
			}
			store, err := NewCandidateStorage(ctx, db)
			s.Require().NoError(err)

			val := fmt.Sprintf("approve-me-%d", time.Now().UnixNano())
			created, err := store.Create(ctx, feedback.CandidateEntry{Type: feedback.CandidateStopPhrase, Value: val, Source: "incident", Status: feedback.CandidatePending})
			s.Require().NoError(err)

			err = store.UpdateStatus(ctx, created.ID, feedback.CandidateApproved, "admin", "looks good")
			s.Require().NoError(err)

			got, err := store.GetByID(ctx, created.ID)
			s.Require().NoError(err)
			s.Equal(feedback.CandidateApproved, got.Status)
			s.Equal("admin", got.ReviewedBy)
		})
	}
}
