package slowpath

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePromptRegistrySetAndGet(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)

	err := reg.Set(PromptEntry{
		Provider:     "openai",
		Version:      "v1",
		SystemPrompt: "prompt1",
		Active:       true,
	})
	require.NoError(t, err)

	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "prompt1", entry.SystemPrompt)
	assert.Equal(t, "v1", entry.Version)
	assert.True(t, entry.Active)
}

func TestFilePromptRegistryActiveReturnsLatest(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)

	require.NoError(t, reg.Set(PromptEntry{
		Provider: "openai", Version: "v1", SystemPrompt: "old", Active: true,
	}))
	require.NoError(t, reg.Set(PromptEntry{
		Provider: "openai", Version: "v2", SystemPrompt: "new", Active: true,
	}))

	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "v2", entry.Version)
	assert.Equal(t, "new", entry.SystemPrompt)
}

func TestFilePromptRegistryGetByVersion(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)

	require.NoError(t, reg.Set(PromptEntry{
		Provider: "openai", Version: "v1", SystemPrompt: "old", Active: false,
	}))
	require.NoError(t, reg.Set(PromptEntry{
		Provider: "openai", Version: "v2", SystemPrompt: "new", Active: true,
	}))

	entry, err := reg.Get("openai", "v1")
	require.NoError(t, err)
	assert.Equal(t, "old", entry.SystemPrompt)
	assert.False(t, entry.Active)
}

func TestFilePromptRegistryGetNotFound(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)

	_, err := reg.Get("openai", "v99")
	require.Error(t, err)
	assert.Error(t, err)
}

func TestFilePromptRegistryActiveDefaultWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)

	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "1", entry.Version)
	assert.Equal(t, defaultSystemPrompt, entry.SystemPrompt)
	assert.True(t, entry.Active)
}

func TestFilePromptRegistryLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	entries := []PromptEntry{
		{Provider: "openai", Version: "v1", SystemPrompt: "loaded", Active: true},
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prompts-openai.json"), data, 0o644))

	reg := NewFilePromptRegistry(dir)
	require.NoError(t, reg.Load())

	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "loaded", entry.SystemPrompt)
}

func TestFilePromptRegistryEmptyDir(t *testing.T) {
	dir := t.TempDir()
	reg := NewFilePromptRegistry(dir)
	require.NoError(t, reg.Load())

	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "1", entry.Version)
}

func TestInMemoryPromptRegistry(t *testing.T) {
	reg := NewInMemoryPromptRegistry()
	reg.Set(PromptEntry{Provider: "test", Version: "v1", SystemPrompt: "p1", Active: true})
	reg.Set(PromptEntry{Provider: "test", Version: "v2", SystemPrompt: "p2", Active: true})

	entry, err := reg.Active("test")
	require.NoError(t, err)
	assert.Equal(t, "p2", entry.SystemPrompt)

	old, err := reg.Get("test", "v1")
	require.NoError(t, err)
	assert.Equal(t, "p1", old.SystemPrompt)
}

func TestInMemoryPromptRegistryNotFound(t *testing.T) {
	reg := NewInMemoryPromptRegistry()
	_, err := reg.Active("missing")
	require.Error(t, err)

	_, err = reg.Get("missing", "v1")
	assert.Error(t, err)
}
