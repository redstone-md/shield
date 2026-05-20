package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func okSend(tbapi.Chattable) (tbapi.Message, error) {
	return tbapi.Message{MessageID: 123}, nil
}

func TestUserReports_checkReportRateLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("rate limit exceeded", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
				return 10, nil
			},
		}

		rep := &userReports{
			ReportConfig: ReportConfig{
				Storage:    mockReports,
				RateLimit:  10,
				RatePeriod: 1 * time.Hour,
			},
		}

		exceeded, err := rep.checkReportRateLimit(ctx, 123)
		require.NoError(t, err)
		assert.True(t, exceeded, "rate limit should be exceeded")
		require.Len(t, mockReports.GetReporterCountSinceCalls(), 1)
	})

	t.Run("rate limit not exceeded", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
				return 5, nil
			},
		}

		rep := &userReports{
			ReportConfig: ReportConfig{
				Storage:    mockReports,
				RateLimit:  10,
				RatePeriod: 1 * time.Hour,
			},
		}

		exceeded, err := rep.checkReportRateLimit(ctx, 123)
		require.NoError(t, err)
		assert.False(t, exceeded, "rate limit should not be exceeded")
		require.Len(t, mockReports.GetReporterCountSinceCalls(), 1)
	})

	t.Run("rate limiting disabled", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
				return 100, nil
			},
		}

		rep := &userReports{
			ReportConfig: ReportConfig{
				Storage:    mockReports,
				RateLimit:  0,
				RatePeriod: 1 * time.Hour,
			},
		}

		exceeded, err := rep.checkReportRateLimit(ctx, 123)
		require.NoError(t, err)
		assert.False(t, exceeded, "rate limit should be disabled")
		require.Empty(t, mockReports.GetReporterCountSinceCalls(), "should not call GetReporterCountSince when disabled")
	})

	t.Run("reports storage not initialized", func(t *testing.T) {
		rep := &userReports{
			ReportConfig: ReportConfig{
				Storage:    nil,
				RateLimit:  10,
				RatePeriod: 1 * time.Hour,
			},
		}

		exceeded, err := rep.checkReportRateLimit(ctx, 123)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reports storage not initialized")
		assert.False(t, exceeded)
	})

	t.Run("database error", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
				return 0, fmt.Errorf("database error")
			},
		}

		rep := &userReports{
			ReportConfig: ReportConfig{
				Storage:    mockReports,
				RateLimit:  10,
				RatePeriod: 1 * time.Hour,
			},
		}

		exceeded, err := rep.checkReportRateLimit(ctx, 123)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reporter count")
		assert.False(t, exceeded)
	})
}
