package controlplane

import (
	"context"
	"fmt"
	"log"
)

type QuotaChecker interface {
	Check(ctx context.Context, tenantID, limitType string) (bool, error)
	Increment(ctx context.Context, tenantID, limitType string) error
}

type TenantLimitStore interface {
	Get(ctx context.Context, limitType string) (limitValue int, currentUsage int, err error)
	Increment(ctx context.Context, limitType string) error
	Set(ctx context.Context, limitType string, limitValue int) error
}

type QuotaService struct {
	limits TenantLimitStore
}

func NewQuotaService(limits TenantLimitStore) *QuotaService {
	return &QuotaService{limits: limits}
}

func (s *QuotaService) Check(ctx context.Context, tenantID, limitType string) (bool, error) {
	if s.limits == nil {
		return true, nil
	}
	limitValue, currentUsage, err := s.limits.Get(ctx, limitType)
	if err != nil {
		return true, fmt.Errorf("quota check: %w", err)
	}
	if limitValue == 0 {
		return true, nil
	}
	if currentUsage >= limitValue {
		log.Printf("[INFO] quota exceeded: tenant=%s type=%s usage=%d limit=%d", tenantID, limitType, currentUsage, limitValue)
		return false, nil
	}
	return true, nil
}

func (s *QuotaService) Increment(ctx context.Context, tenantID, limitType string) error {
	if s.limits == nil {
		return nil
	}
	if err := s.limits.Increment(ctx, limitType); err != nil {
		return fmt.Errorf("quota increment: %w", err)
	}
	return nil
}
