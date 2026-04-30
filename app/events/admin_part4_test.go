package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/events/mocks"
	"testing"
)

func TestAdmin_MsgHandlerWithEmptyText(t *testing.T) {
	tests := []struct {
		name string
		msg  *tbapi.Message
	}{
		{
			name: "audio message without text",
			msg: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123},
				From: &tbapi.User{UserName: "admin", ID: 77},
				ForwardOrigin: &tbapi.MessageOrigin{
					Type: "user",
					SenderUser: &tbapi.User{
						ID:       123,
						UserName: "user",
					},
				},
				Audio: &tbapi.Audio{
					FileID: "123",
				},
				MessageID: 999999,
			},
		},
		{
			name: "video message without text",
			msg: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123},
				From: &tbapi.User{UserName: "admin", ID: 77},
				ForwardOrigin: &tbapi.MessageOrigin{
					Type: "user",
					SenderUser: &tbapi.User{
						ID:       123,
						UserName: "user",
					},
				},
				Video: &tbapi.Video{
					FileID: "123",
				},
				MessageID: 999999,
			},
		},
		{
			name: "photo message without text",
			msg: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123},
				From: &tbapi.User{UserName: "admin", ID: 77},
				ForwardOrigin: &tbapi.MessageOrigin{
					Type: "user",
					SenderUser: &tbapi.User{
						ID:       123,
						UserName: "user",
					},
				},
				Photo: []tbapi.PhotoSize{{
					FileID: "123",
				}},
				MessageID: 999999,
			},
		},
	}

	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {

			switch v := c.(type) {
			case tbapi.MessageConfig:
				return tbapi.Message{Text: v.Text}, nil
			case tbapi.EditMessageTextConfig:
				return tbapi.Message{Text: v.Text}, nil
			default:
				return tbapi.Message{}, nil
			}
		},
	}

	botMock := &mocks.BotMock{
		UpdateSpamFunc: func(msg string) error {
			t.Logf("update-spam: %s", msg)
			return nil
		},
		RemoveApprovedUserFunc: func(id int64) error {
			return nil
		},
	}

	locatorMock := &mocks.LocatorMock{
		AddMessageFunc: func(ctx context.Context, msg string, chatID, userID int64, userName string, msgID int) error {
			return nil
		},
	}

	adminHandler := admin{
		tbAPI:      mockAPI,
		bot:        botMock,
		locator:    locatorMock,
		primChatID: 123,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockAPI.ResetCalls()
			botMock.ResetCalls()

			update := tbapi.Update{Message: tt.msg}
			err := adminHandler.MsgHandler(context.Background(), update)
			require.Error(t, err)
			assert.Equal(t, "empty message text", err.Error())

			assert.Empty(t, mockAPI.RequestCalls())
			assert.Empty(t, botMock.UpdateSpamCalls())
		})
	}
}
