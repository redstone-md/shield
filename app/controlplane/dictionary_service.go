package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/umputun/tg-spam/app/storage"
)

type DictionaryStore interface {
	Add(ctx context.Context, t storage.DictionaryType, data string) error
	Delete(ctx context.Context, id int64) error
	Read(ctx context.Context, t storage.DictionaryType) ([]string, error)
	ReadWithIDs(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error)
	Stats(ctx context.Context) (*storage.DictionaryStats, error)
}

type SampleReloader interface {
	ReloadSamples(ctx context.Context) error
}

type DictionaryService struct {
	store    DictionaryStore
	reloader SampleReloader
	mu       sync.RWMutex
	onChange []func()
}

func NewDictionaryService(store DictionaryStore, reloader SampleReloader) *DictionaryService {
	return &DictionaryService{store: store, reloader: reloader}
}

func (s *DictionaryService) Add(ctx context.Context, _ string, t storage.DictionaryType, data string) error {
	if data == "" {
		return fmt.Errorf("data cannot be empty")
	}
	if err := t.Validate(); err != nil {
		return fmt.Errorf("invalid dictionary type: %w", err)
	}
	if err := s.store.Add(ctx, t, data); err != nil {
		return fmt.Errorf("failed to add %s entry: %w", t, err)
	}
	s.reloadAndNotify(ctx)
	log.Printf("[INFO] dictionary entry added: type=%s, data=%q", t, data)
	return nil
}

func (s *DictionaryService) Delete(ctx context.Context, _ string, id int64) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete dictionary entry %d: %w", id, err)
	}
	s.reloadAndNotify(ctx)
	log.Printf("[INFO] dictionary entry deleted: id=%d", id)
	return nil
}

func (s *DictionaryService) Read(ctx context.Context, _ string, t storage.DictionaryType) ([]string, error) {
	if s.store == nil {
		return nil, fmt.Errorf("dictionary store is nil")
	}
	return s.store.Read(ctx, t)
}

func (s *DictionaryService) ReadWithIDs(ctx context.Context, _ string, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
	if s.store == nil {
		return nil, fmt.Errorf("dictionary store is nil")
	}
	return s.store.ReadWithIDs(ctx, t)
}

func (s *DictionaryService) Stats(ctx context.Context, _ string) (*storage.DictionaryStats, error) {
	if s.store == nil {
		return nil, fmt.Errorf("dictionary store is nil")
	}
	return s.store.Stats(ctx)
}

func (s *DictionaryService) OnChange(fn func()) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

func (s *DictionaryService) reloadAndNotify(ctx context.Context) {
	if s.reloader != nil {
		if err := s.reloader.ReloadSamples(ctx); err != nil {
			log.Printf("[WARN] failed to reload samples after dictionary change: %v", err)
		}
	}
	s.notify()
}

func (s *DictionaryService) notify() {
	s.mu.RLock()
	subs := make([]func(), len(s.onChange))
	copy(subs, s.onChange)
	s.mu.RUnlock()

	for _, fn := range subs {
		fn()
	}
}
