package slowpath

import (
	"context"
	"fmt"
	"log"
	"time"
)

type EngineConfig struct {
	DefaultProvider string
	CostPerToken    float64
}

type Engine struct {
	providers     map[string]LLMProvider
	chat          map[string]ChatProvider
	vision        map[string]VisionProvider
	breakers      map[string]*ProviderBreaker
	budget        BudgetTracker
	systemPrompts map[string]string
	visionPrompt  string
	config        EngineConfig
}

func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		providers:     make(map[string]LLMProvider),
		chat:          make(map[string]ChatProvider),
		vision:        make(map[string]VisionProvider),
		breakers:      make(map[string]*ProviderBreaker),
		systemPrompts: make(map[string]string),
		config:        cfg,
	}
}

func (e *Engine) RegisterProvider(p LLMProvider, breakerCfg BreakerConfig) {
	name := p.Name()
	e.providers[name] = p
	e.breakers[name] = NewProviderBreaker(name, breakerCfg)
}

func (e *Engine) RegisterChat(p ChatProvider, breakerCfg BreakerConfig) {
	name := p.Name()
	e.chat[name] = p
	if _, exists := e.breakers[name]; !exists {
		e.breakers[name] = NewProviderBreaker(name, breakerCfg)
	}
}

func (e *Engine) RegisterVision(p VisionProvider, breakerCfg BreakerConfig) {
	name := p.Name()
	e.vision[name] = p
	if _, exists := e.breakers[name]; !exists {
		e.breakers[name] = NewProviderBreaker(name, breakerCfg)
	}
}

func (e *Engine) SetBudgetTracker(bt BudgetTracker) { e.budget = bt }

// SetSystemPrompt sets the system prompt used for text checks of the given provider.
func (e *Engine) SetSystemPrompt(provider, prompt string) { e.systemPrompts[provider] = prompt }

// SetVisionPrompt sets the system prompt used for vision (image) checks.
func (e *Engine) SetVisionPrompt(prompt string) { e.visionPrompt = prompt }

// visionPromptOrDefault returns the configured vision prompt, or fallback when unset.
func (e *Engine) visionPromptOrDefault(fallback string) string {
	if e.visionPrompt != "" {
		return e.visionPrompt
	}
	return fallback
}

func (e *Engine) Check(ctx context.Context, req SlowPathRequest) (*SlowPathResult, error) {
	if len(req.ImageData) > 0 {
		return e.checkVision(ctx, req)
	}
	return e.checkText(ctx, req)
}

func (e *Engine) Reply(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if len(e.chat) == 0 {
		return nil, fmt.Errorf("chat replies disabled")
	}
	provider := e.resolveChatProvider()
	cp, ok := e.chat[provider]
	if !ok {
		return nil, fmt.Errorf("no chat provider: %s", provider)
	}

	systemPrompt, customPrompts, promptVersion, err := e.resolvePrompt(provider, req.PromptVersion)
	if err != nil {
		log.Printf("[WARN] slowpath prompt resolve: %v", err)
	}
	chatReq := ChatRequest{
		EventID:       req.EventID,
		CorrelationID: req.CorrelationID,
		TenantID:      req.TenantID,
		Message:       req.Message,
		History:       req.History,
		SystemPrompt:  systemPrompt,
		CustomPrompts: customPrompts,
		PromptVersion: promptVersion,
	}
	return e.callChatWithBreaker(ctx, provider, chatReq, cp)
}

func (e *Engine) checkText(ctx context.Context, req SlowPathRequest) (*SlowPathResult, error) {
	provider := e.resolveProvider(req)
	p, ok := e.providers[provider]
	if !ok {
		return nil, fmt.Errorf("no LLM provider: %s", provider)
	}

	systemPrompt, customPrompts, promptVersion, err := e.resolvePrompt(provider, req.PromptVersion)
	if err != nil {
		log.Printf("[WARN] slowpath prompt resolve: %v", err)
	}

	if !e.checkBudget(req.TenantID, req.BudgetClass, 0) {
		return &SlowPathResult{EventID: req.EventID, Providers: []string{provider}, Skipped: true}, nil
	}

	provReq := ProviderRequest{
		Message:       req.Content.Text,
		History:       e.extractHistory(req),
		SystemPrompt:  systemPrompt,
		CustomPrompts: customPrompts,
		PromptVersion: promptVersion,
	}

	result, err := e.callWithBreaker(ctx, provider, provReq, p)
	if err != nil {
		return nil, err
	}

	e.recordUsage(req.TenantID, req.BudgetClass, result)

	return &SlowPathResult{
		EventID:       req.EventID,
		CorrelationID: req.CorrelationID,
		Providers:     []string{provider},
		Spam:          result.Spam,
		Confidence:    result.Confidence,
		Reason:        result.Reason,
		Final:         true,
		Signals:       []ProviderResult{*result},
	}, nil
}

func (e *Engine) checkVision(ctx context.Context, req SlowPathRequest) (*SlowPathResult, error) {
	provider := e.resolveProvider(req)
	v, ok := e.vision[provider]
	if !ok {
		if p, ok2 := e.providers[provider]; ok2 {
			provReq := ProviderRequest{
				Message:       req.Content.Text,
				SystemPrompt:  defaultSystemPrompt,
				PromptVersion: req.PromptVersion,
				ImageData:     req.ImageData,
				ImageMIME:     req.ImageMIME,
			}
			if !e.checkBudget(req.TenantID, req.BudgetClass, 0) {
				return &SlowPathResult{EventID: req.EventID, Skipped: true}, nil
			}
			result, err := e.callWithBreaker(ctx, provider, provReq, p)
			if err != nil {
				return nil, err
			}
			e.recordUsage(req.TenantID, req.BudgetClass, result)
			return &SlowPathResult{
				EventID:       req.EventID,
				CorrelationID: req.CorrelationID,
				Providers:     []string{provider},
				Spam:          result.Spam,
				Confidence:    result.Confidence,
				Reason:        result.Reason,
				Final:         true,
				Signals:       []ProviderResult{*result},
			}, nil
		}
		return nil, fmt.Errorf("no vision provider: %s", provider)
	}

	if !e.checkBudget(req.TenantID, req.BudgetClass, 0) {
		return &SlowPathResult{EventID: req.EventID, Skipped: true}, nil
	}

	prompt := e.visionPromptOrDefault(defaultVisionPrompt)
	result, err := e.callVisionWithBreaker(ctx, provider, req.ImageData, req.ImageMIME, prompt, v)
	if err != nil {
		return nil, err
	}

	e.recordUsage(req.TenantID, req.BudgetClass, result)

	return &SlowPathResult{
		EventID:       req.EventID,
		CorrelationID: req.CorrelationID,
		Providers:     []string{provider},
		Spam:          result.Spam,
		Confidence:    result.Confidence,
		Reason:        result.Reason,
		Final:         true,
		Signals:       []ProviderResult{*result},
	}, nil
}

func (e *Engine) callWithBreaker(
	ctx context.Context, provider string, req ProviderRequest, p LLMProvider,
) (*ProviderResult, error) {
	brk, ok := e.breakers[provider]
	if !ok {
		return p.Check(ctx, req)
	}
	return brk.Execute(ctx, func(ctx context.Context) (*ProviderResult, error) {
		return p.Check(ctx, req)
	})
}

func (e *Engine) callChatWithBreaker(ctx context.Context, provider string, req ChatRequest, p ChatProvider) (*ChatResult, error) {
	brk, ok := e.breakers[provider]
	if !ok {
		return p.Reply(ctx, req)
	}
	var result *ChatResult
	_, err := brk.Execute(ctx, func(ctx context.Context) (*ProviderResult, error) {
		res, err := p.Reply(ctx, req)
		if err != nil {
			return nil, err
		}
		result = res
		return &ProviderResult{}, nil
	})
	return result, err
}

func (e *Engine) callVisionWithBreaker(
	ctx context.Context, provider string, imgData []byte, mime, prompt string, v VisionProvider,
) (*ProviderResult, error) {
	brk, ok := e.breakers[provider]
	if !ok {
		return v.AnalyzeImage(ctx, imgData, mime, prompt)
	}
	return brk.Execute(ctx, func(ctx context.Context) (*ProviderResult, error) {
		return v.AnalyzeImage(ctx, imgData, mime, prompt)
	})
}

func (e *Engine) resolveProvider(_ SlowPathRequest) string {
	if e.config.DefaultProvider != "" {
		return e.config.DefaultProvider
	}
	for name := range e.providers {
		return name
	}
	for name := range e.vision {
		return name
	}
	return ""
}

func (e *Engine) resolveChatProvider() string {
	for name := range e.chat {
		return name
	}
	return ""
}

func (e *Engine) resolvePrompt(provider, version string) (system string, customs []string, ver string, err error) {
	return e.systemPrompts[provider], nil, version, nil
}

func (e *Engine) checkBudget(tenantID string, class BudgetClass, estimatedTokens int) bool {
	if e.budget == nil {
		return true
	}
	return e.budget.Allow(tenantID, class, estimatedTokens)
}

func (e *Engine) recordUsage(tenantID string, class BudgetClass, result *ProviderResult) {
	if e.budget == nil {
		return
	}
	tokens := result.InputTokens + result.OutputTokens
	cost := float64(tokens) * e.config.CostPerToken
	e.budget.Record(tenantID, class, tokens, cost)
}

func (e *Engine) extractHistory(_ SlowPathRequest) []HistoryMessage {
	return nil
}

func (e *Engine) InvocationFromResult(req SlowPathRequest, result *ProviderResult) SlowPathInvocation {
	inv := SlowPathInvocation{
		EventID:       req.EventID,
		CorrelationID: req.CorrelationID,
		TenantID:      req.TenantID,
		Provider:      result.Provider,
		Model:         result.Model,
		PromptVersion: result.PromptVersion,
		Reason:        req.Reason,
		InputTokens:   result.InputTokens,
		OutputTokens:  result.OutputTokens,
		LatencyMs:     result.Latency.Milliseconds(),
		RawResponse:   result.RawResponse,
		Spam:          result.Spam,
		Confidence:    result.Confidence,
		Timestamp:     time.Now(),
	}
	if e.config.CostPerToken > 0 {
		inv.CostEstimate = float64(result.InputTokens+result.OutputTokens) * e.config.CostPerToken
	}
	return inv
}
