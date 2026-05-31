package events

import (
	"context"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlowpathReasonIncludesVisionProviderChecks(t *testing.T) {
	checks := []spamcheck.Response{
		{Name: "message length", Spam: false, Details: "too short"},
		{Name: "openai", Spam: true, Details: "на изображении реклама крипто-схемы, confidence: 94%"},
	}

	assert.Equal(t, "на изображении реклама крипто-схемы, confidence: 94%", slowpathReason(checks))
}

func TestAdminCallbackShowInfoFallsBackToExistingDiagnostics(t *testing.T) {
	var edited tbapi.EditMessageTextConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			var ok bool
			edited, ok = c.(tbapi.EditMessageTextConfig)
			require.True(t, ok)
			return tbapi.Message{}, nil
		},
	}
	adm := admin{
		tbAPI: mockAPI,
		locator: &mocks.LocatorMock{
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, false
			},
		},
	}

	query := &tbapi.CallbackQuery{
		Data: "!777:42:123",
		Message: &tbapi.Message{
			MessageID: 42,
			Chat:      tbapi.Chat{ID: 123},
			Text: "забанен навсегда user 777\n\n[image]\n\n**spam detection results**\n" +
				"- openai: spam, на изображении реклама крипто-схемы, confidence: 94%",
		},
		From: &tbapi.User{ID: 1, UserName: "admin"},
	}

	err := adm.callbackShowInfo(context.Background(), query)
	require.NoError(t, err)

	assert.NotContains(t, edited.Text, "can't get spam info")
	assert.Contains(t, edited.Text, "openai: spam")
	assert.Contains(t, edited.Text, "реклама крипто-схемы")
}
