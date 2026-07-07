package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage/engine"
)

// verifies per-user bulk deletion works on the sqlite path even when different
// users send identical text (which collapses to one row in the hash-keyed
// messages table) and when a user sends empty-text messages.
func TestLocator_GetUserMessageIDs_SqliteUserMessages(t *testing.T) {
	ctx := context.Background()
	db, err := engine.NewSqlite(t.TempDir()+"/usermsg.db", "gr1")
	require.NoError(t, err)
	defer db.Close()

	loc, err := NewLocator(ctx, time.Hour, 1000, db)
	require.NoError(t, err)

	const user = int64(8769433764)
	const chat = int64(-1001420186506)

	// same user, distinct messages incl. duplicate text and empty-text image posts
	require.NoError(t, loc.AddMessage(ctx, "привет", chat, user, "alice", 101))
	require.NoError(t, loc.AddMessage(ctx, "как дела", chat, user, "alice", 102))
	require.NoError(t, loc.AddMessage(ctx, "", chat, user, "alice", 103))  // image, empty text
	require.NoError(t, loc.AddMessage(ctx, "+", chat, user, "alice", 104)) // common text
	require.NoError(t, loc.AddMessage(ctx, "спам", chat, user, "alice", 105))

	// another user re-sends identical texts later — clobbers messages rows, must NOT touch user_messages
	require.NoError(t, loc.AddMessage(ctx, "", chat, 999, "bob", 201))
	require.NoError(t, loc.AddMessage(ctx, "+", chat, 999, "bob", 202))
	require.NoError(t, loc.AddMessage(ctx, "спам", chat, 999, "bob", 203))

	ids, err := loc.GetUserMessageIDs(ctx, user, 50)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{101, 102, 103, 104, 105}, ids,
		"all 5 messages of the user must be retrievable regardless of text collisions")

	// reprocessing same (chat,msg) must not duplicate
	require.NoError(t, loc.AddMessage(ctx, "привет", chat, user, "alice", 101))
	ids, err = loc.GetUserMessageIDs(ctx, user, 50)
	require.NoError(t, err)
	require.Len(t, ids, 5)

	// bob keeps his own rows
	bobIDs, err := loc.GetUserMessageIDs(ctx, 999, 50)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{201, 202, 203}, bobIDs)
}
