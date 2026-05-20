package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
)

func TestTelegramActionExecutor_DeleteExtraMessages(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithModerationMetadata(context.Background(), "evt-1", "corr-1", "key-1")
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
	assert.Equal(t, "key-1", journal.calls[0].IdempotencyKey)
}

func TestTelegramActionExecutor_ApplyBan(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithModerationMetadata(context.Background(), "evt-1", "corr-1", "key-1")
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
	assert.Equal(t, 1, journal.calls[0].Attempt)
}

func TestTelegramActionExecutor_SkipCompletedReplay(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{
		last: storage.ModerationActionReplay{Found: true, Completed: true, Attempt: 1},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithModerationMetadata(context.Background(), "evt-1", "corr-1", "key-1")
	err := exec.DeleteMessage(ctx, 123, 77)
	require.NoError(t, err)
	require.Empty(t, mockAPI.RequestCalls())
	require.Empty(t, journal.calls)
}

func TestTelegramActionExecutor_RetryFailedActionWithNextAttempt(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	journal := &moderationActionsSpy{
		last: storage.ModerationActionReplay{Found: true, Completed: false, Attempt: 1, LastError: "telegram timeout"},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithModerationMetadata(context.Background(), "evt-2", "corr-2", "key-1")
	err := exec.ApplyBan(ctx, banRequest{
		userID:   42,
		chatID:   123,
		userName: "user",
		duration: time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, mockAPI.RequestCalls(), 1)
	require.Len(t, journal.calls, 1)
	assert.Equal(t, 2, journal.calls[0].Attempt)
}

func TestTelegramActionExecutor_WarnUser(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}
	journal := &moderationActionsSpy{}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, journal)

	ctx := observability.WithModerationMetadata(context.Background(), "evt-3", "corr-3", "key-3")
	err := exec.WarnUser(ctx, warnRequest{
		chatID:    123,
		subjectID: 42,
		messageID: 77,
		text:      "warning from admin\n\n@user please follow our rules",
	})
	require.NoError(t, err)
	require.Len(t, mockAPI.SendCalls(), 1)
	require.Len(t, journal.calls, 1)
	assert.Equal(t, "warn_user", journal.calls[0].Command)
	assert.Equal(t, "completed", journal.calls[0].Status)
	assert.Equal(t, 42, int(journal.calls[0].SubjectID))
	assert.Equal(t, 77, journal.calls[0].MessageID)
	assert.Equal(t, "key-3", journal.calls[0].IdempotencyKey)
}

func TestTelegramActionExecutor_WarnUserAppealButton(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 1}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.WarnUser(context.Background(), warnRequest{
		chatID:      100,
		subjectID:   200,
		messageID:   5,
		text:        "warned",
		incidentID:  42,
		botUsername: "shield_bot",
	})
	require.NoError(t, err)

	markup, ok := sent.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
	require.True(t, ok, "warn message must carry an inline keyboard")
	require.Len(t, markup.InlineKeyboard, 1)
	require.Len(t, markup.InlineKeyboard[0], 1)
	btn := markup.InlineKeyboard[0][0]
	assert.Equal(t, "Обжаловать", btn.Text)
	require.NotNil(t, btn.URL)
	assert.Equal(t, "https://t.me/shield_bot?start=42", *btn.URL)
}

func TestTelegramActionExecutor_WarnUserNoButtonWithoutIncident(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 1}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.WarnUser(context.Background(), warnRequest{chatID: 100, subjectID: 200, text: "warned"})
	require.NoError(t, err)
	assert.Nil(t, sent.ReplyMarkup, "no incident id -> no appeal button")
}

func TestTelegramActionExecutor_PostBanMessage(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 7}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.PostBanMessage(context.Background(), banMessageRequest{
		chatID:      100,
		text:        "🚫 Пользователь забанен за спам",
		incidentID:  42,
		botUsername: "shield_bot",
	})
	require.NoError(t, err)
	assert.Equal(t, "🚫 Пользователь забанен за спам", sent.Text)
	markup, ok := sent.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
	require.True(t, ok, "ban message must carry the appeal keyboard")
	require.Len(t, markup.InlineKeyboard, 1)
	require.Len(t, markup.InlineKeyboard[0], 1)
	btn := markup.InlineKeyboard[0][0]
	require.NotNil(t, btn.URL)
	assert.Equal(t, "https://t.me/shield_bot?start=42", *btn.URL)
}

type moderationActionsSpy struct {
	calls []storage.ModerationActionEntry
	last  storage.ModerationActionReplay
}

func (s *moderationActionsSpy) Add(_ context.Context, entry storage.ModerationActionEntry) error {
	s.calls = append(s.calls, entry)
	return nil
}

func (s *moderationActionsSpy) Last(_ context.Context, _ storage.ModerationActionLookup) (storage.ModerationActionReplay, error) {
	return s.last, nil
}
