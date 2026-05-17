package audit

import (
	"context"
	"fmt"
	"log"
)

type BotService interface {
	UnbanUser(ctx context.Context, userID int64) error
	AddHamSample(ctx context.Context, text string) error
	ClearUserWarnings(ctx context.Context, userID int64) error
	NotifyAppealResult(ctx context.Context, userID int64, accepted bool) error
}

type FeedbackLabeler interface {
	AutoLabel(ctx context.Context, incidentID int64, label string) error
}

type AppealService struct {
	appeals   AppealStore
	incidents IncidentStore
	bot       BotService
	labeler   FeedbackLabeler
}

func NewAppealService(appeals AppealStore, incidents IncidentStore, bot BotService) *AppealService {
	return &AppealService{appeals: appeals, incidents: incidents, bot: bot}
}

func (s *AppealService) SetFeedbackLabeler(labeler FeedbackLabeler) {
	s.labeler = labeler
}

// SetBotService wires the bot adapter used to unban users, clear warnings and
// notify them of appeal outcomes. It is set after the listener is constructed.
func (s *AppealService) SetBotService(bot BotService) {
	s.bot = bot
}

func (s *AppealService) Submit(
	ctx context.Context, incidentID, appellantUserID int64, appellantName, appealText string,
) (Appeal, error) {
	_, err := s.incidents.Get(ctx, incidentID)
	if err != nil {
		return Appeal{}, fmt.Errorf("incident %d not found: %w", incidentID, err)
	}

	appeal := Appeal{
		IncidentID:      incidentID,
		AppellantUserID: appellantUserID,
		AppellantName:   appellantName,
		Status:          AppealNew,
		AppealText:      appealText,
	}

	created, err := s.appeals.Create(ctx, appeal)
	if err != nil {
		return Appeal{}, fmt.Errorf("create appeal: %w", err)
	}

	if err := s.incidents.UpdateStatus(ctx, incidentID, IncidentStatusAppealed, ""); err != nil {
		_ = s.appeals.UpdateStatus(ctx, created.ID, AppealNew, "", "")
		return Appeal{}, fmt.Errorf("update incident status: %w", err)
	}

	_, _ = s.incidents.AddComment(ctx, IncidentComment{
		IncidentID: incidentID,
		AuthorType: "user",
		AuthorID:   fmt.Sprintf("%d", appellantUserID),
		Action:     "appeal_submitted",
		Payload:    appealText,
	})

	return created, nil
}

func (s *AppealService) Triage(ctx context.Context, appealID int64, triagerID string) error {
	ap, err := s.appeals.Get(ctx, appealID)
	if err != nil {
		return fmt.Errorf("appeal %d not found: %w", appealID, err)
	}
	if ap.Status != AppealNew {
		return fmt.Errorf("appeal %d is %s, expected new", appealID, ap.Status)
	}

	if err := s.appeals.UpdateStatus(ctx, appealID, AppealTriaged, triagerID, ""); err != nil {
		return fmt.Errorf("update appeal status: %w", err)
	}

	_, _ = s.incidents.AddComment(ctx, IncidentComment{
		IncidentID: ap.IncidentID,
		AuthorType: "admin",
		AuthorID:   triagerID,
		Action:     "appeal_triaged",
	})

	return nil
}

func (s *AppealService) Accept(ctx context.Context, appealID int64, resolverID, resolutionText string) error {
	ap, err := s.appeals.Get(ctx, appealID)
	if err != nil {
		return fmt.Errorf("appeal %d not found: %w", appealID, err)
	}

	inc, err := s.incidents.Get(ctx, ap.IncidentID)
	if err != nil {
		return fmt.Errorf("incident %d not found: %w", ap.IncidentID, err)
	}

	if err := s.appeals.UpdateStatus(ctx, appealID, AppealAccepted, resolverID, resolutionText); err != nil {
		return fmt.Errorf("update appeal status: %w", err)
	}

	if err := s.incidents.UpdateStatus(ctx, ap.IncidentID, IncidentStatusClosed, resolverID); err != nil {
		return fmt.Errorf("update incident status: %w", err)
	}

	if s.bot != nil {
		_ = s.bot.UnbanUser(ctx, inc.SpamUserID)
		if inc.MessageText != "" {
			_ = s.bot.AddHamSample(ctx, inc.MessageText)
		}
		_ = s.bot.ClearUserWarnings(ctx, inc.SpamUserID)
		_ = s.bot.NotifyAppealResult(ctx, inc.SpamUserID, true)
	}

	s.autoLabel(ctx, inc, "false_positive", resolverID)

	_, _ = s.incidents.AddComment(ctx, IncidentComment{
		IncidentID: ap.IncidentID,
		AuthorType: "admin",
		AuthorID:   resolverID,
		Action:     "appeal_accepted",
		Payload:    resolutionText,
	})

	return nil
}

func (s *AppealService) Reject(ctx context.Context, appealID int64, resolverID, resolutionText string) error {
	ap, err := s.appeals.Get(ctx, appealID)
	if err != nil {
		return fmt.Errorf("appeal %d not found: %w", appealID, err)
	}

	if err := s.appeals.UpdateStatus(ctx, appealID, AppealRejected, resolverID, resolutionText); err != nil {
		return fmt.Errorf("update appeal status: %w", err)
	}

	if err := s.incidents.UpdateStatus(ctx, ap.IncidentID, IncidentStatusClosed, resolverID); err != nil {
		return fmt.Errorf("update incident status: %w", err)
	}

	if s.bot != nil {
		_ = s.bot.NotifyAppealResult(ctx, ap.AppellantUserID, false)
	}

	s.autoLabelAppeal(ctx, ap, "rejected", resolverID)

	_, _ = s.incidents.AddComment(ctx, IncidentComment{
		IncidentID: ap.IncidentID,
		AuthorType: "admin",
		AuthorID:   resolverID,
		Action:     "appeal_rejected",
		Payload:    resolutionText,
	})

	return nil
}

func (s *AppealService) Escalate(ctx context.Context, appealID int64) error {
	ap, err := s.appeals.Get(ctx, appealID)
	if err != nil {
		return fmt.Errorf("appeal %d not found: %w", appealID, err)
	}

	if err := s.appeals.UpdateStatus(ctx, appealID, AppealEscalated, "system", ""); err != nil {
		return fmt.Errorf("update appeal status: %w", err)
	}

	if err := s.incidents.UpdateStatus(ctx, ap.IncidentID, IncidentStatusReviewing, "system"); err != nil {
		return fmt.Errorf("update incident status: %w", err)
	}

	_, _ = s.incidents.AddComment(ctx, IncidentComment{
		IncidentID: ap.IncidentID,
		AuthorType: "system",
		AuthorID:   "system",
		Action:     "appeal_escalated",
	})

	return nil
}

func (s *AppealService) StoreReplayResult(ctx context.Context, appealID int64, result ReplayResult) error {
	return s.appeals.UpdateReplayResult(ctx, appealID, result)
}

func (s *AppealService) ListByStatus(ctx context.Context, status AppealStatus, limit, offset int) ([]Appeal, error) {
	return s.appeals.List(ctx, AppealFilter{Status: status, Limit: limit, Offset: offset})
}

func (s *AppealService) GetForIncident(ctx context.Context, incidentID int64) (Appeal, error) {
	return s.appeals.GetByIncident(ctx, incidentID)
}

// GetAppeal returns a single appeal by id.
func (s *AppealService) GetAppeal(ctx context.Context, appealID int64) (Appeal, error) {
	return s.appeals.Get(ctx, appealID)
}

// GetIncident returns a single incident by id.
func (s *AppealService) GetIncident(ctx context.Context, incidentID int64) (Incident, error) {
	return s.incidents.Get(ctx, incidentID)
}

func (s *AppealService) autoLabel(ctx context.Context, inc Incident, label, _ string) {
	if s.labeler == nil {
		return
	}
	if err := s.labeler.AutoLabel(ctx, inc.ID, label); err != nil {
		log.Printf("[WARN] auto-label incident %d as %s failed: %v", inc.ID, label, err)
	}
}

func (s *AppealService) autoLabelAppeal(ctx context.Context, ap Appeal, action, _ string) {
	if s.labeler == nil {
		return
	}
	inc, err := s.incidents.Get(ctx, ap.IncidentID)
	if err != nil {
		return
	}
	label := "confirmed_spam"
	if action == "accepted" {
		label = "false_positive"
	}
	if err := s.labeler.AutoLabel(ctx, inc.ID, label); err != nil {
		log.Printf("[WARN] auto-label incident %d as %s failed: %v", inc.ID, label, err)
	}
}
