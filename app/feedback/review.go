package feedback

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type DictionaryAdder interface {
	AddStopPhrase(ctx context.Context, phrase string) error
}

type ReviewService struct {
	store CandidateStore
	dict  DictionaryAdder
	svc   *Service
}

func NewReviewService(store CandidateStore, dict DictionaryAdder, svc *Service) *ReviewService {
	return &ReviewService{store: store, dict: dict, svc: svc}
}

func (r *ReviewService) GenerateFromIncident(ctx context.Context, incidentID int64, messageText string) ([]CandidateEntry, error) {
	if messageText == "" {
		return nil, nil
	}

	existing, err := r.store.List(ctx, CandidateFilter{SourceID: incidentID, Source: "incident", Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("check existing candidates: %w", err)
	}
	existingSet := make(map[string]bool)
	for _, e := range existing {
		existingSet[e.Value] = true
	}

	var candidates []CandidateEntry
	tokens := extractCandidateTokens(messageText)
	for _, token := range tokens {
		if existingSet[token] {
			continue
		}
		c := CandidateEntry{
			Type:     CandidateStopPhrase,
			Value:    token,
			Source:   "incident",
			SourceID: incidentID,
			Status:   CandidatePending,
		}
		created, createErr := r.store.Create(ctx, c)
		if createErr != nil {
			log.Printf("[WARN] failed to create candidate: %v", createErr)
			continue
		}
		candidates = append(candidates, created)
	}

	return candidates, nil
}

func (r *ReviewService) GenerateFromSpamText(ctx context.Context, spamID int64, messageText string) ([]CandidateEntry, error) {
	if messageText == "" {
		return nil, nil
	}

	var candidates []CandidateEntry
	tokens := extractCandidateTokens(messageText)
	for _, token := range tokens {
		c := CandidateEntry{
			Type:     CandidateStopPhrase,
			Value:    token,
			Source:   "detected_spam",
			SourceID: spamID,
			Status:   CandidatePending,
		}
		created, err := r.store.Create(ctx, c)
		if err != nil {
			log.Printf("[WARN] failed to create candidate: %v", err)
			continue
		}
		candidates = append(candidates, created)
	}

	return candidates, nil
}

func (r *ReviewService) Approve(ctx context.Context, candidateID int64, reviewer string) error {
	c, err := r.store.GetByID(ctx, candidateID)
	if err != nil {
		return fmt.Errorf("candidate %d not found: %w", candidateID, err)
	}
	if c.Status != CandidatePending {
		return fmt.Errorf("candidate %d is %s, expected pending", candidateID, c.Status)
	}

	if err = r.store.UpdateStatus(ctx, candidateID, CandidateApproved, reviewer, ""); err != nil {
		return fmt.Errorf("update candidate status: %w", err)
	}

	if r.dict != nil && c.Type == CandidateStopPhrase {
		if addErr := r.dict.AddStopPhrase(ctx, c.Value); addErr != nil {
			log.Printf("[WARN] failed to add stop phrase '%s': %v", c.Value, addErr)
		}
	}

	return nil
}

func (r *ReviewService) Reject(ctx context.Context, candidateID int64, reviewer string) error {
	c, err := r.store.GetByID(ctx, candidateID)
	if err != nil {
		return fmt.Errorf("candidate %d not found: %w", candidateID, err)
	}
	if c.Status != CandidatePending {
		return fmt.Errorf("candidate %d is %s, expected pending", candidateID, c.Status)
	}

	return r.store.UpdateStatus(ctx, candidateID, CandidateRejected, reviewer, "")
}

func (r *ReviewService) ListPending(ctx context.Context, limit, offset int) ([]CandidateEntry, error) {
	return r.store.List(ctx, CandidateFilter{Status: CandidatePending, Limit: limit, Offset: offset})
}

func (r *ReviewService) ListAll(ctx context.Context, filter CandidateFilter) ([]CandidateEntry, error) {
	return r.store.List(ctx, filter)
}

func extractCandidateTokens(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool)
	var result []string
	for _, w := range words {
		cleaned := strings.Trim(w, ".,!?;:\"'()[]{}<>/\\|@#$%^&*+=~`")
		if len(cleaned) < 4 || len(cleaned) > 50 {
			continue
		}
		if !seen[cleaned] {
			seen[cleaned] = true
			result = append(result, cleaned)
		}
	}
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}
