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

func TestDetectedSpamService_Read(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewDetectedSpam(context.Background(), db)
	require.NoError(t, err)

	svc := NewDetectedSpamService(store)

	entries, err := svc.Read(context.Background(), "t1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestDetectedSpamService_FindByUserID(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewDetectedSpam(context.Background(), db)
	require.NoError(t, err)

	svc := NewDetectedSpamService(store)

	entry, err := svc.FindByUserID(context.Background(), "t1", 999)
	require.NoError(t, err)
	assert.Nil(t, entry)
}

func TestDetectedSpamService_NilStore(t *testing.T) {
	svc := NewDetectedSpamService(nil)

	_, err := svc.Read(context.Background(), "t1")
	require.Error(t, err)

	_, err = svc.FindByUserID(context.Background(), "t1", 1)
	assert.Error(t, err)
}

func TestDetectedSpamService_OnChange(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewDetectedSpam(context.Background(), db)
	require.NoError(t, err)

	svc := NewDetectedSpamService(store)

	var notified atomic.Int32
	svc.OnChange(func() { notified.Add(1) })

	require.NoError(t, svc.SetAddedToSamplesFlag(context.Background(), "t1", 42))
	assert.Equal(t, int32(1), notified.Load())
}
