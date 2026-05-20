package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/redstone-md/shield/app/storage"
)

type WorkspaceRole string

const (
	RoleOwner  WorkspaceRole = "owner"
	RoleAdmin  WorkspaceRole = "admin"
	RoleViewer WorkspaceRole = "viewer"
)

type WorkspaceService struct {
	store WorkspaceStore
}

type WorkspaceStore interface {
	Add(ctx context.Context, rec storage.WorkspaceRecord) (int64, error)
	Get(ctx context.Context, name string) (storage.WorkspaceRecord, error)
	AddMember(ctx context.Context, workspaceID int64, userID, role string) error
	GetMember(ctx context.Context, workspaceID int64, userID string) (storage.WorkspaceMemberRecord, error)
}

type WorkspaceBootstrap struct {
	Name    string
	OwnerID string
}

func NewWorkspaceService(store WorkspaceStore) *WorkspaceService {
	return &WorkspaceService{store: store}
}

func (s *WorkspaceService) EnsureDefaultWorkspace(ctx context.Context, req WorkspaceBootstrap) (storage.WorkspaceRecord, error) {
	if s.store == nil {
		return storage.WorkspaceRecord{}, fmt.Errorf("workspace store is nil")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.OwnerID = strings.TrimSpace(req.OwnerID)
	if req.Name == "" {
		return storage.WorkspaceRecord{}, fmt.Errorf("workspace name is required")
	}
	if req.OwnerID == "" {
		return storage.WorkspaceRecord{}, fmt.Errorf("workspace owner id is required")
	}

	rec, err := s.store.Get(ctx, req.Name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return storage.WorkspaceRecord{}, fmt.Errorf("failed to get workspace %q: %w", req.Name, err)
		}
		id, addErr := s.store.Add(ctx, storage.WorkspaceRecord{
			Name:    req.Name,
			OwnerID: req.OwnerID,
			Status:  "active",
		})
		if addErr != nil {
			return storage.WorkspaceRecord{}, fmt.Errorf("failed to create workspace %q: %w", req.Name, addErr)
		}
		rec, err = s.store.Get(ctx, req.Name)
		if err != nil {
			return storage.WorkspaceRecord{}, fmt.Errorf("failed to reload workspace %q after create: %w", req.Name, err)
		}
		rec.ID = id
	}

	if err := s.EnsureMember(ctx, rec.ID, req.OwnerID, RoleOwner); err != nil {
		return storage.WorkspaceRecord{}, err
	}
	return rec, nil
}

func (s *WorkspaceService) EnsureMember(ctx context.Context, workspaceID int64, userID string, role WorkspaceRole) error {
	if s.store == nil {
		return fmt.Errorf("workspace store is nil")
	}
	userID = strings.TrimSpace(userID)
	if workspaceID <= 0 {
		return fmt.Errorf("workspace id is required")
	}
	if userID == "" {
		return fmt.Errorf("workspace member user id is required")
	}
	if !role.Valid() {
		return fmt.Errorf("invalid workspace role %q", role)
	}

	member, err := s.store.GetMember(ctx, workspaceID, userID)
	if err == nil {
		if member.Role == string(role) {
			return nil
		}
		return fmt.Errorf("workspace member %q already has role %q", userID, member.Role)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to get workspace member %q: %w", userID, err)
	}

	if err = s.store.AddMember(ctx, workspaceID, userID, string(role)); err != nil {
		return fmt.Errorf("failed to add workspace member %q: %w", userID, err)
	}
	return nil
}

func (r WorkspaceRole) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleViewer:
		return true
	default:
		return false
	}
}
