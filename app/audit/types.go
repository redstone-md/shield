package audit

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type IncidentStatus string

const (
	IncidentStatusOpen      IncidentStatus = "open"
	IncidentStatusReviewing IncidentStatus = "reviewing"
	IncidentStatusResolved  IncidentStatus = "resolved"
	IncidentStatusAppealed  IncidentStatus = "appealed"
	IncidentStatusClosed    IncidentStatus = "closed"
)

type IncidentSource string

const (
	SourceAutoMod     IncidentSource = "auto_mod"
	SourceUserReport  IncidentSource = "user_report"
	SourceAdminAction IncidentSource = "admin_action"
	SourceAppeal      IncidentSource = "appeal"
)

type IncidentSeverity string

const (
	SeverityLow      IncidentSeverity = "low"
	SeverityMedium   IncidentSeverity = "medium"
	SeverityHigh     IncidentSeverity = "high"
	SeverityCritical IncidentSeverity = "critical"
)

type ReasonCode string

const (
	ReasonRegexMatch      ReasonCode = "regex_match"
	ReasonStopWord        ReasonCode = "stop_word"
	ReasonSimilarity      ReasonCode = "similarity"
	ReasonCAS             ReasonCode = "cas"
	ReasonMetaLink        ReasonCode = "meta_link"
	ReasonMetaMention     ReasonCode = "meta_mention"
	ReasonMultiLang       ReasonCode = "multi_lang"
	ReasonAbnormalSpacing ReasonCode = "abnormal_spacing"
	ReasonEmojiSpam       ReasonCode = "emoji_spam"
	ReasonLLMOpenAI       ReasonCode = "llm_openai"
	ReasonLLMGemini       ReasonCode = "llm_gemini"
	ReasonVision          ReasonCode = "vision"
	ReasonUserReport      ReasonCode = "user_report"
	ReasonAdminAction     ReasonCode = "admin_action"
	ReasonEscalation      ReasonCode = "escalation"
	ReasonPolicyRule      ReasonCode = "policy_rule"
	ReasonUnknown         ReasonCode = "unknown"
)

type AppealStatus string

const (
	AppealNew       AppealStatus = "new"
	AppealTriaged   AppealStatus = "triaged"
	AppealAccepted  AppealStatus = "accepted"
	AppealRejected  AppealStatus = "rejected"
	AppealReplayed  AppealStatus = "replayed"
	AppealEscalated AppealStatus = "escalated"
)

type Incident struct {
	ID             int64
	GID            string
	TenantID       string
	Source         IncidentSource
	Status         IncidentStatus
	Severity       IncidentSeverity
	IdempotencyKey string
	DetectedSpamID int64
	ReportID       int64
	ReasonCode     ReasonCode
	ReasonText     string
	SpamUserID     int64
	SpamUserName   string
	ChatID         int64
	MessageText    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ResolvedAt     *time.Time
	ResolvedBy     string
	Comment        string
}

type IncidentComment struct {
	ID         int64
	IncidentID int64
	AuthorType string
	AuthorID   string
	Action     string
	Payload    string
	CreatedAt  time.Time
}

type Appeal struct {
	ID              int64
	IncidentID      int64
	GID             string
	TenantID        string
	AppellantUserID int64
	AppellantName   string
	Status          AppealStatus
	AppealText      string
	ResolutionText  string
	ResolvedBy      string
	ReplayResult    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ResolvedAt      *time.Time
}

type SpamCheckResult struct {
	Name    string
	Spam    bool
	Details string
}

type AuditEventData struct {
	IdempotencyKey  string
	ChatID          int64
	RuleSetVersion  int
	SpamUserID      int64
	SpamUserName    string
	MessageText     string
	CheckResults    []SpamCheckResult
	SlowPathInvoked bool
	SlowProvider    string
	SlowPromptVer   string
}

func ClassifySeverity(reason ReasonCode) IncidentSeverity {
	switch reason {
	case ReasonUserReport, ReasonAdminAction:
		return SeverityHigh
	case ReasonLLMOpenAI, ReasonLLMGemini, ReasonVision:
		return SeverityMedium
	case ReasonEscalation:
		return SeverityCritical
	default:
		return SeverityLow
	}
}

func MapCheckNameToReason(name string) ReasonCode {
	mapping := map[string]ReasonCode{
		"regex":      ReasonRegexMatch,
		"stop word":  ReasonStopWord,
		"similarity": ReasonSimilarity,
		"cas":        ReasonCAS,
		"links":      ReasonMetaLink,
		"mentions":   ReasonMetaMention,
		"multi-lang": ReasonMultiLang,
		"spacing":    ReasonAbnormalSpacing,
		"emoji":      ReasonEmojiSpam,
		"openai":     ReasonLLMOpenAI,
		"gemini":     ReasonLLMGemini,
		"vision":     ReasonVision,
	}
	if rc, ok := mapping[name]; ok {
		return rc
	}
	return ReasonUnknown
}
