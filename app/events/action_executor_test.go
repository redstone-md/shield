package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestTelegramActionExecutor_DeleteExtraMessages(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil)

	err := exec.DeleteExtraMessages(context.Background(), []spamcheck.Response{{
		Name:           "duplicates",
		Spam:           true,
		ExtraDeleteIDs: []int{11, 12},
	}}, 42, "user", 123)
	require.NoError(t, err)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 11, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, 12, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
}

func TestTelegramActionExecutor_ApplyBan(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil)

	err := exec.ApplyBan(context.Background(), banRequest{
		userID:   42,
		chatID:   123,
		userName: "user",
		duration: time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, mockAPI.RequestCalls(), 1)
	_, ok := mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig)
	assert.True(t, ok)
}
