package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/umputun/tg-spam/app/storage"
)

type TenantStore interface {
	Get(ctx context.Context, id string) (storage.TenantRecord, error)
	Add(ctx context.Context, rec storage.TenantRecord) error
	UpdateStatus(ctx context.Context, id, status string) error
}

type TenantService struct {
	store    TenantStore
	mu       sync.RWMutex
	onChange []func(tenantID string)
}

func NewTenantService(store TenantStore) *TenantService {
	return &TenantService{store: store}
}

func (s *TenantService) Suspend(ctx context.Context, tenantID string) error {
	if err := s.updateStatus(ctx, tenantID, "suspended"); err != nil {
		return err
	}
	log.Printf("[INFO] tenant %s suspended", tenantID)
	s.notify(tenantID)
	return nil
}

func (s *TenantService) Resume(ctx context.Context, tenantID string) error {
	if err := s.updateStatus(ctx, tenantID, "active"); err != nil {
		return err
	}
	log.Printf("[INFO] tenant %s resumed", tenantID)
	s.notify(tenantID)
	return nil
}

func (s *TenantService) SoftDelete(ctx context.Context, tenantID string) error {
	if err := s.updateStatus(ctx, tenantID, "deleted"); err != nil {
		return err
	}
	log.Printf("[INFO] tenant %s soft-deleted", tenantID)
	s.notify(tenantID)
	return nil
}

func (s *TenantService) GetStatus(ctx context.Context, tenantID string) (string, error) {
	rec, err := s.store.Get(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("failed to get tenant %s status: %w", tenantID, err)
	}
	return rec.Status, nil
}

func (s *TenantService) OnChange(fn func(tenantID string)) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

func (s *TenantService) updateStatus(ctx context.Context, tenantID, status string) error {
	if s.store == nil {
		return fmt.Errorf("tenant store is nil")
	}
	if tenantID == "" {
		return fmt.Errorf("tenant id is required")
	}
	rec, err := s.store.Get(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tenant %s not found: %w", tenantID, err)
	}
	if rec.Status == status {
		return nil
	}
	return s.store.UpdateStatus(ctx, tenantID, status)
}

func (s *TenantService) notify(tenantID string) {
	s.mu.RLock()
	subs := make([]func(string), len(s.onChange))
	copy(subs, s.onChange)
	s.mu.RUnlock()

	for _, fn := range subs {
		fn(tenantID)
	}
}
