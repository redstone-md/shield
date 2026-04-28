package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage"
)

type mockTenantStore struct {
	record  storage.TenantRecord
	status  string
	getErr  error
	updErr  error
}

func (m *mockTenantStore) Get(_ context.Context, id string) (storage.TenantRecord, error) {
	if m.getErr != nil {
		return storage.TenantRecord{}, m.getErr
	}
	m.record.ID = id
	m.record.Status = m.status
	return m.record, nil
}

func (m *mockTenantStore) UpdateStatus(_ context.Context, _, status string) error {
	m.status = status
	return m.updErr
}

func TestTenantService_Suspend(t *testing.T) {
	store := &mockTenantStore{status: "active"}
	svc := NewTenantService(store)

	err := svc.Suspend(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "suspended", store.status)
}

func TestTenantService_Resume(t *testing.T) {
	store := &mockTenantStore{status: "suspended"}
	svc := NewTenantService(store)

	err := svc.Resume(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "active", store.status)
}

func TestTenantService_SoftDelete(t *testing.T) {
	store := &mockTenantStore{status: "active"}
	svc := NewTenantService(store)

	err := svc.SoftDelete(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "deleted", store.status)
}

func TestTenantService_SuspendIdempotent(t *testing.T) {
	store := &mockTenantStore{status: "suspended"}
	svc := NewTenantService(store)

	err := svc.Suspend(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "suspended", store.status)
}

func TestTenantService_NotFound(t *testing.T) {
	store := &mockTenantStore{getErr: errors.New("not found")}
	svc := NewTenantService(store)

	err := svc.Suspend(context.Background(), "t1")
	require.Error(t, err)
}

func TestTenantService_EmptyID(t *testing.T) {
	svc := NewTenantService(&mockTenantStore{})
	err := svc.Suspend(context.Background(), "")
	require.Error(t, err)
}

func TestTenantService_GetStatus(t *testing.T) {
	store := &mockTenantStore{status: "active"}
	svc := NewTenantService(store)

	status, err := svc.GetStatus(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "active", status)
}

func TestTenantService_OnChange(t *testing.T) {
	store := &mockTenantStore{status: "active"}
	svc := NewTenantService(store)

	var notifiedID string
	svc.OnChange(func(id string) { notifiedID = id })

	err := svc.Suspend(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "t1", notifiedID)
}
