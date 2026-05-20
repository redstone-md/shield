package main

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/redstone-md/shield/app/controlplane"
	"github.com/redstone-md/shield/app/feedback"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/app/webapi"
)

type knowledgeDictAdapter struct {
	dict *storage.Dictionary
}

func (a *knowledgeDictAdapter) ReadStopPhrases(ctx context.Context) ([]string, error) {
	return a.dict.Read(ctx, storage.DictionaryTypeStopPhrase)
}

func (a *knowledgeDictAdapter) ReadIgnoredWords(_ context.Context) error {
	return nil
}

func (a *knowledgeDictAdapter) CountEntries(ctx context.Context) (int, error) {
	stats, err := a.dict.Stats(ctx)
	if err != nil {
		return 0, err
	}
	return stats.TotalStopPhrases + stats.TotalIgnoredWords, nil
}

type knowledgeSamplesAdapter struct {
	samples *storage.Samples
}

func (a *knowledgeSamplesAdapter) CountSamples(ctx context.Context) (spamCount, hamCount int, err error) {
	stats, err := a.samples.Stats(ctx)
	if err != nil {
		return 0, 0, err
	}
	return stats.TotalSpam, stats.TotalHam, nil
}

type usageMeterAdapter struct {
	store *storage.UsageMetering
}

func (a *usageMeterAdapter) Increment(ctx context.Context, meterType string) error {
	now := time.Now().UTC()
	windowStart := now.Truncate(time.Hour)
	windowEnd := windowStart.Add(time.Hour)
	return a.store.Increment(ctx, meterType, windowStart, windowEnd)
}

type onboardingAdapter struct {
	inner *controlplane.OnboardingService
}

func (a *onboardingAdapter) Onboard(ctx context.Context, req webapi.OnboardRequest) (*webapi.OnboardResult, error) {
	res, err := a.inner.Onboard(ctx, controlplane.OnboardRequest{
		TenantID: req.TenantID,
		Name:     req.Name,
		OwnerID:  req.OwnerID,
		GID:      req.GID,
	})
	if err != nil {
		return nil, err
	}
	return &webapi.OnboardResult{
		TenantID:    res.TenantID,
		WorkspaceID: res.WorkspaceID,
		RuleSetVer:  res.RuleSetVer,
	}, nil
}

func (a *onboardingAdapter) Offboard(ctx context.Context, tenantID string) error {
	return a.inner.Offboard(ctx, tenantID)
}

type restoreProviderAdapter struct {
	svc *storage.RestoreService
}

func (a *restoreProviderAdapter) RestoreTenant(ctx context.Context, tenantID string, r io.Reader) error {
	return a.svc.RestoreTenant(ctx, tenantID, r)
}

type sampleAdderAdapter struct {
	samples *storage.Samples
}

func (a *sampleAdderAdapter) AddSpamSample(ctx context.Context, text string) error {
	return a.samples.Add(ctx, storage.SampleTypeSpam, storage.SampleOriginUser, text)
}

func (a *sampleAdderAdapter) AddHamSample(ctx context.Context, text string) error {
	return a.samples.Add(ctx, storage.SampleTypeHam, storage.SampleOriginUser, text)
}

type spamTextProviderAdapter struct {
	store *storage.DetectedSpam
}

func (a *spamTextProviderAdapter) GetSpamText(ctx context.Context, spamID int64) (string, error) {
	entry, err := a.store.GetByID(ctx, spamID)
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", nil
	}
	return entry.Text, nil
}

type dictAdderAdapter struct {
	dict *storage.Dictionary
}

func (a *dictAdderAdapter) AddStopPhrase(ctx context.Context, phrase string) error {
	return a.dict.Add(ctx, storage.DictionaryTypeStopPhrase, phrase)
}

type stopPhraseRestorerAdapter struct {
	dict *storage.Dictionary
}

func (a *stopPhraseRestorerAdapter) ReadStopPhrases(ctx context.Context) ([]string, error) {
	return a.dict.Read(ctx, storage.DictionaryTypeStopPhrase)
}

func (a *stopPhraseRestorerAdapter) DeleteStopPhrases(ctx context.Context) error {
	_, err := a.dict.DeleteByType(ctx, storage.DictionaryTypeStopPhrase)
	return err
}

func (a *stopPhraseRestorerAdapter) AddStopPhrase(ctx context.Context, phrase string) error {
	return a.dict.Add(ctx, storage.DictionaryTypeStopPhrase, phrase)
}

type candidateGenerator interface {
	GenerateFromSpamText(ctx context.Context, sourceID int64, text string) ([]feedback.CandidateEntry, error)
}

type autoLearnerAdapter struct {
	samples   feedback.SampleAdder
	reviewSvc candidateGenerator
}

func (a *autoLearnerAdapter) LearnSpam(ctx context.Context, text, _ string) {
	if text == "" {
		return
	}
	if a.samples != nil {
		if err := a.samples.AddSpamSample(ctx, text); err != nil {
			log.Printf("[WARN] auto-learn: add spam sample failed: %v", err)
		}
	}
	if a.reviewSvc != nil {
		if _, err := a.reviewSvc.GenerateFromSpamText(ctx, 0, text); err != nil {
			log.Printf("[WARN] auto-learn: candidate generation failed: %v", err)
		}
	}
}

func (a *autoLearnerAdapter) LearnHam(ctx context.Context, text, _ string) {
	if text == "" {
		return
	}
	if a.samples != nil {
		if err := a.samples.AddHamSample(ctx, text); err != nil {
			log.Printf("[WARN] auto-learn: add ham sample failed: %v", err)
		}
	}
}

type candidateGeneratorAdapter struct {
	svc *feedback.ReviewService
}

func (a *candidateGeneratorAdapter) GenerateCandidates(ctx context.Context, text string) {
	if text == "" {
		return
	}
	if _, err := a.svc.GenerateFromSpamText(ctx, 0, text); err != nil {
		log.Printf("[WARN] auto candidate generation failed: %v", err)
	}
}
