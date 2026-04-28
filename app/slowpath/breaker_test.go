package slowpath

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProviderBreakerSuccess(t *testing.T) {
	b := NewProviderBreaker("test", BreakerConfig{
		MaxRequests:    3,
		Interval:       time.Second,
		Timeout:        time.Second,
		FailuresToTrip: 3,
	})
	result, err := b.Execute(context.Background(), func(ctx context.Context) (*ProviderResult, error) {
		return &ProviderResult{Spam: false, Provider: "test"}, nil
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test", result.Provider)
}

func TestProviderBreakerOpensOnFailures(t *testing.T) {
	b := NewProviderBreaker("test", BreakerConfig{
		MaxRequests:    1,
		Interval:       time.Second,
		Timeout:        2 * time.Second,
		FailuresToTrip: 2,
	})

	failFn := func(ctx context.Context) (*ProviderResult, error) {
		return nil, errors.New("fail")
	}

	_, _ = b.Execute(context.Background(), failFn)
	_, _ = b.Execute(context.Background(), failFn)

	_, err := b.Execute(context.Background(), failFn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
}

func TestProviderBreakerDefaultConfig(t *testing.T) {
	b := NewProviderBreaker("test", BreakerConfig{})
	result, err := b.Execute(context.Background(), func(ctx context.Context) (*ProviderResult, error) {
		return &ProviderResult{Spam: true, Provider: "x"}, nil
	})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
}

func TestProviderBreakerRecoversAfterTimeout(t *testing.T) {
	b := NewProviderBreaker("test", BreakerConfig{
		MaxRequests:    1,
		Interval:       100 * time.Millisecond,
		Timeout:        200 * time.Millisecond,
		FailuresToTrip: 1,
	})

	failFn := func(ctx context.Context) (*ProviderResult, error) {
		return nil, errors.New("fail")
	}

	_, _ = b.Execute(context.Background(), failFn)
	time.Sleep(250 * time.Millisecond)

	result, err := b.Execute(context.Background(), func(ctx context.Context) (*ProviderResult, error) {
		return &ProviderResult{Spam: false, Provider: "recovered"}, nil
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
