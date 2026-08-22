package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/storage"
)

type OnboardingService struct {
	tenants    TenantStore
	workspaces WorkspaceStore
	ruleSets   RuleSetBootstrapper
	cache      RuleSetCache

	// onboardMu makes the exist-check and tenant Add atomic: without it two
	// concurrent Onboard calls for the same tenant can both pass the Get miss
	// and both Add, double-onboarding the tenant.
	onboardMu sync.Mutex
}

type RuleSetBootstrapper interface {
	EnsureBootstrap(ctx context.Context, rs rules.RuleSet) (bool, error)
	Active(ctx context.Context, workspaceID string) (rules.RuleSet, error)
}

type OnboardRequest struct {
	TenantID string
	Name     string
	OwnerID  string
	GID      string
}

type OnboardResult struct {
	TenantID    string
	WorkspaceID string
	RuleSetVer  int
}

func NewOnboardingService(
	tenants TenantStore,
	workspaces WorkspaceStore,
	ruleSets RuleSetBootstrapper,
	cache RuleSetCache,
) *OnboardingService {
	return &OnboardingService{
		tenants:    tenants,
		workspaces: workspaces,
		ruleSets:   ruleSets,
		cache:      cache,
	}
}

func (s *OnboardingService) Onboard(ctx context.Context, req OnboardRequest) (*OnboardResult, error) {
	if req.TenantID == "" || req.Name == "" || req.OwnerID == "" {
		return nil, fmt.Errorf("tenant_id, name and owner_id are required")
	}

	s.onboardMu.Lock()
	_, err := s.tenants.Get(ctx, req.TenantID)
	if err == nil {
		s.onboardMu.Unlock()
		return nil, fmt.Errorf("tenant %s already exists", req.TenantID)
	}

	addErr := s.tenants.Add(ctx, storage.TenantRecord{
		ID:      req.TenantID,
		GID:     req.GID,
		Name:    req.Name,
		Status:  "active",
		OwnerID: req.OwnerID,
	})
	s.onboardMu.Unlock()
	if addErr != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", addErr)
	}
	log.Printf("[INFO] tenant onboarded: id=%s name=%s", req.TenantID, req.Name)

	wsID, err := s.workspaces.Add(ctx, storage.WorkspaceRecord{
		Name:    req.Name,
		OwnerID: req.OwnerID,
		Status:  "active",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	if addErr := s.workspaces.AddMember(ctx, wsID, req.OwnerID, string(RoleOwner)); addErr != nil {
		return nil, fmt.Errorf("failed to add owner to workspace: %w", addErr)
	}
	log.Printf("[INFO] workspace created: id=%d tenant=%s", wsID, req.TenantID)

	wsIDStr := fmt.Sprintf("%d", wsID)
	created, err := s.ruleSets.EnsureBootstrap(ctx, rules.RuleSet{
		WorkspaceID: wsIDStr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap rule set: %w", err)
	}
	_ = created

	rs, err := s.ruleSets.Active(ctx, wsIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to load active rule set: %w", err)
	}
	ver := rs.Version

	if s.cache != nil {
		s.cache.Set(ctx, req.TenantID, wsIDStr, rs)
	}

	return &OnboardResult{
		TenantID:    req.TenantID,
		WorkspaceID: wsIDStr,
		RuleSetVer:  ver,
	}, nil
}

func (s *OnboardingService) Offboard(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}

	rec, err := s.tenants.Get(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tenant %s not found: %w", tenantID, err)
	}
	if rec.Status == "deleted" {
		return nil
	}

	if err := s.tenants.UpdateStatus(ctx, tenantID, "deleted"); err != nil {
		return fmt.Errorf("failed to soft-delete tenant: %w", err)
	}

	if s.cache != nil {
		s.cache.InvalidateAll(ctx, tenantID)
	}

	log.Printf("[INFO] tenant offboarded: id=%s name=%s", tenantID, rec.Name)
	return nil
}
