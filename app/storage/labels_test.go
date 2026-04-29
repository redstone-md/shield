package storage

import (
	"fmt"
	"time"

	"github.com/umputun/tg-spam/app/feedback"
)

func (s *StorageTestSuite) TestLabels_CreateAndGet() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Create uses LastInsertId, unsupported by pgx")
			}
			store, err := NewLabelStorage(ctx, db)
			s.Require().NoError(err)

			entry := feedback.LabelEntry{
				DetectedSpamID: time.Now().UnixNano(),
				IncidentID:     200,
				Label:          feedback.LabelConfirmedSpam,
				LabeledBy:      "admin",
				Comment:        "confirmed spam",
			}

			created, err := store.Create(ctx, entry)
			s.Require().NoError(err)
			s.Assert().True(created.ID > 0)
			s.Assert().Equal(feedback.LabelConfirmedSpam, created.Label)
			s.Assert().Equal("admin", created.LabeledBy)

			got, err := store.GetByID(ctx, created.ID)
			s.Require().NoError(err)
			s.Assert().Equal(created.ID, got.ID)
			s.Assert().Equal(feedback.LabelConfirmedSpam, got.Label)
		})
	}
}

func (s *StorageTestSuite) TestLabels_GetByDetectedSpamID() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewLabelStorage(ctx, db)
			s.Require().NoError(err)

			spamID := time.Now().UnixNano()

			_, err = store.Create(ctx, feedback.LabelEntry{
				DetectedSpamID: spamID,
				Label:          feedback.LabelConfirmedSpam,
				LabeledBy:      "admin1",
			})
			s.Require().NoError(err)

			_, err = store.Create(ctx, feedback.LabelEntry{
				DetectedSpamID: spamID,
				Label:          feedback.LabelFalsePositive,
				LabeledBy:      "admin2",
			})
			s.Require().NoError(err)

			labels, err := store.GetByDetectedSpamID(ctx, spamID)
			s.Require().NoError(err)
			s.Assert().Len(labels, 2)
		})
	}
}

func (s *StorageTestSuite) TestLabels_GetByIncidentID() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewLabelStorage(ctx, db)
			s.Require().NoError(err)

			incID := time.Now().UnixNano()

			_, err = store.Create(ctx, feedback.LabelEntry{
				IncidentID: incID,
				Label:      feedback.LabelPolicyOverride,
				LabeledBy:  "admin",
			})
			s.Require().NoError(err)

			labels, err := store.GetByIncidentID(ctx, incID)
			s.Require().NoError(err)
			s.Assert().Len(labels, 1)
			s.Assert().Equal(feedback.LabelPolicyOverride, labels[0].Label)
		})
	}
}

func (s *StorageTestSuite) TestLabels_ListWithFilter() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewLabelStorage(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelConfirmedSpam, LabeledBy: fmt.Sprintf("a-%d", ts)})
			s.Require().NoError(err)
			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelFalsePositive, LabeledBy: fmt.Sprintf("b-%d", ts)})
			s.Require().NoError(err)
			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelConfirmedSpam, LabeledBy: fmt.Sprintf("c-%d", ts)})
			s.Require().NoError(err)

			spamOnly, err := store.List(ctx, feedback.LabelFilter{Label: feedback.LabelConfirmedSpam, LabeledBy: fmt.Sprintf("a-%d", ts), Limit: 10})
			s.Require().NoError(err)
			s.Assert().Len(spamOnly, 1)

			byUser, err := store.List(ctx, feedback.LabelFilter{LabeledBy: fmt.Sprintf("b-%d", ts), Limit: 10})
			s.Require().NoError(err)
			s.Assert().Len(byUser, 1)
			s.Assert().Equal(feedback.LabelFalsePositive, byUser[0].Label)
		})
	}
}

func (s *StorageTestSuite) TestLabels_Stats() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			store, err := NewLabelStorage(ctx, db)
			s.Require().NoError(err)

			stats, err := store.Stats(ctx)
			s.Require().NoError(err)
			beforeSpam := stats[feedback.LabelConfirmedSpam]
			beforeFP := stats[feedback.LabelFalsePositive]

			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelConfirmedSpam})
			s.Require().NoError(err)
			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelConfirmedSpam})
			s.Require().NoError(err)
			_, err = store.Create(ctx, feedback.LabelEntry{Label: feedback.LabelFalsePositive})
			s.Require().NoError(err)

			stats, err = store.Stats(ctx)
			s.Require().NoError(err)
			s.Assert().Equal(beforeSpam+2, stats[feedback.LabelConfirmedSpam])
			s.Assert().Equal(beforeFP+1, stats[feedback.LabelFalsePositive])
		})
	}
}
