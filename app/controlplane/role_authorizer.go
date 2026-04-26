package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	AccessRead  = "read"
	AccessWrite = "write"
)

type RoleAuthorizer struct {
	store WorkspaceStore
}

func NewRoleAuthorizer(store WorkspaceStore) *RoleAuthorizer {
	return &RoleAuthorizer{store: store}
}

func (a *RoleAuthorizer) Authorize(ctx context.Context, workspaceID string, userID string, access string) error {
	if a.store == nil {
		return fmt.Errorf("workspace store is nil")
	}

	workspaceID = strings.TrimSpace(workspaceID)
	userID = strings.TrimSpace(userID)
	access = strings.TrimSpace(access)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if userID == "" {
		return fmt.Errorf("user id is required")
	}

	ws, err := a.store.Get(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace %q: %w", workspaceID, err)
	}

	member, err := a.store.GetMember(ctx, ws.ID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %q is not a workspace member: %w", userID, err)
		}
		return fmt.Errorf("failed to get workspace member %q: %w", userID, err)
	}

	role := WorkspaceRole(member.Role)
	if !role.Valid() {
		return fmt.Errorf("invalid stored workspace role %q", member.Role)
	}

	if access == AccessRead {
		return nil
	}
	if access == AccessWrite && role.CanWrite() {
		return nil
	}
	return fmt.Errorf("role %q cannot %s workspace %q", role, access, workspaceID)
}

func (r WorkspaceRole) CanWrite() bool {
	switch r {
	case RoleOwner, RoleAdmin:
		return true
	default:
		return false
	}
}
