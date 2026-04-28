package controlplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationService_DisabledReturnsNil(t *testing.T) {
	svc := NewFederationService(false)
	bans, err := svc.SharedBans(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Nil(t, bans)

	policies, err := svc.InheritedPolicies(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Nil(t, policies)
}

func TestFederationService_EnabledReturnsNil(t *testing.T) {
	svc := NewFederationService(true)
	bans, err := svc.SharedBans(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Nil(t, bans)

	policies, err := svc.InheritedPolicies(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Nil(t, policies)
}

func TestFederationService_InterfaceContract(t *testing.T) {
	var _ interface {
		SharedBans(ctx context.Context, tenantID string) ([]string, error)
		InheritedPolicies(ctx context.Context, tenantID string) ([]string, error)
	} = NewFederationService(false)
}
