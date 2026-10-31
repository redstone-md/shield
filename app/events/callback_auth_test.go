package events

import (
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/events/mocks"
)

func TestAuthorizeCallback(t *testing.T) {
	const adminChat = int64(456)
	adminMsg := &tbapi.Message{Chat: tbapi.Chat{ID: adminChat}}
	supers := SuperUsers{"123", "boss", ""}

	tests := []struct {
		name    string
		query   *tbapi.CallbackQuery
		allowed bool
		toast   bool
	}{
		{name: "superuser by id", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 123}, Message: adminMsg}, allowed: true},
		{name: "superuser by name", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 1, UserName: "BOSS"}, Message: adminMsg}, allowed: true},
		{name: "admin chat member without rights", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 999, UserName: "guest"}, Message: adminMsg}, toast: true},
		{name: "no username does not match empty entry", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 999}, Message: adminMsg}, toast: true},
		{name: "no sender", query: &tbapi.CallbackQuery{Message: adminMsg}, toast: true},
		{name: "other chat", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 123}, Message: &tbapi.Message{Chat: tbapi.Chat{ID: 789}}}},
		{name: "inline message without chat", query: &tbapi.CallbackQuery{From: &tbapi.User{ID: 123}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mocks.TbAPIMock{
				RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
			}

			assert.Equal(t, tt.allowed, authorizeCallback(mockAPI, tt.query, adminChat, supers))

			assert.Empty(t, mockAPI.SendCalls(), "refusal must not post to the admin chat")
			if !tt.toast {
				assert.Empty(t, mockAPI.RequestCalls())
				return
			}
			require.Len(t, mockAPI.RequestCalls(), 1)
			answer, ok := mockAPI.RequestCalls()[0].C.(tbapi.CallbackConfig)
			require.True(t, ok)
			assert.Equal(t, "нет прав", answer.Text)
		})
	}
}

func TestInlineCallbackHandlerRefusesNonSuperuser(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
	}
	a := admin{tbAPI: mockAPI, adminChatID: 456, superUsers: SuperUsers{"123"}}

	err := a.InlineCallbackHandler(t.Context(), &tbapi.CallbackQuery{
		Data:    "777:999",
		From:    &tbapi.User{ID: 999, UserName: "guest"},
		Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}},
	})

	require.NoError(t, err)
	assert.Empty(t, mockAPI.SendCalls())
	require.Len(t, mockAPI.RequestCalls(), 1, "only the refusal toast is sent")
}

func TestHandleReportCallbackRefusesNonSuperuser(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
	}
	r := userReports{tbAPI: mockAPI, adminChatID: 456, superUsers: SuperUsers{"123"}}

	err := r.HandleReportCallback(t.Context(), &tbapi.CallbackQuery{
		Data:    "R+123:456",
		From:    &tbapi.User{ID: 999, UserName: "guest"},
		Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}},
	})

	require.NoError(t, err)
	assert.Empty(t, mockAPI.SendCalls())
	require.Len(t, mockAPI.RequestCalls(), 1, "only the refusal toast is sent")
}
