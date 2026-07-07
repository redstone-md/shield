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
	const chat2 = int64(-1009999999999)

	// same user, distinct messages incl. duplicate text and empty-text image posts
	require.NoError(t, loc.AddMessage(ctx, "привет", chat, user, "alice", 101))
	require.NoError(t, loc.AddMessage(ctx, "как дела", chat, user, "alice", 102))
	require.NoError(t, loc.AddMessage(ctx, "", chat, user, "alice", 103))  // image, empty text
	require.NoError(t, loc.AddMessage(ctx, "+", chat, user, "alice", 104)) // common text
	require.NoError(t, loc.AddMessage(ctx, "спам", chat, user, "alice", 105))
	// same user posts in a second group — must be tracked with its own chat_id
	require.NoError(t, loc.AddMessage(ctx, "другая группа", chat2, user, "alice", 501))

	// another user re-sends identical texts later — clobbers messages rows, must NOT touch user_messages
	require.NoError(t, loc.AddMessage(ctx, "", chat, 999, "bob", 201))
	require.NoError(t, loc.AddMessage(ctx, "+", chat, 999, "bob", 202))
	require.NoError(t, loc.AddMessage(ctx, "спам", chat, 999, "bob", 203))

	msgs, err := loc.GetUserMessages(ctx, user, 50)
	require.NoError(t, err)
	gotIDs := make([]int, len(msgs))
	chatByMsg := map[int]int64{}
	for i, m := range msgs {
		gotIDs[i] = m.MsgID
		chatByMsg[m.MsgID] = m.ChatID
	}
	require.ElementsMatch(t, []int{101, 102, 103, 104, 105, 501}, gotIDs,
		"all messages of the user across chats must be retrievable regardless of text collisions")
	require.Equal(t, chat, chatByMsg[105], "chat_id must be preserved per message")
	require.Equal(t, chat2, chatByMsg[501], "message from second group must carry its own chat_id")

	// reprocessing same (chat,msg) must not duplicate
	require.NoError(t, loc.AddMessage(ctx, "привет", chat, user, "alice", 101))
	msgs, err = loc.GetUserMessages(ctx, user, 50)
	require.NoError(t, err)
	require.Len(t, msgs, 6)

	// bob keeps his own rows
	bobMsgs, err := loc.GetUserMessages(ctx, 999, 50)
	require.NoError(t, err)
	bobIDs := make([]int, len(bobMsgs))
	for i, m := range bobMsgs {
		bobIDs[i] = m.MsgID
	}
	require.ElementsMatch(t, []int{201, 202, 203}, bobIDs)
}
