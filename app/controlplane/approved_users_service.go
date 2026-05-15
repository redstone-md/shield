package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/umputun/tg-spam/lib/approved"
)

type ApprovedUsersStore interface {
	Read(ctx context.Context) ([]approved.UserInfo, error)
	Write(ctx context.Context, au approved.UserInfo) error
	Delete(ctx context.Context, id string) error
}

type ApprovedUsersDetector interface {
	AddApprovedUser(user approved.UserInfo) error
	RemoveApprovedUser(id string) error
	ApprovedUsers() []approved.UserInfo
}

type ApprovedUsersService struct {
	store    ApprovedUsersStore
	detector ApprovedUsersDetector
	mu       sync.RWMutex
	onChange []func()
}

func NewApprovedUsersService(store ApprovedUsersStore, detector ApprovedUsersDetector) *ApprovedUsersService {
	return &ApprovedUsersService{store: store, detector: detector}
}

func (s *ApprovedUsersService) List(ctx context.Context, _ string) ([]approved.UserInfo, error) {
	if s.store == nil {
		return nil, fmt.Errorf("approved users store is nil")
	}
	users, err := s.store.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read approved users: %w", err)
	}
	return users, nil
}

func (s *ApprovedUsersService) Add(_ context.Context, _ string, user approved.UserInfo) error {
	if user.UserID == "" {
		return fmt.Errorf("user id is required")
	}
	if err := s.detector.AddApprovedUser(user); err != nil {
		return fmt.Errorf("failed to add approved user %s: %w", user.UserID, err)
	}
	log.Printf("[INFO] approved user added: %s (%s)", user.UserName, user.UserID)
	s.notify()
	return nil
}

func (s *ApprovedUsersService) Remove(_ context.Context, _, id string) error {
	if id == "" {
		return fmt.Errorf("user id is required")
	}
	if err := s.detector.RemoveApprovedUser(id); err != nil {
		return fmt.Errorf("failed to remove approved user %s: %w", id, err)
	}
	log.Printf("[INFO] approved user removed: %s", id)
	s.notify()
	return nil
}

func (s *ApprovedUsersService) OnChange(fn func()) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

func (s *ApprovedUsersService) notify() {
	s.mu.RLock()
	subs := make([]func(), len(s.onChange))
	copy(subs, s.onChange)
	s.mu.RUnlock()

	for _, fn := range subs {
		fn()
	}
}
