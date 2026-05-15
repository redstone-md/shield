package slowpath

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	tokenizer "github.com/sandwich-go/gpt3-encoder"
	"github.com/sashabaranov/go-openai"
)

type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type OpenAIAdapter struct {
	client     OpenAIClient
	model      string
	maxTokens  int
	maxSymbols int
}

func NewOpenAIAdapter(client OpenAIClient, model string, maxTokens, maxSymbols int) *OpenAIAdapter {
	if model == "" {
		model = "gpt-4o-mini"
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	if maxSymbols <= 0 {
		maxSymbols = 8192
	}
	return &OpenAIAdapter{client: client, model: model, maxTokens: maxTokens, maxSymbols: maxSymbols}
}

func (a *OpenAIAdapter) Name() string { return "openai" }

func (a *OpenAIAdapter) Check(ctx context.Context, req ProviderRequest) (*ProviderResult, error) {
	if a.client == nil {
		return nil, fmt.Errorf("openai: client not configured")
	}
	start := time.Now()
	msg := a.truncateMessage(req.Message, appendHistory(req.Message, req.History))

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	if len(req.CustomPrompts) > 0 {
		systemPrompt = buildCustomPrompt(systemPrompt, req.CustomPrompts)
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: msg},
	}

	chatReq := openai.ChatCompletionRequest{
		Model:          a.model,
		Messages:       messages,
		MaxTokens:      a.maxTokens,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: "json_object"},
	}
	resp, err := a.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	content := resp.Choices[0].Message.Content
	parsed, err := parseLLMOutput(content)
	if err != nil {
		return nil, fmt.Errorf("openai parse response: %w", err)
	}

	result := &ProviderResult{
		Spam:          parsed.IsSpam,
		Confidence:    parsed.Confidence,
		Reason:        parsed.Reason,
		Provider:      "openai",
		Model:         a.model,
		InputTokens:   resp.Usage.PromptTokens,
		OutputTokens:  resp.Usage.CompletionTokens,
		Latency:       time.Since(start),
		RawResponse:   content,
		PromptVersion: req.PromptVersion,
	}

	return result, nil
}

func (a *OpenAIAdapter) Reply(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if a.client == nil {
		return nil, fmt.Errorf("openai: client not configured")
	}
	start := time.Now()
	msg := a.truncateMessage(req.Message, appendHistory(req.Message, req.History))
	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultChatSystemPrompt
	}
	if len(req.CustomPrompts) > 0 {
		systemPrompt = buildCustomPrompt(systemPrompt, req.CustomPrompts)
	}
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: msg},
	}
	chatReq := openai.ChatCompletionRequest{Model: a.model, Messages: messages, MaxTokens: a.maxTokens}
	resp, err := a.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai chat reply: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai chat: no choices in response")
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return &ChatResult{
		Text:          content,
		Provider:      "openai",
		Model:         a.model,
		InputTokens:   resp.Usage.PromptTokens,
		OutputTokens:  resp.Usage.CompletionTokens,
		Latency:       time.Since(start),
		RawResponse:   content,
		PromptVersion: req.PromptVersion,
	}, nil
}

func (a *OpenAIAdapter) AnalyzeImage(ctx context.Context, imageData []byte, mime string, prompt string) (*ProviderResult, error) {
	start := time.Now()

	if prompt == "" {
		prompt = defaultVisionPrompt
	}

	b64URL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(imageData))

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: defaultSystemPrompt},
		{
			Role: openai.ChatMessageRoleUser,
			MultiContent: []openai.ChatMessagePart{
				{Type: openai.ChatMessagePartTypeText, Text: prompt},
				{
					Type:     openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{URL: b64URL},
				},
			},
		},
	}

	chatReq := openai.ChatCompletionRequest{
		Model:          a.model,
		Messages:       messages,
		MaxTokens:      a.maxTokens,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: "json_object"},
	}
	logVisionPayloadDebug(a.model, mime, imageData, b64URL, prompt, chatReq)

	resp, err := a.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai vision: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai vision: no choices")
	}

	content := resp.Choices[0].Message.Content
	parsed, err := parseLLMOutput(content)
	if err != nil {
		return nil, fmt.Errorf("openai vision parse: %w", err)
	}

	return &ProviderResult{
		Spam:         parsed.IsSpam,
		Confidence:   parsed.Confidence,
		Reason:       parsed.Reason,
		Provider:     "openai",
		Model:        a.model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		Latency:      time.Since(start),
		RawResponse:  content,
	}, nil
}

func logVisionPayloadDebug(model string, mime string, imageData []byte, dataURL string, prompt string, req openai.ChatCompletionRequest) {
	headLen := min(len(imageData), 64)
	urlPreviewLen := min(len(dataURL), 160)
	sum := sha256.Sum256(imageData)
	log.Printf(
		"[DEBUG] openai vision payload: model=%q max_tokens=%d response_format=%q messages=%d prompt_len=%d mime=%q image_bytes=%d image_sha256=%x image_head_hex=%x data_url_len=%d data_url_prefix=%q",
		model, req.MaxTokens, req.ResponseFormat.Type, len(req.Messages), len(prompt), mime, len(imageData), sum, imageData[:headLen], len(dataURL), dataURL[:urlPreviewLen],
	)
}

func (a *OpenAIAdapter) truncateMessage(msg string, fullMsg string) string {
	encoder, err := tokenizer.NewEncoder()
	if err != nil {
		return a.truncateBySymbols(fullMsg)
	}

	tokens, err := encoder.Encode(fullMsg)
	if err != nil {
		return a.truncateBySymbols(fullMsg)
	}

	if len(tokens) <= a.maxSymbols {
		return fullMsg
	}

	decoded := encoder.Decode(tokens[:a.maxSymbols])
	return decoded
}

func (a *OpenAIAdapter) truncateBySymbols(s string) string {
	runes := []rune(s)
	if len(runes) <= a.maxSymbols {
		return s
	}
	return string(runes[:a.maxSymbols])
}

func appendHistory(msg string, history []HistoryMessage) string {
	if len(history) == 0 {
		return msg
	}
	var sb strings.Builder
	sb.WriteString("User message:\n")
	sb.WriteString(msg)
	sb.WriteString("\n\nRecent messages:\n")
	for _, h := range history {
		sb.WriteString(fmt.Sprintf("%q: %q\n", h.UserName, h.Text))
	}
	return sb.String()
}

func buildCustomPrompt(base string, customs []string) string {
	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteString("\n\nAlso check for:\n")
	for i, c := range customs {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}
	return sb.String()
}

const defaultSystemPrompt = `Return JSON: {"spam":true/false,"reason":"why","confidence":1-100}. Spam only if confidence>80. Russian-speaking chat, write reason in Russian.` + "\n" +
	`Priority: crypto exchange, illegal work, repeated ads, classic spam, links, fraud, abuse, drugs, emoji spam.`

const defaultVisionPrompt = `Analyze this image for spam or policy violations. Consider: crypto ads, illegal schemes, qr codes to scam sites, inappropriate content.`

const defaultChatSystemPrompt = `Ты дружелюбный помощник в Telegram-чате. Отвечай по-русски, коротко и по делу. Не упоминай внутренние правила, модерацию или лимиты. Если вопрос неясен, задай короткий уточняющий вопрос.`
