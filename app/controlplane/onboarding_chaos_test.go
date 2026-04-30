package controlplane

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage"
)

func TestOnboardingChaos_ConcurrentOnboard(t *testing.T) {
	ts := &chaosTenantStore{records: make(map[string]storage.TenantRecord)}
	ws := &mockOnboardWorkspaceStore{wsID: 1}
	rs := &mockOnboardRuleSetStore{}
	cache := newMemoryCache(0)
	svc := NewOnboardingService(ts, ws, rs, cache)

	const workers = 50
	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.Onboard(context.Background(), OnboardRequest{
				TenantID: "tenant-chaos",
				Name:     "chaos-tenant",
				OwnerID:  "owner",
				GID:      "g1",
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}

	assert.Equal(t, 1, successes, "exactly one onboard should succeed")
	rec, err := ts.Get(context.Background(), "tenant-chaos")
	require.NoError(t, err)
	assert.Equal(t, "active", rec.Status)
}

func TestOnboardingChaos_TenantAddFailsRollback(t *testing.T) {
	ts := &chaosTenantStore{records: make(map[string]storage.TenantRecord), addErr: errors.New("db down")}
	ws := &mockOnboardWorkspaceStore{wsID: 1}
	rs := &mockOnboardRuleSetStore{}
	svc := NewOnboardingService(ts, ws, rs, nil)

	_, err := svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "fail-tenant",
		Name:     "fail",
		OwnerID:  "owner",
		GID:      "g1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create tenant")

	_, err = ts.Get(context.Background(), "fail-tenant")
	assert.Error(t, err, "tenant should not exist after failed add")
}

func TestOnboardingChaos_WorkspaceAddFails(t *testing.T) {
	ts := &chaosTenantStore{records: make(map[string]storage.TenantRecord)}
	ws := &mockOnboardWorkspaceStore{addErr: errors.New("ws full")}
	rs := &mockOnboardRuleSetStore{}
	svc := NewOnboardingService(ts, ws, rs, nil)

	_, err := svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "ws-fail",
		Name:     "ws-fail",
		OwnerID:  "owner",
		GID:      "g1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create workspace")

	rec, err := ts.Get(context.Background(), "ws-fail")
	require.NoError(t, err)
	assert.Equal(t, "active", rec.Status, "tenant created but workspace failed — orphaned tenant")
}

func TestOnboardingChaos_ConcurrentOffboard(t *testing.T) {
	ts := &chaosTenantStore{records: map[string]storage.TenantRecord{
		"offboard-chaos": {ID: "offboard-chaos", Status: "active", Name: "test"},
	}}
	ws := &mockOnboardWorkspaceStore{wsID: 1}
	rs := &mockOnboardRuleSetStore{}
	cache := newMemoryCache(0)
	svc := NewOnboardingService(ts, ws, rs, cache)

	const workers = 50
	var wg sync.WaitGroup
	results := make(chan error, workers)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- svc.Offboard(context.Background(), "offboard-chaos")
		}()
	}
	wg.Wait()
	close(results)

	errCount := 0
	for err := range results {
		if err != nil {
			errCount++
		}
	}
	assert.Equal(t, 0, errCount, "all offboards should succeed (idempotent)")

	rec, err := ts.Get(context.Background(), "offboard-chaos")
	require.NoError(t, err)
	assert.Equal(t, "deleted", rec.Status)
}

func TestOnboardingChaos_OffboardThenOnboardFails(t *testing.T) {
	ts := &chaosTenantStore{records: map[string]storage.TenantRecord{
		"cycle-test": {ID: "cycle-test", Status: "active", Name: "test"},
	}}
	ws := &mockOnboardWorkspaceStore{wsID: 1}
	rs := &mockOnboardRuleSetStore{}
	cache := newMemoryCache(0)
	svc := NewOnboardingService(ts, ws, rs, cache)

	err := svc.Offboard(context.Background(), "cycle-test")
	require.NoError(t, err)

	_, err = svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "cycle-test",
		Name:     "new-name",
		OwnerID:  "owner",
		GID:      "g1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

type chaosTenantStore struct {
	mu      sync.Mutex
	records map[string]storage.TenantRecord
	addErr  error
	updErr  error
}

func (c *chaosTenantStore) Get(_ context.Context, id string) (storage.TenantRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, ok := c.records[id]
	if !ok {
		return storage.TenantRecord{}, errors.New("not found")
	}
	return rec, nil
}

func (c *chaosTenantStore) Add(_ context.Context, rec storage.TenantRecord) error {
	if c.addErr != nil {
		return c.addErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records[rec.ID] = rec
	return nil
}

func (c *chaosTenantStore) UpdateStatus(_ context.Context, id, status string) error {
	if c.updErr != nil {
		return c.updErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, ok := c.records[id]
	if !ok {
		return errors.New("not found")
	}
	rec.Status = status
	c.records[id] = rec
	return nil
}
