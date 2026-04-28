package slowpath

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePromptRegistryActive(t *testing.T) {
	dir := t.TempDir()
	entries := []PromptEntry{
		{Provider: "openai", Version: "v1", SystemPrompt: "prompt1", Active: true},
		{Provider: "openai", Version: "v2", SystemPrompt: "prompt2", Active: false},
		{Provider: "gemini", Version: "v1", SystemPrompt: "gemini1", Active: true},
	}
	writeTestPrompts(t, dir, entries)

	reg := NewFilePromptRegistry(filepath.Join(dir, "prompts.json"))
	entry, err := reg.Active("openai")
	require.NoError(t, err)
	assert.Equal(t, "v1", entry.Version)
	assert.Equal(t, "prompt1", entry.SystemPrompt)

	geminiEntry, err := reg.Active("gemini")
	require.NoError(t, err)
	assert.Equal(t, "v1", geminiEntry.Version)
}

func TestFilePromptRegistryGet(t *testing.T) {
	dir := t.TempDir()
	entries := []PromptEntry{
		{Provider: "openai", Version: "v2", SystemPrompt: "new prompt", Active: false},
	}
	writeTestPrompts(t, dir, entries)

	reg := NewFilePromptRegistry(filepath.Join(dir, "prompts.json"))
	entry, err := reg.Get("openai", "v2")
	require.NoError(t, err)
	assert.Equal(t, "new prompt", entry.SystemPrompt)
}

func TestFilePromptRegistryNoActive(t *testing.T) {
	dir := t.TempDir()
	entries := []PromptEntry{
		{Provider: "openai", Version: "v1", SystemPrompt: "old", Active: false},
	}
	writeTestPrompts(t, dir, entries)

	reg := NewFilePromptRegistry(filepath.Join(dir, "prompts.json"))
	_, err := reg.Active("openai")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active prompt")
}

func TestFilePromptRegistryNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTestPrompts(t, dir, nil)

	reg := NewFilePromptRegistry(filepath.Join(dir, "prompts.json"))
	_, err := reg.Get("unknown", "v99")
	assert.Error(t, err)
}

func TestFilePromptRegistryMissingFile(t *testing.T) {
	reg := NewFilePromptRegistry("/nonexistent/prompts.json")
	_, err := reg.Active("openai")
	assert.Error(t, err)
}

func TestFilePromptRegistryEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0644))

	reg := NewFilePromptRegistry(path)
	_, err := reg.Active("openai")
	assert.Error(t, err)
}

func writeTestPrompts(t *testing.T, dir string, entries []PromptEntry) {
	t.Helper()
	path := filepath.Join(dir, "prompts.json")
	data, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}
