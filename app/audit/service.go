package audit

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type Service struct {
	store IncidentStore
}

func NewService(store IncidentStore) *Service {
	return &Service{store: store}
}

func NewAuditService(store IncidentStore) *Service {
	return NewService(store)
}

func (s *Service) CreateFromSpam(ctx context.Context, incident Incident) (Incident, error) {
	if incident.Severity == "" {
		incident.Severity = ClassifySeverity(incident.ReasonCode)
	}
	incident.Source = SourceAutoMod
	if incident.Status == "" {
		incident.Status = IncidentStatusOpen
	}

	if incident.IdempotencyKey != "" {
		existing, err := s.store.GetByIdempotencyKey(ctx, "", incident.IdempotencyKey)
		if err == nil && existing.ID > 0 {
			return existing, nil
		}
	}

	created, err := s.store.Create(ctx, incident)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			return Incident{}, nil
		}
		return Incident{}, fmt.Errorf("create incident: %w", err)
	}

	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: created.ID,
		AuthorType: "system",
		AuthorID:   "pipeline",
		Action:     "created",
	})

	return created, nil
}

func (s *Service) CreateIncident(ctx context.Context, data AuditEventData) error {
	reasonCode := ReasonUnknown
	reasonText := "spam detected"
	for _, cr := range data.CheckResults {
		if cr.Spam {
			reasonCode = MapCheckNameToReason(cr.Name)
			reasonText = cr.Details
			break
		}
	}

	severity := ClassifySeverity(reasonCode)

	incident := Incident{
		Source:         SourceAutoMod,
		Status:         IncidentStatusOpen,
		Severity:       severity,
		IdempotencyKey: data.IdempotencyKey,
		ReasonCode:     reasonCode,
		ReasonText:     truncateMsg(reasonText, 500),
		SpamUserID:     data.SpamUserID,
		SpamUserName:   data.SpamUserName,
		ChatID:         data.ChatID,
		MessageText:    truncateMsg(data.MessageText, 1000),
	}

	created, err := s.store.Create(ctx, incident)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil
		}
		return fmt.Errorf("create incident: %w", err)
	}

	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: created.ID,
		AuthorType: "system",
		AuthorID:   "pipeline",
		Action:     "created",
		Payload:    fmt.Sprintf(`{"source":"auto_mod","rule_set_version":%d}`, data.RuleSetVersion),
	})

	if data.SlowProvider != "" {
		_, _ = s.store.AddComment(ctx, IncidentComment{
			IncidentID: created.ID,
			AuthorType: "system",
			AuthorID:   "slow_path",
			Action:     "slow_path_invoked",
			Payload:    fmt.Sprintf(`{"provider":"%s","prompt_version":"%s"}`, data.SlowProvider, data.SlowPromptVer),
		})
	}

	return nil
}

type ReportParams struct {
	ReportID       int64
	MsgText        string
	UserID         int64
	UserName       string
	ChatID         int64
	IdempotencyKey string
}

func (s *Service) CreateFromReport(ctx context.Context, p ReportParams) error {
	incident := Incident{
		Source:         SourceUserReport,
		Status:         IncidentStatusOpen,
		Severity:       SeverityHigh,
		IdempotencyKey: p.IdempotencyKey,
		ReportID:       p.ReportID,
		ReasonCode:     ReasonUserReport,
		ReasonText:     "user report threshold reached",
		SpamUserID:     p.UserID,
		SpamUserName:   p.UserName,
		ChatID:         p.ChatID,
		MessageText:    truncateMsg(p.MsgText, 1000),
	}

	created, err := s.store.Create(ctx, incident)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil
		}
		return fmt.Errorf("create incident from report: %w", err)
	}

	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: created.ID,
		AuthorType: "system",
		AuthorID:   "reports",
		Action:     "created",
		Payload:    fmt.Sprintf(`{"source":"user_report","report_id":%d}`, p.ReportID),
	})

	return nil
}

type AdminActionParams struct {
	AdminUserName  string
	UserID         int64
	UserName       string
	ChatID         int64
	Action         string
	IdempotencyKey string
}

func (s *Service) CreateFromAdminAction(ctx context.Context, p AdminActionParams) error {
	incident := Incident{
		Source:         SourceAdminAction,
		Status:         IncidentStatusResolved,
		Severity:       SeverityHigh,
		IdempotencyKey: p.IdempotencyKey,
		ReasonCode:     ReasonAdminAction,
		ReasonText:     fmt.Sprintf("admin %s performed %s", p.AdminUserName, p.Action),
		SpamUserID:     p.UserID,
		SpamUserName:   p.UserName,
		ChatID:         p.ChatID,
	}

	created, err := s.store.Create(ctx, incident)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil
		}
		return fmt.Errorf("create incident from admin action: %w", err)
	}

	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: created.ID,
		AuthorType: "admin",
		AuthorID:   p.AdminUserName,
		Action:     p.Action,
		Payload:    fmt.Sprintf(`{"admin":"%s","user_id":%d,"action":"%s"}`, p.AdminUserName, p.UserID, p.Action),
	})

	return nil
}

func (s *Service) GetIncident(ctx context.Context, id int64) (Incident, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) GetByIdempotencyKey(ctx context.Context, tenantID, key string) (Incident, error) {
	return s.store.GetByIdempotencyKey(ctx, tenantID, key)
}

func (s *Service) ListIncidents(ctx context.Context, filter IncidentFilter) ([]Incident, error) {
	return s.store.List(ctx, filter)
}

func (s *Service) ListByStatus(ctx context.Context, status IncidentStatus, limit int) ([]Incident, error) {
	return s.store.List(ctx, IncidentFilter{Status: status, Limit: limit})
}

func (s *Service) UpdateIncidentStatus(ctx context.Context, id int64, status IncidentStatus, resolvedBy string) error {
	return s.store.UpdateStatus(ctx, id, status, resolvedBy)
}

func (s *Service) Resolve(ctx context.Context, id int64, resolvedBy, comment string) error {
	if err := s.store.UpdateStatus(ctx, id, IncidentStatusResolved, resolvedBy); err != nil {
		return err
	}
	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: id,
		AuthorType: "admin",
		AuthorID:   resolvedBy,
		Action:     "resolved",
		Payload:    comment,
	})
	return nil
}

func (s *Service) AddComment(ctx context.Context, comment IncidentComment) (IncidentComment, error) {
	return s.store.AddComment(ctx, comment)
}

func (s *Service) AddRawComment(ctx context.Context, comment IncidentComment) (IncidentComment, error) {
	return s.store.AddComment(ctx, comment)
}

func (s *Service) ListComments(ctx context.Context, incidentID int64) ([]IncidentComment, error) {
	return s.store.ListComments(ctx, incidentID)
}

func (s *Service) ResolveFromChecks(ctx context.Context, tenantID, idempotencyKey string, checkNames []string) {
	if s.store == nil {
		return
	}
	incident, err := s.store.GetByIdempotencyKey(ctx, tenantID, idempotencyKey)
	if err != nil {
		log.Printf("[DEBUG] audit: incident not found for key %s: %v", idempotencyKey, err)
		return
	}
	if incident.Status != IncidentStatusOpen {
		return
	}
	_ = s.store.UpdateStatus(ctx, incident.ID, IncidentStatusResolved, "system")
}

func truncateMsg(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
