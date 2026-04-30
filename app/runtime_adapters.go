package main

import (
	"context"
	"io"
	"time"

	"github.com/umputun/tg-spam/app/controlplane"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi"
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

func (a *knowledgeSamplesAdapter) CountSamples(ctx context.Context) (int, int, error) {
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

type quotaLimitAdapter struct {
	inner *storage.TenantLimits
}

func (a *quotaLimitAdapter) Get(ctx context.Context, limitType string) (int, int, error) {
	rec, err := a.inner.Get(ctx, limitType)
	if err != nil {
		return 0, 0, err
	}
	return rec.LimitValue, rec.CurrentUsage, nil
}

func (a *quotaLimitAdapter) Increment(ctx context.Context, limitType string) error {
	return a.inner.Increment(ctx, limitType)
}

func (a *quotaLimitAdapter) Set(ctx context.Context, limitType string, limitValue int) error {
	return a.inner.Set(ctx, limitType, limitValue)
}
