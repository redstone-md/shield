package controlplane

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/redstone-md/shield/lib/approved"
)

type mockApprovedDetector struct {
	store   *storage.ApprovedUsers
	added   []approved.UserInfo
	removed []string
}

func (m *mockApprovedDetector) AddApprovedUser(user approved.UserInfo) error {
	m.added = append(m.added, user)
	ctx := context.Background()
	return m.store.Write(ctx, user)
}

func (m *mockApprovedDetector) RemoveApprovedUser(id string) error {
	m.removed = append(m.removed, id)
	ctx := context.Background()
	return m.store.Delete(ctx, id)
}

func (m *mockApprovedDetector) ApprovedUsers() []approved.UserInfo {
	return nil
}

func TestApprovedUsersService_Add(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewApprovedUsers(context.Background(), db)
	require.NoError(t, err)

	det := &mockApprovedDetector{store: store}
	svc := NewApprovedUsersService(store, det)

	err = svc.Add(context.Background(), "t1", approved.UserInfo{UserID: "123", UserName: "alice"})
	require.NoError(t, err)
	assert.Len(t, det.added, 1)
	assert.Equal(t, "123", det.added[0].UserID)

	users, err := svc.List(context.Background(), "t1")
	require.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Equal(t, "123", users[0].UserID)
}

func TestApprovedUsersService_Remove(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewApprovedUsers(context.Background(), db)
	require.NoError(t, err)

	det := &mockApprovedDetector{store: store}
	svc := NewApprovedUsersService(store, det)

	err = svc.Add(context.Background(), "t1", approved.UserInfo{UserID: "123", UserName: "alice"})
	require.NoError(t, err)

	err = svc.Remove(context.Background(), "t1", "123")
	require.NoError(t, err)
	assert.Len(t, det.removed, 1)
	assert.Equal(t, "123", det.removed[0])

	users, err := svc.List(context.Background(), "t1")
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestApprovedUsersService_Validation(t *testing.T) {
	svc := NewApprovedUsersService(nil, nil)

	err := svc.Add(context.Background(), "t1", approved.UserInfo{UserID: ""})
	require.EqualError(t, err, "user id is required")

	err = svc.Remove(context.Background(), "t1", "")
	assert.EqualError(t, err, "user id is required")
}

func TestApprovedUsersService_OnChange(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewApprovedUsers(context.Background(), db)
	require.NoError(t, err)

	det := &mockApprovedDetector{store: store}
	svc := NewApprovedUsersService(store, det)

	var notified atomic.Int32
	svc.OnChange(func() { notified.Add(1) })
	svc.OnChange(func() { notified.Add(10) })

	err = svc.Add(context.Background(), "t1", approved.UserInfo{UserID: "1"})
	require.NoError(t, err)
	assert.Equal(t, int32(11), notified.Load())

	err = svc.Remove(context.Background(), "t1", "1")
	require.NoError(t, err)
	assert.Equal(t, int32(22), notified.Load())
}

func TestApprovedUsersService_ListError(t *testing.T) {
	svc := NewApprovedUsersService(nil, nil)
	_, err := svc.List(context.Background(), "t1")
	assert.Error(t, err)
}
