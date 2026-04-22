package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestTelegramActionExecutor_DeleteExtraMessages(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithEventMetadata(context.Background(), "evt-1", "corr-1")
	err := exec.DeleteExtraMessages(ctx, []spamcheck.Response{{
		Name:           "duplicates",
		Spam:           true,
		ExtraDeleteIDs: []int{11, 12},
	}}, 42, "user", 123)
	require.NoError(t, err)
	require.Len(t, mockAPI.RequestCalls(), 2)
	require.Len(t, journal.calls, 2)
	assert.Equal(t, 11, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, 12, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, "delete_message", journal.calls[0].Command)
	assert.Equal(t, "completed", journal.calls[0].Status)
}

func TestTelegramActionExecutor_ApplyBan(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithEventMetadata(context.Background(), "evt-1", "corr-1")
	err := exec.ApplyBan(ctx, banRequest{
		userID:   42,
		chatID:   123,
		userName: "user",
		duration: time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, mockAPI.RequestCalls(), 1)
	require.Len(t, journal.calls, 1)
	_, ok := mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig)
	assert.True(t, ok)
	assert.Equal(t, "ban_user", journal.calls[0].Command)
	assert.Equal(t, "completed", journal.calls[0].Status)
}

type moderationActionsSpy struct {
	calls []storage.ModerationActionEntry
}

func (s *moderationActionsSpy) Add(_ context.Context, entry storage.ModerationActionEntry) error {
	s.calls = append(s.calls, entry)
	return nil
}
