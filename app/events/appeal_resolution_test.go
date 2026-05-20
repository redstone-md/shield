package events

import (
	"context"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/events/mocks"
)

func TestAppealBotAdapter_NotifyAppealResult(t *testing.T) {
	var sent []tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = append(sent, c.(tbapi.MessageConfig))
			return tbapi.Message{}, nil
		},
	}
	adapter := NewAppealBotAdapter(&TelegramListener{TbAPI: mockAPI})

	require.NoError(t, adapter.NotifyAppealResult(context.Background(), 900, true))
	require.NoError(t, adapter.NotifyAppealResult(context.Background(), 900, false))

	require.Len(t, sent, 2)
	assert.Equal(t, int64(900), sent[0].ChatID)
	assert.Contains(t, sent[0].Text, "принята")
	assert.Contains(t, sent[1].Text, "отклонена")
}

func TestAppealBotAdapter_ClearUserWarningsNoCounter(t *testing.T) {
	adapter := NewAppealBotAdapter(&TelegramListener{})
	assert.NoError(t, adapter.ClearUserWarnings(context.Background(), 1), "no detected-spam counter -> no-op")
}

func TestAppealBotAdapter_UnbanUserNoAdminHandler(t *testing.T) {
	adapter := NewAppealBotAdapter(&TelegramListener{})
	err := adapter.UnbanUser(context.Background(), 1)
	require.ErrorContains(t, err, "admin handler not initialized")
}
