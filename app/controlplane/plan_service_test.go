package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTenantPlanStore struct {
	plan    string
	getErr  error
	setErr  error
	setPlan string
}

func (m *mockTenantPlanStore) GetPlan(_ context.Context, _ string) (Plan, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	return Plan(m.plan), nil
}

func (m *mockTenantPlanStore) SetPlan(_ context.Context, _, planName string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.setPlan = planName
	return nil
}

func TestPlanService_GetPlanFree(t *testing.T) {
	store := &mockTenantPlanStore{plan: "free"}
	svc := NewPlanService(store)
	def, err := svc.GetPlan(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, PlanFree, def.Name)
	assert.Equal(t, 1000, def.Limits.SpamChecksPerHour)
	assert.Equal(t, 1, def.Limits.MaxWorkspaces)
	assert.Equal(t, 168, def.Limits.HistoryRetentionHrs)
}

func TestPlanService_GetPlanPro(t *testing.T) {
	store := &mockTenantPlanStore{plan: "pro"}
	svc := NewPlanService(store)
	def, err := svc.GetPlan(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, PlanPro, def.Name)
	assert.Equal(t, 10000, def.Limits.SpamChecksPerHour)
	assert.Equal(t, 500, def.Limits.SlowPathPerHour)
	assert.Equal(t, 5, def.Limits.MaxWorkspaces)
	assert.Equal(t, 720, def.Limits.HistoryRetentionHrs)
}

func TestPlanService_GetPlanEnterprise(t *testing.T) {
	store := &mockTenantPlanStore{plan: "enterprise"}
	svc := NewPlanService(store)
	def, err := svc.GetPlan(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, PlanEnterprise, def.Name)
	assert.Equal(t, 0, def.Limits.SpamChecksPerHour)
	assert.Equal(t, 0, def.Limits.MaxWorkspaces)
	assert.Contains(t, def.Features, FeatureVision)
	assert.Contains(t, def.Features, FeaturePriorityQueue)
}

func TestPlanService_HasFeatureTrue(t *testing.T) {
	store := &mockTenantPlanStore{plan: "pro"}
	svc := NewPlanService(store)
	ok, err := svc.HasFeature(context.Background(), "t1", FeatureSlowPath)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPlanService_HasFeatureFalse(t *testing.T) {
	store := &mockTenantPlanStore{plan: "free"}
	svc := NewPlanService(store)
	ok, err := svc.HasFeature(context.Background(), "t1", FeatureSlowPath)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPlanService_SetPlan(t *testing.T) {
	store := &mockTenantPlanStore{}
	svc := NewPlanService(store)
	err := svc.SetPlan(context.Background(), "t1", "pro")
	require.NoError(t, err)
	assert.Equal(t, "pro", store.setPlan)
}

func TestPlanService_GetPlanFallback(t *testing.T) {
	store := &mockTenantPlanStore{getErr: errors.New("db error")}
	svc := NewPlanService(store)
	def, err := svc.GetPlan(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, PlanFree, def.Name)
}

func TestPlanCatalogCompleteness(t *testing.T) {
	for _, p := range []Plan{PlanFree, PlanPro, PlanEnterprise} {
		def, ok := GetPlanDefinition(p)
		assert.True(t, ok, "plan %s should exist in catalog", p)
		assert.Equal(t, p, def.Name)
		assert.NotEmpty(t, def.Features)
	}

	_, ok := GetPlanDefinition(Plan("nonexistent"))
	assert.False(t, ok)
}
