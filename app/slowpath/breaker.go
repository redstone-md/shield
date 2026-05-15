package slowpath

import (
	"context"
	"fmt"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
)

type BreakerConfig struct {
	MaxRequests    uint32
	Interval       time.Duration
	Timeout        time.Duration
	FailuresToTrip uint32
}

func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		MaxRequests:    3,
		Interval:       60 * time.Second,
		Timeout:        30 * time.Second,
		FailuresToTrip: 5,
	}
}

type ProviderBreaker struct {
	name    string
	breaker *gobreaker.CircuitBreaker[*ProviderResult]
}

func NewProviderBreaker(name string, cfg BreakerConfig) *ProviderBreaker {
	if cfg.Interval == 0 {
		cfg = DefaultBreakerConfig()
	}
	cb := gobreaker.NewCircuitBreaker[*ProviderResult](gobreaker.Settings{
		Name:        name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.FailuresToTrip
		},
	})
	return &ProviderBreaker{name: name, breaker: cb}
}

func (b *ProviderBreaker) Execute(
	ctx context.Context, fn func(ctx context.Context) (*ProviderResult, error),
) (*ProviderResult, error) {
	result, err := b.breaker.Execute(func() (*ProviderResult, error) {
		return fn(ctx)
	})
	if err != nil {
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			return nil, fmt.Errorf("circuit breaker %s open: %w", b.name, err)
		}
		return nil, err
	}
	return result, nil
}

func (b *ProviderBreaker) State() gobreaker.State {
	return b.breaker.State()
}
