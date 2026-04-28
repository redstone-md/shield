package controlplane

import (
	"context"
	"log"
)

type FederationService struct {
	enabled bool
}

func NewFederationService(enabled bool) *FederationService {
	return &FederationService{enabled: enabled}
}

func (f *FederationService) SharedBans(_ context.Context, tenantID string) ([]string, error) {
	if !f.enabled {
		return nil, nil
	}
	log.Printf("[DEBUG] federation shared bans requested for tenant %s (no-op)", tenantID)
	return nil, nil
}

func (f *FederationService) InheritedPolicies(_ context.Context, tenantID string) ([]string, error) {
	if !f.enabled {
		return nil, nil
	}
	log.Printf("[DEBUG] federation inherited policies requested for tenant %s (no-op)", tenantID)
	return nil, nil
}
