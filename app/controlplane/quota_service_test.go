package controlplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLimitStore struct {
	limits map[string]struct {
		limit int
		usage int
	}
}

func newMockLimitStore() *mockLimitStore {
	return &mockLimitStore{
		limits: make(map[string]struct {
			limit int
			usage int
		}),
	}
}

func (m *mockLimitStore) Get(_ context.Context, limitType string) (int, int, error) {
	rec, ok := m.limits[limitType]
	if !ok {
		return 0, 0, nil
	}
	return rec.limit, rec.usage, nil
}

func (m *mockLimitStore) Increment(_ context.Context, limitType string) error {
	rec, ok := m.limits[limitType]
	if !ok {
		m.limits[limitType] = struct {
			limit int
			usage int
		}{limit: 0, usage: 1}
		return nil
	}
	m.limits[limitType] = struct {
		limit int
		usage int
	}{limit: rec.limit, usage: rec.usage + 1}
	return nil
}

func (m *mockLimitStore) Set(_ context.Context, limitType string, limitValue int) error {
	rec, ok := m.limits[limitType]
	usage := 0
	if ok {
		usage = rec.usage
	}
	m.limits[limitType] = struct {
		limit int
		usage int
	}{limit: limitValue, usage: usage}
	return nil
}

func TestQuotaService_NilLimitsAlwaysAllows(t *testing.T) {
	svc := NewQuotaService(nil)
	ok, err := svc.Check(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestQuotaService_NoLimitSetAllows(t *testing.T) {
	svc := NewQuotaService(newMockLimitStore())
	ok, err := svc.Check(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestQuotaService_UnderLimitAllows(t *testing.T) {
	store := newMockLimitStore()
	require.NoError(t, store.Set(context.Background(), "throughput", 100))
	svc := NewQuotaService(store)
	ok, err := svc.Check(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestQuotaService_AtLimitBlocks(t *testing.T) {
	store := newMockLimitStore()
	require.NoError(t, store.Set(context.Background(), "throughput", 3))
	for i := 0; i < 3; i++ {
		require.NoError(t, store.Increment(context.Background(), "throughput"))
	}
	svc := NewQuotaService(store)
	ok, err := svc.Check(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestQuotaService_IncrementTracksUsage(t *testing.T) {
	store := newMockLimitStore()
	require.NoError(t, store.Set(context.Background(), "throughput", 10))
	svc := NewQuotaService(store)
	for i := 0; i < 5; i++ {
		require.NoError(t, svc.Increment(context.Background(), "tenant-1", "throughput"))
	}
	limit, usage, err := store.Get(context.Background(), "throughput")
	require.NoError(t, err)
	assert.Equal(t, 10, limit)
	assert.Equal(t, 5, usage)
}
