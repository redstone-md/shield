package events

import (
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
)

// TestTelegramListener_UpdateConfig_SubscribesChatMember guards the join-time DC
// ban gate: chat_member is excluded from Telegram's default allowed_updates, so it
// must be requested explicitly or member-transition updates never arrive. The
// list is pinned to exactly the types the listener dispatches plus chat_member,
// so accidentally re-subscribing unhandled types is caught.
func TestTelegramListener_UpdateConfig_SubscribesChatMember(t *testing.T) {
	u := (&TelegramListener{}).updateConfig()

	assert.Equal(t, 60, u.Timeout, "long-poll timeout must stay 60s")
	want := []string{
		tbapi.UpdateTypeMessage,
		tbapi.UpdateTypeEditedMessage,
		tbapi.UpdateTypeCallbackQuery,
		tbapi.UpdateTypeChatMember,
	}
	assert.ElementsMatch(t, want, u.AllowedUpdates,
		"AllowedUpdates must be exactly the dispatched types plus chat_member")
}
