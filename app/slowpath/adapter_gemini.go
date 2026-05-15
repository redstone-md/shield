package slowpath

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

type GeminiAdapter struct {
	client GeminiClient
	model  string
	config GeminiAdapterConfig
}

type GeminiAdapterConfig struct {
	MaxOutputTokens   int32
	MaxSymbolsRequest int
}

type GeminiClient interface {
	GenerateContent(
		ctx context.Context,
		model string,
		contents []*genai.Content,
		config *genai.GenerateContentConfig,
	) (*genai.GenerateContentResponse, error)
}

func NewGeminiAdapter(client GeminiClient, model string, cfg GeminiAdapterConfig) *GeminiAdapter {
	if model == "" {
		model = "gemma-4-31b-it"
	}
	if cfg.MaxOutputTokens == 0 {
		cfg.MaxOutputTokens = 1024
	}
	if cfg.MaxSymbolsRequest == 0 {
		cfg.MaxSymbolsRequest = 8192
	}
	return &GeminiAdapter{client: client, model: model, config: cfg}
}

func (g *GeminiAdapter) Name() string { return "gemini" }

func (g *GeminiAdapter) Check(ctx context.Context, req ProviderRequest) (*ProviderResult, error) {
	start := time.Now()
	msg := req.Message
	if len(msg) > g.config.MaxSymbolsRequest {
		runes := []rune(msg)
		if len(runes) > g.config.MaxSymbolsRequest {
			msg = string(runes[:g.config.MaxSymbolsRequest])
		}
	}

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	parts := []*genai.Part{{Text: msg}}
	if len(req.ImageData) > 0 {
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{Data: req.ImageData, MIMEType: req.ImageMIME},
		})
	}

	contents := make([]*genai.Content, 0, 1+len(req.History))
	contents = append(contents, &genai.Content{Parts: parts, Role: "user"})
	for _, h := range req.History {
		contents = append(contents, &genai.Content{
			Parts: []*genai.Part{{Text: fmt.Sprintf("%s: %s", h.UserName, h.Text)}},
			Role:  "user",
		})
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens:   g.config.MaxOutputTokens,
		ResponseMIMEType:  "application/json",
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		SafetySettings: []*genai.SafetySetting{
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdOff},
		},
	}

	resp, err := g.client.GenerateContent(ctx, g.model, contents, config)
	latency := time.Since(start)
	if err != nil {
		return &ProviderResult{Provider: "gemini", Model: g.model, Latency: latency},
			fmt.Errorf("gemini generate: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return &ProviderResult{Provider: "gemini", Model: g.model, Latency: latency},
			fmt.Errorf("no candidates")
	}

	content := resp.Text()
	raw := content
	llmResp, parseErr := parseLLMOutput(content)
	if parseErr != nil {
		return &ProviderResult{
			Provider:    "gemini",
			Model:       g.model,
			Latency:     latency,
			RawResponse: raw,
		}, fmt.Errorf("parse response: %w", parseErr)
	}

	return &ProviderResult{
		Spam:          llmResp.IsSpam,
		Confidence:    llmResp.Confidence,
		Reason:        llmResp.Reason,
		Provider:      "gemini",
		Model:         g.model,
		Latency:       latency,
		RawResponse:   raw,
		PromptVersion: req.PromptVersion,
	}, nil
}

func (g *GeminiAdapter) Reply(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	start := time.Now()
	msg := req.Message
	if len(msg) > g.config.MaxSymbolsRequest {
		runes := []rune(msg)
		if len(runes) > g.config.MaxSymbolsRequest {
			msg = string(runes[:g.config.MaxSymbolsRequest])
		}
	}
	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultChatSystemPrompt
	}
	parts := []*genai.Part{{Text: msg}}
	contents := make([]*genai.Content, 0, 1+len(req.History))
	contents = append(contents, &genai.Content{Parts: parts, Role: "user"})
	for _, h := range req.History {
		contents = append(contents, &genai.Content{
			Parts: []*genai.Part{{Text: fmt.Sprintf("%s: %s", h.UserName, h.Text)}},
			Role:  "user",
		})
	}
	config := &genai.GenerateContentConfig{
		MaxOutputTokens:   g.config.MaxOutputTokens,
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
	}
	resp, err := g.client.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini chat: %w", err)
	}
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini chat: no candidates")
	}
	content := strings.TrimSpace(resp.Text())
	return &ChatResult{
		Text:          content,
		Provider:      "gemini",
		Model:         g.model,
		Latency:       time.Since(start),
		RawResponse:   content,
		PromptVersion: req.PromptVersion,
	}, nil
}

func (g *GeminiAdapter) AnalyzeImage(ctx context.Context, imageData []byte, mime, prompt string) (*ProviderResult, error) {
	start := time.Now()
	if prompt == "" {
		prompt = defaultVisionPrompt
	}

	parts := []*genai.Part{
		{Text: prompt},
		{InlineData: &genai.Blob{Data: imageData, MIMEType: mime}},
	}
	contents := []*genai.Content{{Parts: parts, Role: "user"}}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens:   g.config.MaxOutputTokens,
		ResponseMIMEType:  "application/json",
		SystemInstruction: genai.NewContentFromText(defaultSystemPrompt, genai.RoleUser),
		SafetySettings: []*genai.SafetySetting{
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdOff},
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdOff},
		},
	}

	resp, err := g.client.GenerateContent(ctx, g.model, contents, config)
	latency := time.Since(start)
	if err != nil {
		return &ProviderResult{Provider: "gemini", Model: g.model, Latency: latency}, fmt.Errorf("gemini vision: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return &ProviderResult{Provider: "gemini", Model: g.model, Latency: latency}, fmt.Errorf("gemini vision: no candidates")
	}

	content := resp.Text()
	llmResp, parseErr := parseLLMOutput(content)
	if parseErr != nil {
		return &ProviderResult{Provider: "gemini", Model: g.model, Latency: latency, RawResponse: content},
			fmt.Errorf("gemini vision parse: %w", parseErr)
	}

	return &ProviderResult{
		Spam:        llmResp.IsSpam,
		Confidence:  llmResp.Confidence,
		Reason:      llmResp.Reason,
		Provider:    "gemini",
		Model:       g.model,
		Latency:     latency,
		RawResponse: content,
	}, nil
}
