package audit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockIncidentStore struct {
	incidents []Incident
	comments  map[int64][]IncidentComment
	nextID    int64
}

func newMockIncidentStore() *mockIncidentStore {
	return &mockIncidentStore{
		incidents: nil,
		comments:  make(map[int64][]IncidentComment),
		nextID:    1,
	}
}

func (m *mockIncidentStore) Create(_ context.Context, inc Incident) (Incident, error) {
	inc.ID = m.nextID
	m.nextID++
	now := time.Now()
	inc.CreatedAt = now
	inc.UpdatedAt = now
	inc.GID = "test"
	inc.TenantID = "test"
	m.incidents = append(m.incidents, inc)
	return inc, nil
}

func (m *mockIncidentStore) Get(_ context.Context, id int64) (Incident, error) {
	for _, inc := range m.incidents {
		if inc.ID == id {
			return inc, nil
		}
	}
	return Incident{}, ErrNotFound
}

func (m *mockIncidentStore) GetByIdempotencyKey(_ context.Context, _, key string) (Incident, error) {
	for _, inc := range m.incidents {
		if inc.IdempotencyKey == key {
			return inc, nil
		}
	}
	return Incident{}, ErrNotFound
}

func (m *mockIncidentStore) List(_ context.Context, filter IncidentFilter) ([]Incident, error) {
	var result []Incident
	for _, inc := range m.incidents {
		if filter.Status != "" && inc.Status != filter.Status {
			continue
		}
		if filter.Source != "" && inc.Source != filter.Source {
			continue
		}
		result = append(result, inc)
	}
	return result, nil
}

func (m *mockIncidentStore) UpdateStatus(_ context.Context, id int64, status IncidentStatus, resolvedBy string) error {
	for i := range m.incidents {
		if m.incidents[i].ID == id {
			m.incidents[i].Status = status
			m.incidents[i].ResolvedBy = resolvedBy
			m.incidents[i].UpdatedAt = time.Now()
			if status == IncidentStatusResolved || status == IncidentStatusClosed {
				now := time.Now()
				m.incidents[i].ResolvedAt = &now
			}
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockIncidentStore) UpdateSeverity(_ context.Context, id int64, severity IncidentSeverity) error {
	for i := range m.incidents {
		if m.incidents[i].ID == id {
			m.incidents[i].Severity = severity
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockIncidentStore) AddComment(_ context.Context, c IncidentComment) (IncidentComment, error) {
	c.ID = int64(len(m.comments) + 1)
	c.CreatedAt = time.Now()
	m.comments[c.IncidentID] = append(m.comments[c.IncidentID], c)
	return c, nil
}

func (m *mockIncidentStore) ListComments(_ context.Context, incidentID int64) ([]IncidentComment, error) {
	return m.comments[incidentID], nil
}

type mockAppealStore struct {
	appeals []Appeal
	nextID  int64
}

func newMockAppealStore() *mockAppealStore {
	return &mockAppealStore{nextID: 1}
}

func (m *mockAppealStore) Create(_ context.Context, ap Appeal) (Appeal, error) {
	ap.ID = m.nextID
	m.nextID++
	now := time.Now()
	ap.CreatedAt = now
	ap.UpdatedAt = now
	ap.GID = "test"
	ap.TenantID = "test"
	m.appeals = append(m.appeals, ap)
	return ap, nil
}

func (m *mockAppealStore) Get(_ context.Context, id int64) (Appeal, error) {
	for _, ap := range m.appeals {
		if ap.ID == id {
			return ap, nil
		}
	}
	return Appeal{}, ErrNotFound
}

func (m *mockAppealStore) GetByIncident(_ context.Context, incidentID int64) (Appeal, error) {
	for _, ap := range m.appeals {
		if ap.IncidentID == incidentID {
			return ap, nil
		}
	}
	return Appeal{}, ErrNotFound
}

func (m *mockAppealStore) List(_ context.Context, filter AppealFilter) ([]Appeal, error) {
	var result []Appeal
	for _, ap := range m.appeals {
		if filter.Status != "" && ap.Status != filter.Status {
			continue
		}
		result = append(result, ap)
	}
	return result, nil
}

func (m *mockAppealStore) UpdateStatus(_ context.Context, id int64, status AppealStatus, resolvedBy, resolutionText string) error {
	for i := range m.appeals {
		if m.appeals[i].ID == id {
			m.appeals[i].Status = status
			m.appeals[i].ResolvedBy = resolvedBy
			m.appeals[i].ResolutionText = resolutionText
			m.appeals[i].UpdatedAt = time.Now()
			if status == AppealAccepted || status == AppealRejected {
				now := time.Now()
				m.appeals[i].ResolvedAt = &now
			}
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockAppealStore) UpdateReplayResult(_ context.Context, id int64, result ReplayResult) error {
	return nil
}

type mockBotService struct {
	unbannedIDs []int64
	hamAdded    []string
}

func (m *mockBotService) UnbanUser(_ context.Context, userID int64) error {
	m.unbannedIDs = append(m.unbannedIDs, userID)
	return nil
}

func (m *mockBotService) AddHamSample(_ context.Context, text string) error {
	m.hamAdded = append(m.hamAdded, text)
	return nil
}

func TestAuditService_CreateFromSpam(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	inc, err := svc.CreateFromSpam(context.Background(), Incident{
		Source:         SourceAutoMod,
		IdempotencyKey: "spam-1",
		ReasonCode:     ReasonRegexMatch,
		ReasonText:     "regex matched",
		SpamUserID:     42,
		SpamUserName:   "spammer",
		ChatID:         100,
		MessageText:    "buy stuff",
	})
	require.NoError(t, err)
	assert.NotZero(t, inc.ID)
	assert.Equal(t, SourceAutoMod, inc.Source)
	assert.Equal(t, IncidentStatusOpen, inc.Status)
	assert.Equal(t, ReasonRegexMatch, inc.ReasonCode)
	assert.Equal(t, SeverityLow, inc.Severity)

	comments := incStore.comments[inc.ID]
	assert.Len(t, comments, 1)
	assert.Equal(t, "created", comments[0].Action)
}

func TestAuditService_CreateFromSpam_Dedup(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	_, err := svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "dup-key",
		ReasonCode:     ReasonStopWord,
	})
	require.NoError(t, err)

	_, err = svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "dup-key",
		ReasonCode:     ReasonStopWord,
	})
	require.NoError(t, err)

	assert.Len(t, incStore.incidents, 1)
}

func TestAuditService_Resolve(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	inc, err := svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "resolve-test",
		ReasonCode:     ReasonRegexMatch,
	})
	require.NoError(t, err)

	err = svc.Resolve(context.Background(), inc.ID, "admin1", "confirmed spam")
	require.NoError(t, err)

	got, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusResolved, got.Status)
	assert.Equal(t, "admin1", got.ResolvedBy)
	assert.NotNil(t, got.ResolvedAt)
}

func TestAuditService_AddComment(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	inc, _ := svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "comment-test",
		ReasonCode:     ReasonRegexMatch,
	})

	c, err := svc.AddComment(context.Background(), IncidentComment{
		IncidentID: inc.ID, AuthorType: "admin", AuthorID: "admin1", Action: "reviewed", Payload: "looks good",
	})
	require.NoError(t, err)
	assert.NotZero(t, c.ID)
	assert.Equal(t, "reviewed", c.Action)
}

func TestAuditService_ListByStatus(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	svc.CreateFromSpam(context.Background(), Incident{IdempotencyKey: "a", ReasonCode: ReasonRegexMatch})
	svc.CreateFromSpam(context.Background(), Incident{IdempotencyKey: "b", ReasonCode: ReasonStopWord})
	svc.Resolve(context.Background(), 1, "admin", "done")

	open, err := svc.ListByStatus(context.Background(), IncidentStatusOpen, 10)
	require.NoError(t, err)
	assert.Len(t, open, 1)

	resolved, err := svc.ListByStatus(context.Background(), IncidentStatusResolved, 10)
	require.NoError(t, err)
	assert.Len(t, resolved, 1)
}

func TestAppealService_Submit(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}
	svc := NewAppealService(apStore, incStore, bot)

	inc, _ := incStore.Create(context.Background(), Incident{
		Source:         SourceAutoMod,
		IdempotencyKey: "appeal-test",
		ReasonCode:     ReasonRegexMatch,
		SpamUserID:     42,
		MessageText:    "spam text",
	})

	ap, err := svc.Submit(context.Background(), inc.ID, 42, "testuser", "I'm innocent")
	require.NoError(t, err)
	assert.NotZero(t, ap.ID)
	assert.Equal(t, AppealNew, ap.Status)
	assert.Equal(t, "I'm innocent", ap.AppealText)

	got, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusAppealed, got.Status)
}

func TestAppealService_Accept(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}
	svc := NewAppealService(apStore, incStore, bot)

	inc, _ := incStore.Create(context.Background(), Incident{
		IdempotencyKey: "accept-test",
		ReasonCode:     ReasonRegexMatch,
		SpamUserID:     42,
		MessageText:    "spam text",
	})

	ap, _ := svc.Submit(context.Background(), inc.ID, 42, "user", "appeal")

	err := svc.Accept(context.Background(), ap.ID, "admin1", "false positive confirmed")
	require.NoError(t, err)

	got, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealAccepted, got.Status)
	assert.Equal(t, "admin1", got.ResolvedBy)

	incGot, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusClosed, incGot.Status)

	assert.Contains(t, bot.unbannedIDs, int64(42))
	assert.Contains(t, bot.hamAdded, "spam text")
}

func TestAppealService_Reject(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}
	svc := NewAppealService(apStore, incStore, bot)

	inc, _ := incStore.Create(context.Background(), Incident{
		IdempotencyKey: "reject-test",
		ReasonCode:     ReasonStopWord,
		SpamUserID:     99,
	})

	ap, _ := svc.Submit(context.Background(), inc.ID, 99, "user", "appeal")

	err := svc.Reject(context.Background(), ap.ID, "admin1", "spam confirmed")
	require.NoError(t, err)

	got, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealRejected, got.Status)
	assert.Empty(t, bot.unbannedIDs)
}

func TestAppealService_Escalate(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}
	svc := NewAppealService(apStore, incStore, bot)

	inc, _ := incStore.Create(context.Background(), Incident{
		IdempotencyKey: "esc-test",
		ReasonCode:     ReasonRegexMatch,
		SpamUserID:     77,
	})

	ap, _ := svc.Submit(context.Background(), inc.ID, 77, "user", "please review")

	err := svc.Escalate(context.Background(), ap.ID)
	require.NoError(t, err)

	got, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealEscalated, got.Status)

	incGot, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusReviewing, incGot.Status)
}

func TestAppealService_Triage(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}
	svc := NewAppealService(apStore, incStore, bot)

	inc, _ := incStore.Create(context.Background(), Incident{
		IdempotencyKey: "triage-test",
		ReasonCode:     ReasonRegexMatch,
		SpamUserID:     55,
	})

	ap, _ := svc.Submit(context.Background(), inc.ID, 55, "user", "triage me")

	err := svc.Triage(context.Background(), ap.ID, "admin1")
	require.NoError(t, err)

	got, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealTriaged, got.Status)
}

func TestFullWorkflow_SpamDetectionToAppealAccepted(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}

	auditSvc := NewAuditService(incStore)
	appealSvc := NewAppealService(apStore, incStore, bot)

	inc, err := auditSvc.CreateFromSpam(context.Background(), Incident{
		Source:         SourceAutoMod,
		IdempotencyKey: fmt.Sprintf("wf-%d", time.Now().UnixNano()),
		ReasonCode:     ReasonRegexMatch,
		ReasonText:     "regex matched promotional content",
		SpamUserID:     123,
		SpamUserName:   "spammer123",
		ChatID:         999,
		MessageText:    "buy cheap stuff now!!!",
	})
	require.NoError(t, err)
	assert.Equal(t, IncidentStatusOpen, inc.Status)
	assert.Equal(t, SeverityLow, inc.Severity)

	comments, _ := incStore.ListComments(context.Background(), inc.ID)
	assert.Len(t, comments, 1)
	assert.Equal(t, "created", comments[0].Action)

	ap, err := appealSvc.Submit(context.Background(), inc.ID, 123, "spammer123", "I was not spamming, this was a legitimate message")
	require.NoError(t, err)
	assert.Equal(t, AppealNew, ap.Status)

	updatedInc, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusAppealed, updatedInc.Status)

	err = appealSvc.Triage(context.Background(), ap.ID, "moderator1")
	require.NoError(t, err)

	err = appealSvc.Accept(context.Background(), ap.ID, "admin1", "false positive confirmed, user is legitimate")
	require.NoError(t, err)

	finalAp, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealAccepted, finalAp.Status)
	assert.Equal(t, "admin1", finalAp.ResolvedBy)
	assert.Equal(t, "false positive confirmed, user is legitimate", finalAp.ResolutionText)
	assert.NotNil(t, finalAp.ResolvedAt)

	finalInc, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusClosed, finalInc.Status)

	assert.Contains(t, bot.unbannedIDs, int64(123))
	assert.Contains(t, bot.hamAdded, "buy cheap stuff now!!!")

	allComments, _ := incStore.ListComments(context.Background(), inc.ID)
	assert.GreaterOrEqual(t, len(allComments), 4)
}

func TestFullWorkflow_ReportToRejection(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}

	auditSvc := NewAuditService(incStore)
	appealSvc := NewAppealService(apStore, incStore, bot)

	inc, err := auditSvc.CreateFromSpam(context.Background(), Incident{
		Source:         SourceUserReport,
		IdempotencyKey: fmt.Sprintf("report-%d", time.Now().UnixNano()),
		ReasonCode:     ReasonUserReport,
		ReasonText:     "multiple user reports",
		Severity:       SeverityHigh,
		SpamUserID:     456,
		SpamUserName:   "reportedUser",
		ChatID:         100,
		MessageText:    "suspicious message",
	})
	require.NoError(t, err)
	assert.Equal(t, SeverityHigh, inc.Severity)

	ap, err := appealSvc.Submit(context.Background(), inc.ID, 456, "reportedUser", "this was taken out of context")
	require.NoError(t, err)

	err = appealSvc.Reject(context.Background(), ap.ID, "admin1", "spam confirmed after review")
	require.NoError(t, err)

	finalAp, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealRejected, finalAp.Status)

	finalInc, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusClosed, finalInc.Status)

	assert.Empty(t, bot.unbannedIDs, "rejected appeal should not unban")
	assert.Empty(t, bot.hamAdded, "rejected appeal should not add ham")
}

func TestFullWorkflow_EscalationPath(t *testing.T) {
	incStore := newMockIncidentStore()
	apStore := newMockAppealStore()
	bot := &mockBotService{}

	auditSvc := NewAuditService(incStore)
	appealSvc := NewAppealService(apStore, incStore, bot)

	inc, _ := auditSvc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: fmt.Sprintf("esc-%d", time.Now().UnixNano()),
		ReasonCode:     ReasonLLMOpenAI,
		Severity:       SeverityMedium,
		SpamUserID:     789,
		MessageText:    "edge case message",
	})

	ap, err := appealSvc.Submit(context.Background(), inc.ID, 789, "user", "dispute LLM classification")
	require.NoError(t, err)

	err = appealSvc.Triage(context.Background(), ap.ID, "mod1")
	require.NoError(t, err)

	err = appealSvc.Escalate(context.Background(), ap.ID)
	require.NoError(t, err)

	gotAp, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealEscalated, gotAp.Status)

	gotInc, _ := incStore.Get(context.Background(), inc.ID)
	assert.Equal(t, IncidentStatusReviewing, gotInc.Status)

	err = appealSvc.Accept(context.Background(), ap.ID, "senior-admin", "after senior review, confirmed false positive")
	require.NoError(t, err)

	gotAp2, _ := apStore.Get(context.Background(), ap.ID)
	assert.Equal(t, AppealAccepted, gotAp2.Status)
	assert.Contains(t, bot.unbannedIDs, int64(789))
}

func TestListIncidents_Filtering(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewAuditService(incStore)

	svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "f1", ReasonCode: ReasonRegexMatch, Severity: SeverityLow,
	})
	svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "f2", ReasonCode: ReasonUserReport, Severity: SeverityHigh,
	})
	inc3, _ := svc.CreateFromSpam(context.Background(), Incident{
		IdempotencyKey: "f3", ReasonCode: ReasonStopWord, Severity: SeverityLow,
	})
	svc.Resolve(context.Background(), inc3.ID, "admin", "done")

	open, err := svc.ListByStatus(context.Background(), IncidentStatusOpen, 10)
	require.NoError(t, err)
	assert.Len(t, open, 2)

	resolved, err := svc.ListByStatus(context.Background(), IncidentStatusResolved, 10)
	require.NoError(t, err)
	assert.Len(t, resolved, 1)
}
