package slowpath

import "context"

type LLMProvider interface {
	Name() string
	Check(ctx context.Context, req ProviderRequest) (*ProviderResult, error)
}

type VisionProvider interface {
	Name() string
	AnalyzeImage(ctx context.Context, imageData []byte, mime string, prompt string) (*ProviderResult, error)
}

type BudgetTracker interface {
	Allow(tenantID string, class BudgetClass, estimatedTokens int) bool
	Record(tenantID string, class BudgetClass, tokensUsed int, cost float64)
	Usage(tenantID string) (requests int, tokens int, cost float64)
}

type PromptRegistry interface {
	Active(provider string) (*PromptEntry, error)
	Get(provider string, version string) (*PromptEntry, error)
}
