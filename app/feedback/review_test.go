package feedback

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCandidateStore struct {
	entries []CandidateEntry
	nextID  int64
}

func (m *mockCandidateStore) Create(_ context.Context, entry CandidateEntry) (CandidateEntry, error) {
	m.nextID++
	entry.ID = m.nextID
	if entry.Status == "" {
		entry.Status = CandidatePending
	}
	m.entries = append(m.entries, entry)
	return entry, nil
}

func (m *mockCandidateStore) GetByID(_ context.Context, id int64) (CandidateEntry, error) {
	for _, e := range m.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return CandidateEntry{}, ErrNotFound
}

func (m *mockCandidateStore) List(_ context.Context, filter CandidateFilter) ([]CandidateEntry, error) {
	var res []CandidateEntry
	for _, e := range m.entries {
		if filter.Status != "" && e.Status != filter.Status {
			continue
		}
		if filter.Type != "" && e.Type != filter.Type {
			continue
		}
		if filter.Source != "" && e.Source != filter.Source {
			continue
		}
		res = append(res, e)
	}
	if filter.Limit > 0 && len(res) > filter.Limit {
		res = res[:filter.Limit]
	}
	return res, nil
}

func (m *mockCandidateStore) UpdateStatus(_ context.Context, id int64, status CandidateStatus, reviewedBy, _ string) error {
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Status = status
			m.entries[i].ReviewedBy = reviewedBy
			return nil
		}
	}
	return ErrNotFound
}

type mockDictAdder struct {
	added []string
}

func (m *mockDictAdder) AddStopPhrase(_ context.Context, phrase string) error {
	m.added = append(m.added, phrase)
	return nil
}

func TestReviewService_GenerateFromIncident(t *testing.T) {
	store := &mockCandidateStore{}
	svc := NewReviewService(store, nil, nil)

	candidates, err := svc.GenerateFromIncident(context.Background(), 1, "buy cheap stuff now")
	require.NoError(t, err)
	assert.NotEmpty(t, candidates)

	for _, c := range candidates {
		assert.Equal(t, "incident", c.Source)
		assert.Equal(t, int64(1), c.SourceID)
		assert.Equal(t, CandidatePending, c.Status)
	}
}

func TestReviewService_GenerateFromIncident_Empty(t *testing.T) {
	store := &mockCandidateStore{}
	svc := NewReviewService(store, nil, nil)

	candidates, err := svc.GenerateFromIncident(context.Background(), 1, "")
	require.NoError(t, err)
	assert.Nil(t, candidates)
}

func TestReviewService_Approve(t *testing.T) {
	store := &mockCandidateStore{}
	dict := &mockDictAdder{}
	svc := NewReviewService(store, dict, nil)

	created, err := store.Create(context.Background(), CandidateEntry{
		Type:   CandidateStopPhrase,
		Value:  "cheap stuff",
		Source: "incident",
	})
	require.NoError(t, err)

	err = svc.Approve(context.Background(), created.ID, "admin")
	require.NoError(t, err)

	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, CandidateApproved, got.Status)
	assert.Equal(t, "admin", got.ReviewedBy)
	assert.Contains(t, dict.added, "cheap stuff")
}

func TestReviewService_Reject(t *testing.T) {
	store := &mockCandidateStore{}
	svc := NewReviewService(store, nil, nil)

	created, err := store.Create(context.Background(), CandidateEntry{
		Type:   CandidateStopPhrase,
		Value:  "bad phrase",
		Source: "incident",
	})
	require.NoError(t, err)

	err = svc.Reject(context.Background(), created.ID, "admin")
	require.NoError(t, err)

	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, CandidateRejected, got.Status)
}

func TestReviewService_ListPending(t *testing.T) {
	store := &mockCandidateStore{}
	svc := NewReviewService(store, nil, nil)

	_, err := store.Create(context.Background(), CandidateEntry{Value: "a", Status: CandidatePending})
	require.NoError(t, err)
	_, err = store.Create(context.Background(), CandidateEntry{Value: "b", Status: CandidatePending})
	require.NoError(t, err)
	_, err = store.Create(context.Background(), CandidateEntry{Value: "c", Status: CandidateRejected})
	require.NoError(t, err)

	pending, err := svc.ListPending(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Len(t, pending, 2)
}
