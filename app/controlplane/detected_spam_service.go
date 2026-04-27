package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/umputun/tg-spam/app/storage"
)

type DetectedSpamStore interface {
	Read(ctx context.Context) ([]storage.DetectedSpamInfo, error)
	FindByUserID(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error)
	SetAddedToSamplesFlag(ctx context.Context, id int64) error
}

type DetectedSpamService struct {
	store    DetectedSpamStore
	mu       sync.RWMutex
	onChange []func()
}

func NewDetectedSpamService(store DetectedSpamStore) *DetectedSpamService {
	return &DetectedSpamService{store: store}
}

func (s *DetectedSpamService) Read(ctx context.Context, _ string) ([]storage.DetectedSpamInfo, error) {
	if s.store == nil {
		return nil, fmt.Errorf("detected spam store is nil")
	}
	entries, err := s.store.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read detected spam: %w", err)
	}
	return entries, nil
}

func (s *DetectedSpamService) FindByUserID(ctx context.Context, _ string, userID int64) (*storage.DetectedSpamInfo, error) {
	if s.store == nil {
		return nil, fmt.Errorf("detected spam store is nil")
	}
	entry, err := s.store.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find detected spam for user %d: %w", userID, err)
	}
	return entry, nil
}

func (s *DetectedSpamService) SetAddedToSamplesFlag(ctx context.Context, _ string, id int64) error {
	if s.store == nil {
		return fmt.Errorf("detected spam store is nil")
	}
	if err := s.store.SetAddedToSamplesFlag(ctx, id); err != nil {
		return fmt.Errorf("failed to mark detected spam %d as added: %w", id, err)
	}
	log.Printf("[INFO] detected spam entry %d marked as added to samples", id)
	s.notify()
	return nil
}

func (s *DetectedSpamService) OnChange(fn func()) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

func (s *DetectedSpamService) notify() {
	s.mu.RLock()
	subs := make([]func(), len(s.onChange))
	copy(subs, s.onChange)
	s.mu.RUnlock()

	for _, fn := range subs {
		fn()
	}
}
