package moderation

import "time"

// Action represents a policy outcome for a moderation event.
type Action string

const (
	ActionAllow    Action = "allow"
	ActionDelete   Action = "delete"
	ActionWarn     Action = "warn"
	ActionRestrict Action = "restrict"
	ActionBan      Action = "ban"
)

// Subject identifies the actor that produced an event.
type Subject struct {
	ID       int64
	UserName string
	IsBot    bool
}

// Content contains the normalized moderation payload.
type Content struct {
	Text       string
	Links      []string
	HasMedia   bool
	Attributes map[string]string
}

// IncomingEvent is the transport-neutral moderation input contract.
type IncomingEvent struct {
	EventID         string
	CorrelationID   string
	TenantID        string
	Source          string
	UpdateID        int
	ChatID          int64
	MessageID       int
	EditedMessageID int
	IdempotencyKey  string
	Subject         Subject
	Content         Content
	ReceivedAt      time.Time
}

// DetectionSignal records one spam heuristic or model signal.
type DetectionSignal struct {
	Name    string
	Score   float64
	Matched bool
	Reason  string
}

// DetectionResult captures the output of the detection stage.
type DetectionResult struct {
	EventID       string
	CorrelationID string
	Spam          bool
	Score         float64
	Signals       []DetectionSignal
	DetectedAt    time.Time
}

// PolicyDecision captures the policy layer outcome.
type PolicyDecision struct {
	EventID       string
	CorrelationID string
	Action        Action
	Reason        string
	Score         float64
	DecidedAt     time.Time
	PolicyVersion int
	ProfileName   string
}

// ModerationActionResult stores the executor result for a policy action.
type ModerationActionResult struct {
	EventID       string
	CorrelationID string
	Action        Action
	Applied       bool
	Provider      string
	Error         string
	AppliedAt     time.Time
}
