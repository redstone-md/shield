package main

import (
	"context"

	"github.com/redstone-md/shield/app/storage"
)

type tenantStatusAdapter struct {
	inner *storage.Tenants
}

func (a tenantStatusAdapter) Status(ctx context.Context, tenantID string) (string, error) {
	rec, err := a.inner.Get(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return rec.Status, nil
}

type detectedSpamStoreAdapter struct {
	inner *storage.DetectedSpam
}

func (a detectedSpamStoreAdapter) Read(ctx context.Context, _ string) ([]storage.DetectedSpamInfo, error) {
	return a.inner.Read(ctx)
}

func (a detectedSpamStoreAdapter) FindByUserID(ctx context.Context, _ string, userID int64) (*storage.DetectedSpamInfo, error) {
	return a.inner.FindByUserID(ctx, userID)
}

func (a detectedSpamStoreAdapter) SetAddedToSamplesFlag(ctx context.Context, _ string, id int64) error {
	return a.inner.SetAddedToSamplesFlag(ctx, id)
}

type dictionaryFallbackAdapter struct {
	inner *storage.Dictionary
}

func (a dictionaryFallbackAdapter) Add(ctx context.Context, _ string, t storage.DictionaryType, data string) error {
	return a.inner.Add(ctx, t, data)
}

func (a dictionaryFallbackAdapter) Delete(ctx context.Context, _ string, id int64) error {
	return a.inner.Delete(ctx, id)
}

func (a dictionaryFallbackAdapter) Read(ctx context.Context, _ string, t storage.DictionaryType) ([]string, error) {
	return a.inner.Read(ctx, t)
}

func (a dictionaryFallbackAdapter) ReadWithIDs(
	ctx context.Context, _ string, t storage.DictionaryType,
) ([]storage.DictionaryEntry, error) {
	return a.inner.ReadWithIDs(ctx, t)
}

func (a dictionaryFallbackAdapter) Stats(ctx context.Context, _ string) (*storage.DictionaryStats, error) {
	return a.inner.Stats(ctx)
}
