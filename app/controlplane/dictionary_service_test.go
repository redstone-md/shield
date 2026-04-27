package controlplane

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
)

type mockSampleReloader struct {
	reloadCalled atomic.Int32
	reloadErr    error
}

func (m *mockSampleReloader) ReloadSamples() error {
	m.reloadCalled.Add(1)
	return m.reloadErr
}

func TestDictionaryService_Add(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	dict, err := storage.NewDictionary(context.Background(), db)
	require.NoError(t, err)

	rl := &mockSampleReloader{}
	svc := NewDictionaryService(dict, rl)

	err = svc.Add(context.Background(), storage.DictionaryTypeStopPhrase, "buy now")
	require.NoError(t, err)
	assert.Equal(t, int32(1), rl.reloadCalled.Load())

	entries, err := svc.Read(context.Background(), storage.DictionaryTypeStopPhrase)
	require.NoError(t, err)
	assert.Equal(t, []string{"buy now"}, entries)
}

func TestDictionaryService_Delete(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	dict, err := storage.NewDictionary(context.Background(), db)
	require.NoError(t, err)

	rl := &mockSampleReloader{}
	svc := NewDictionaryService(dict, rl)

	err = svc.Add(context.Background(), storage.DictionaryTypeStopPhrase, "buy now")
	require.NoError(t, err)

	withIDs, err := svc.ReadWithIDs(context.Background(), storage.DictionaryTypeStopPhrase)
	require.NoError(t, err)
	require.Len(t, withIDs, 1)

	err = svc.Delete(context.Background(), withIDs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), rl.reloadCalled.Load())

	entries, err := svc.Read(context.Background(), storage.DictionaryTypeStopPhrase)
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestDictionaryService_Validation(t *testing.T) {
	svc := NewDictionaryService(nil, nil)

	err := svc.Add(context.Background(), storage.DictionaryTypeStopPhrase, "")
	assert.EqualError(t, err, "data cannot be empty")

	err = svc.Add(context.Background(), storage.DictionaryType("invalid"), "data")
	assert.Error(t, err)
}

func TestDictionaryService_OnChange(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	dict, err := storage.NewDictionary(context.Background(), db)
	require.NoError(t, err)

	rl := &mockSampleReloader{}
	svc := NewDictionaryService(dict, rl)

	var notified atomic.Int32
	svc.OnChange(func() { notified.Add(1) })
	svc.OnChange(func() { notified.Add(10) })

	err = svc.Add(context.Background(), storage.DictionaryTypeIgnoredWord, "the")
	require.NoError(t, err)
	assert.Equal(t, int32(11), notified.Load())

	withIDs, err := svc.ReadWithIDs(context.Background(), storage.DictionaryTypeIgnoredWord)
	require.NoError(t, err)
	require.Len(t, withIDs, 1)

	err = svc.Delete(context.Background(), withIDs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, int32(22), notified.Load())
}

func TestDictionaryService_Stats(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	dict, err := storage.NewDictionary(context.Background(), db)
	require.NoError(t, err)

	svc := NewDictionaryService(dict, &mockSampleReloader{})

	err = svc.Add(context.Background(), storage.DictionaryTypeStopPhrase, "spam1")
	require.NoError(t, err)
	err = svc.Add(context.Background(), storage.DictionaryTypeStopPhrase, "spam2")
	require.NoError(t, err)
	err = svc.Add(context.Background(), storage.DictionaryTypeIgnoredWord, "the")
	require.NoError(t, err)

	stats, err := svc.Stats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalStopPhrases)
	assert.Equal(t, 1, stats.TotalIgnoredWords)
}

func TestDictionaryService_NilStore(t *testing.T) {
	svc := NewDictionaryService(nil, nil)

	_, err := svc.Read(context.Background(), storage.DictionaryTypeStopPhrase)
	assert.Error(t, err)

	_, err = svc.ReadWithIDs(context.Background(), storage.DictionaryTypeStopPhrase)
	assert.Error(t, err)

	_, err = svc.Stats(context.Background())
	assert.Error(t, err)
}
