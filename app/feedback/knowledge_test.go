package feedback

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockKnowledgeStore struct {
	snapshots map[int64]KnowledgeSnapshot
	nextID    int64
}

func newMockKnowledgeStore() *mockKnowledgeStore {
	return &mockKnowledgeStore{snapshots: make(map[int64]KnowledgeSnapshot), nextID: 1}
}

func (m *mockKnowledgeStore) Create(_ context.Context, snap KnowledgeSnapshot) (KnowledgeSnapshot, error) {
	snap.ID = m.nextID
	m.nextID++
	m.snapshots[snap.ID] = snap
	return snap, nil
}

func (m *mockKnowledgeStore) GetByID(_ context.Context, id int64) (KnowledgeSnapshot, error) {
	snap, ok := m.snapshots[id]
	if !ok {
		return KnowledgeSnapshot{}, ErrNotFound
	}
	return snap, nil
}

func (m *mockKnowledgeStore) List(_ context.Context, limit, offset int) ([]KnowledgeSnapshot, error) {
	res := make([]KnowledgeSnapshot, 0, len(m.snapshots))
	for _, s := range m.snapshots {
		res = append(res, s)
	}
	return res, nil
}

type mockStopPhraseRestorer struct {
	phrases []string
	added   []string
	deleted bool
}

func (m *mockStopPhraseRestorer) ReadStopPhrases(_ context.Context) ([]string, error) {
	return m.phrases, nil
}

func (m *mockStopPhraseRestorer) DeleteStopPhrases(_ context.Context) error {
	m.deleted = true
	m.phrases = nil
	return nil
}

func (m *mockStopPhraseRestorer) AddStopPhrase(_ context.Context, phrase string) error {
	m.added = append(m.added, phrase)
	m.phrases = append(m.phrases, phrase)
	return nil
}

type mockDictReader struct {
	phrases []string
	count   int
}

func (m *mockDictReader) ReadStopPhrases(_ context.Context) ([]string, error) {
	return m.phrases, nil
}

func (m *mockDictReader) ReadIgnoredWords(_ context.Context) error { return nil }

func (m *mockDictReader) CountEntries(_ context.Context) (int, error) {
	return m.count, nil
}

func TestKnowledgeService_Snapshot(t *testing.T) {
	store := newMockKnowledgeStore()
	dict := &mockDictReader{phrases: []string{"spam1", "spam2"}, count: 5}
	svc := NewKnowledgeService(store, dict, nil, nil)

	snap, err := svc.Snapshot(context.Background(), "test-user")
	require.NoError(t, err)
	assert.Equal(t, int64(1), snap.ID)
	assert.Equal(t, []string{"spam1", "spam2"}, snap.Data.StopPhrases)
}

func TestKnowledgeService_GetSnapshot(t *testing.T) {
	store := newMockKnowledgeStore()
	data := KnowledgeData{StopPhrases: []string{"a", "b"}, CreatedAt: time.Now().Format(time.RFC3339)}
	jsonBytes, _ := json.Marshal(data)
	store.snapshots[1] = KnowledgeSnapshot{ID: 1, DataJSON: string(jsonBytes)}

	svc := NewKnowledgeService(store, nil, nil, nil)
	snap, err := svc.GetSnapshot(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, snap.Data.StopPhrases)
}

func TestKnowledgeService_Rollback(t *testing.T) {
	store := newMockKnowledgeStore()
	data := KnowledgeData{StopPhrases: []string{"phrase1", "phrase2", "phrase3"}}
	jsonBytes, _ := json.Marshal(data)
	store.snapshots[5] = KnowledgeSnapshot{ID: 5, DataJSON: string(jsonBytes), Data: data}

	restorer := &mockStopPhraseRestorer{phrases: []string{"old"}}
	svc := NewKnowledgeService(store, nil, nil, restorer)

	snap, err := svc.Rollback(context.Background(), 5, "admin")
	require.NoError(t, err)
	assert.Equal(t, []string{"phrase1", "phrase2", "phrase3"}, snap.Data.StopPhrases)
	assert.True(t, restorer.deleted)
	assert.Equal(t, []string{"phrase1", "phrase2", "phrase3"}, restorer.added)
}

func TestKnowledgeService_Rollback_NoRestorer(t *testing.T) {
	store := newMockKnowledgeStore()
	svc := NewKnowledgeService(store, nil, nil, nil)

	_, err := svc.Rollback(context.Background(), 1, "admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no restorer configured")
}

func TestKnowledgeService_Rollback_SnapshotNotFound(t *testing.T) {
	store := newMockKnowledgeStore()
	restorer := &mockStopPhraseRestorer{}
	svc := NewKnowledgeService(store, nil, nil, restorer)

	_, err := svc.Rollback(context.Background(), 999, "admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get snapshot for rollback")
}
