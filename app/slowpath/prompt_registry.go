package slowpath

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type FilePromptRegistry struct {
	mu      sync.RWMutex
	dir     string
	entries map[string][]PromptEntry
}

func NewFilePromptRegistry(dir string) *FilePromptRegistry {
	return &FilePromptRegistry{
		dir:     dir,
		entries: make(map[string][]PromptEntry),
	}
}

func (r *FilePromptRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := r.loadFromDir()
	if err != nil {
		return err
	}
	r.entries = entries
	return nil
}

func (r *FilePromptRegistry) Active(provider string) (*PromptEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, ok := r.entries[provider]
	if !ok || len(entries) == 0 {
		return r.defaultEntry(provider), nil
	}

	for i := range entries {
		if entries[i].Active {
			return &entries[i], nil
		}
	}

	return r.defaultEntry(provider), nil
}

func (r *FilePromptRegistry) Get(provider string, version string) (*PromptEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, ok := r.entries[provider]
	if !ok {
		return nil, fmt.Errorf("no prompts for provider %s", provider)
	}

	for i := range entries {
		if entries[i].Version == version {
			return &entries[i], nil
		}
	}

	return nil, fmt.Errorf("prompt version %s not found for provider %s", version, provider)
}

func (r *FilePromptRegistry) Set(entry PromptEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	provider := entry.Provider
	entries := r.entries[provider]

	if entry.Active {
		for i := range entries {
			entries[i].Active = false
		}
	}

	r.entries[provider] = append(entries, entry)
	sort.Slice(r.entries[provider], func(i, j int) bool {
		return r.entries[provider][i].CreatedAt.Before(r.entries[provider][j].CreatedAt)
	})

	return r.saveToDir()
}

func (r *FilePromptRegistry) defaultEntry(provider string) *PromptEntry {
	return &PromptEntry{
		Version:      "1",
		Provider:     provider,
		SystemPrompt: defaultSystemPrompt,
		Active:       true,
		CreatedAt:    time.Now(),
	}
}

func (r *FilePromptRegistry) loadFromDir() (map[string][]PromptEntry, error) {
	result := make(map[string][]PromptEntry)

	if r.dir == "" {
		return result, nil
	}

	matches, err := filepath.Glob(filepath.Join(r.dir, "prompts-*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob prompts: %w", err)
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var entries []PromptEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			continue
		}

		for _, e := range entries {
			result[e.Provider] = append(result[e.Provider], e)
		}
	}

	return result, nil
}

func (r *FilePromptRegistry) saveToDir() error {
	if r.dir == "" {
		return nil
	}

	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir prompts: %w", err)
	}

	for provider, entries := range r.entries {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal prompts: %w", err)
		}

		path := filepath.Join(r.dir, "prompts-"+provider+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write prompts: %w", err)
		}
	}

	return nil
}

type InMemoryPromptRegistry struct {
	mu      sync.RWMutex
	entries map[string][]PromptEntry
}

func NewInMemoryPromptRegistry() *InMemoryPromptRegistry {
	return &InMemoryPromptRegistry{entries: make(map[string][]PromptEntry)}
}

func (r *InMemoryPromptRegistry) Set(entry PromptEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[entry.Provider] = append(r.entries[entry.Provider], entry)
}

func (r *InMemoryPromptRegistry) Active(provider string) (*PromptEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := r.entries[provider]
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Active {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("no active prompt for %s", provider)
}

func (r *InMemoryPromptRegistry) Get(provider string, version string) (*PromptEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries[provider] {
		if e.Version == version {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("prompt %s/%s not found", provider, version)
}
