package controlplane

import (
	"context"
	"log"
)

type QuotaChecker interface {
	Check(ctx context.Context, tenantID, limitType string) (bool, error)
	Increment(ctx context.Context, tenantID, limitType string) error
}

type QuotaService struct{}

func NewQuotaService() *QuotaService {
	return &QuotaService{}
}

func (s *QuotaService) Check(_ context.Context, tenantID, limitType string) (bool, error) {
	log.Printf("[DEBUG] quota check: tenant=%s type=%s (stub, always allowed)", tenantID, limitType)
	return true, nil
}

func (s *QuotaService) Increment(_ context.Context, tenantID, limitType string) error {
	log.Printf("[DEBUG] quota increment: tenant=%s type=%s (stub, no-op)", tenantID, limitType)
	return nil
}
