package audit

import (
	"context"
	"time"
)

type IncidentStore interface {
	Create(ctx context.Context, incident Incident) (Incident, error)
	Get(ctx context.Context, id int64) (Incident, error)
	GetByIdempotencyKey(ctx context.Context, tenantID, key string) (Incident, error)
	List(ctx context.Context, filter IncidentFilter) ([]Incident, error)
	UpdateStatus(ctx context.Context, id int64, status IncidentStatus, resolvedBy string) error
	UpdateSeverity(ctx context.Context, id int64, severity IncidentSeverity) error

	AddComment(ctx context.Context, comment IncidentComment) (IncidentComment, error)
	ListComments(ctx context.Context, incidentID int64) ([]IncidentComment, error)
}

type AppealStore interface {
	Create(ctx context.Context, appeal Appeal) (Appeal, error)
	Get(ctx context.Context, id int64) (Appeal, error)
	GetByIncident(ctx context.Context, incidentID int64) (Appeal, error)
	List(ctx context.Context, filter AppealFilter) ([]Appeal, error)
	UpdateStatus(ctx context.Context, id int64, status AppealStatus, resolvedBy, resolutionText string) error
	UpdateReplayResult(ctx context.Context, id int64, result ReplayResult) error
}

type IncidentFilter struct {
	TenantID  string
	Status    IncidentStatus
	Source    IncidentSource
	Severity  IncidentSeverity
	Reason    ReasonCode
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

type AppealFilter struct {
	TenantID string
	Status   AppealStatus
	Limit    int
	Offset   int
}

type ReplayResult struct {
	DetectionSpam   bool    `json:"detection_spam"`
	DetectionScore  float64 `json:"detection_score"`
	PolicyAction    string  `json:"policy_action"`
	PolicyReason    string  `json:"policy_reason"`
	PolicyScore     float64 `json:"policy_score"`
	ReplayTimestamp string  `json:"replay_timestamp"`
}
