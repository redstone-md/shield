package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
)

type mockOnboardTenantStore struct {
	record storage.TenantRecord
	exists bool
	addErr error
	updErr error
}

func (m *mockOnboardTenantStore) Get(_ context.Context, id string) (storage.TenantRecord, error) {
	if m.exists {
		m.record.ID = id
		return m.record, nil
	}
	return storage.TenantRecord{}, errors.New("not found")
}

func (m *mockOnboardTenantStore) Add(_ context.Context, rec storage.TenantRecord) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.record = rec
	m.exists = true
	return nil
}

func (m *mockOnboardTenantStore) UpdateStatus(_ context.Context, _, status string) error {
	if m.updErr != nil {
		return m.updErr
	}
	m.record.Status = status
	return nil
}

type mockOnboardWorkspaceStore struct {
	wsID    int64
	addErr  error
	members []storage.WorkspaceMemberRecord
}

func (m *mockOnboardWorkspaceStore) Add(_ context.Context, _ storage.WorkspaceRecord) (int64, error) {
	if m.addErr != nil {
		return 0, m.addErr
	}
	return m.wsID, nil
}

func (m *mockOnboardWorkspaceStore) Get(_ context.Context, _ string) (storage.WorkspaceRecord, error) {
	return storage.WorkspaceRecord{ID: m.wsID}, nil
}

func (m *mockOnboardWorkspaceStore) AddMember(_ context.Context, wsID int64, userID, role string) error {
	m.members = append(m.members, storage.WorkspaceMemberRecord{
		WorkspaceID: wsID, UserID: userID, Role: role,
	})
	return nil
}

func (m *mockOnboardWorkspaceStore) GetMember(_ context.Context, _ int64, _ string) (storage.WorkspaceMemberRecord, error) {
	return storage.WorkspaceMemberRecord{}, errors.New("not found")
}

type mockOnboardRuleSetStore struct {
	bootstrapped bool
	rs           rules.RuleSet
}

func (m *mockOnboardRuleSetStore) EnsureBootstrap(_ context.Context, rs rules.RuleSet) (bool, error) {
	m.bootstrapped = true
	m.rs = rs
	return true, nil
}

func (m *mockOnboardRuleSetStore) Active(_ context.Context, _ string) (rules.RuleSet, error) {
	m.rs.Version = 1
	return m.rs, nil
}

func TestOnboardingService_Onboard(t *testing.T) {
	ts := &mockOnboardTenantStore{}
	ws := &mockOnboardWorkspaceStore{wsID: 42}
	rs := &mockOnboardRuleSetStore{}
	cache := newMemoryCache(0)

	svc := NewOnboardingService(ts, ws, rs, cache)

	res, err := svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "t1",
		Name:     "acme",
		OwnerID:  "user1",
		GID:      "g1",
	})
	require.NoError(t, err)
	assert.Equal(t, "t1", res.TenantID)
	assert.Equal(t, "42", res.WorkspaceID)
	assert.Equal(t, 1, res.RuleSetVer)
	assert.True(t, ts.exists)
	assert.True(t, rs.bootstrapped)
	assert.Len(t, ws.members, 1)
	assert.Equal(t, "user1", ws.members[0].UserID)
}

func TestOnboardingService_OnboardAlreadyExists(t *testing.T) {
	ts := &mockOnboardTenantStore{exists: true}
	svc := NewOnboardingService(ts, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)

	_, err := svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "t1", Name: "acme", OwnerID: "u1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestOnboardingService_OnboardMissingFields(t *testing.T) {
	svc := NewOnboardingService(&mockOnboardTenantStore{}, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)

	_, err := svc.Onboard(context.Background(), OnboardRequest{TenantID: "t1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestOnboardingService_OnboardTenantAddFails(t *testing.T) {
	ts := &mockOnboardTenantStore{addErr: errors.New("db error")}
	svc := NewOnboardingService(ts, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)

	_, err := svc.Onboard(context.Background(), OnboardRequest{
		TenantID: "t1", Name: "acme", OwnerID: "u1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create tenant")
}

func TestOnboardingService_Offboard(t *testing.T) {
	ts := &mockOnboardTenantStore{exists: true, record: storage.TenantRecord{Status: "active"}}
	cache := newMemoryCache(0)
	svc := NewOnboardingService(ts, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, cache)

	err := svc.Offboard(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "deleted", ts.record.Status)
}

func TestOnboardingService_OffboardAlreadyDeleted(t *testing.T) {
	ts := &mockOnboardTenantStore{exists: true, record: storage.TenantRecord{Status: "deleted"}}
	svc := NewOnboardingService(ts, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)

	err := svc.Offboard(context.Background(), "t1")
	require.NoError(t, err)
}

func TestOnboardingService_OffboardNotFound(t *testing.T) {
	ts := &mockOnboardTenantStore{}
	svc := NewOnboardingService(ts, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)

	err := svc.Offboard(context.Background(), "t1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOnboardingService_OffboardEmptyID(t *testing.T) {
	svc := NewOnboardingService(&mockOnboardTenantStore{}, &mockOnboardWorkspaceStore{}, &mockOnboardRuleSetStore{}, nil)
	err := svc.Offboard(context.Background(), "")
	require.Error(t, err)
}
