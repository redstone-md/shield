package storage

import (
	"fmt"
	"time"
)

func (s *StorageTestSuite) TestUsageMetering_IncrementAutoCreate() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Increment uses sql.Result, unsupported by pgx")
			}
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			meterType := fmt.Sprintf("auto_create_%d", ts)
			wStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

			err = m.Increment(ctx, meterType, wStart, wEnd)
			s.Require().NoError(err)

			counter, err := m.Get(ctx, meterType, wStart)
			s.Require().NoError(err)
			s.Assert().Equal(1, counter.Count)
			s.Assert().Equal(meterType, counter.MeterType)
		})
	}
}

func (s *StorageTestSuite) TestUsageMetering_IncrementExisting() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Increment uses sql.Result, unsupported by pgx")
			}
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			meterType := fmt.Sprintf("incr_existing_%d", ts)
			wStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

			s.Require().NoError(m.Increment(ctx, meterType, wStart, wEnd))
			s.Require().NoError(m.Increment(ctx, meterType, wStart, wEnd))
			s.Require().NoError(m.Increment(ctx, meterType, wStart, wEnd))

			counter, err := m.Get(ctx, meterType, wStart)
			s.Require().NoError(err)
			s.Assert().Equal(3, counter.Count)
		})
	}
}

func (s *StorageTestSuite) TestUsageMetering_GetMissing() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			wStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			_, err = m.Get(ctx, "nonexistent_meter", wStart)
			s.Assert().Error(err)
		})
	}
}

func (s *StorageTestSuite) TestUsageMetering_GetPresent() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Increment uses sql.Result, unsupported by pgx")
			}
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			meterType := fmt.Sprintf("get_present_%d", ts)
			wStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
			wEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

			s.Require().NoError(m.Increment(ctx, meterType, wStart, wEnd))

			counter, err := m.Get(ctx, meterType, wStart)
			s.Require().NoError(err)
			s.Assert().Equal(meterType, counter.MeterType)
			s.Assert().Equal(1, counter.Count)
			s.Assert().True(counter.WindowStart.Equal(wStart))
			s.Assert().True(counter.WindowEnd.Equal(wEnd))
		})
	}
}

func (s *StorageTestSuite) TestUsageMetering_ListByWindow() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Increment uses sql.Result, unsupported by pgx")
			}
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			mt1 := fmt.Sprintf("list_api_%d", ts)
			mt2 := fmt.Sprintf("list_tok_%d", ts)

			wStart1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			wEnd1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
			wStart2 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
			wEnd2 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

			s.Require().NoError(m.Increment(ctx, mt1, wStart1, wEnd1))
			s.Require().NoError(m.Increment(ctx, mt2, wStart1, wEnd1))
			s.Require().NoError(m.Increment(ctx, mt1, wStart2, wEnd2))

			counters, err := m.ListByWindow(ctx, wStart1, wEnd2)
			s.Require().NoError(err)
			s.Assert().Len(counters, 3)
		})
	}
}

func (s *StorageTestSuite) TestUsageMetering_ResetTenant() {
	ctx := s.T().Context()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			if db.Type() == "postgres" {
				s.T().Skip("Increment uses sql.Result, unsupported by pgx")
			}
			m, err := NewUsageMetering(ctx, db)
			s.Require().NoError(err)

			ts := time.Now().UnixNano()
			meterType := fmt.Sprintf("reset_%d", ts)

			oldStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			oldEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
			newStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			newEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

			s.Require().NoError(m.Increment(ctx, meterType, oldStart, oldEnd))
			s.Require().NoError(m.Increment(ctx, meterType, newStart, newEnd))

			err = m.ResetTenant(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			s.Require().NoError(err)

			counters, err := m.ListByWindow(ctx, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
			s.Require().NoError(err)

			var found []UsageCounter
			for _, c := range counters {
				if c.MeterType == meterType {
					found = append(found, c)
				}
			}
			s.Assert().Len(found, 1)
			s.Assert().True(newStart.Equal(found[0].WindowStart), "window_start mismatch: want %v, got %v", newStart, found[0].WindowStart)
		})
	}
}
