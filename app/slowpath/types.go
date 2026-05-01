package slowpath

import "time"

type EscalationReason string

const (
	EscalationAmbiguousFast  EscalationReason = "ambiguous_fast"
	EscalationImageContent   EscalationReason = "image_content"
	EscalationUserReport     EscalationReason = "user_report"
	EscalationHighRiskPolicy EscalationReason = "high_risk_policy"
	EscalationForceLLM       EscalationReason = "force_llm"
)

type BudgetClass string

const (
	BudgetClassStandard BudgetClass = "standard"
	BudgetClassHigh     BudgetClass = "high"
	BudgetClassPremium  BudgetClass = "premium"
)

type SlowPathRequest struct {
	EventID       string
	CorrelationID string
	TenantID      string
	Reason        EscalationReason
	FastResult    DetectionResult
	Content       Content
	PromptVersion string
	BudgetClass   BudgetClass
	ImageData     []byte
	ImageMIME     string
}

type DetectionResult struct {
	Spam    bool
	Score   float64
	Signals []DetectionSignal
}

type DetectionSignal struct {
	Name    string
	Score   float64
	Matched bool
	Reason  string
}

type Content struct {
	Text     string
	HasMedia bool
}

type ProviderRequest struct {
	Message       string
	History       []HistoryMessage
	SystemPrompt  string
	CustomPrompts []string
	PromptVersion string
	ImageData     []byte
	ImageMIME     string
}

type HistoryMessage struct {
	UserName string
	Text     string
}

type ProviderResult struct {
	Spam          bool
	Confidence    int
	Reason        string
	Provider      string
	Model         string
	InputTokens   int
	OutputTokens  int
	Latency       time.Duration
	RawResponse   string
	PromptVersion string
}

type PromptEntry struct {
	ID            string
	Version       string
	Provider      string
	SystemPrompt  string
	CustomPrompts []string
	CreatedAt     time.Time
	Active        bool
}

type SlowPathInvocation struct {
	EventID       string
	CorrelationID string
	TenantID      string
	Provider      string
	Model         string
	PromptVersion string
	Reason        EscalationReason
	InputTokens   int
	OutputTokens  int
	LatencyMs     int64
	CostEstimate  float64
	RawResponse   string
	Spam          bool
	Confidence    int
	Timestamp     time.Time
}

type SlowPathResult struct {
	EventID       string
	CorrelationID string
	Providers     []string
	Spam          bool
	Confidence    int
	Reason        string
	Final         bool
	Skipped       bool
	Signals       []ProviderResult
}
