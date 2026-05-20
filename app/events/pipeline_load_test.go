package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/moderation"
	"github.com/redstone-md/shield/app/storage"
)

func TestPipeline_LoadBurst(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	var processed atomic.Int64
	spy := &processorSpy{}
	spy.process = func(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
		processed.Add(1)
		return nil
	}

	eventStore := &incomingEventsSpy{}
	eventStore.reserve = func(ctx context.Context, event moderation.IncomingEvent) (storage.IncomingEventReplay, error) {
		return storage.IncomingEventReplay{Recorded: true}, nil
	}

	l := TelegramListener{
		Bot:            &mocks.BotMock{},
		Locator:        locator,
		IncomingEvents: eventStore,
		Group:          "123",
		TenantID:       "tg-spam",
		chatID:         123,
		Queue:          moderation.NewInMemoryQueue(1000),
		processor:      spy,
	}
	defer l.shutdownPipeline()

	const numMessages = 500
	for i := range numMessages {
		update := tbapi.Update{
			UpdateID: 9000 + i,
			Message: &tbapi.Message{
				MessageID: 100 + i,
				Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
				Text:      "burst load message",
				From:      &tbapi.User{ID: int64(i % 50), UserName: "user"},
				Date:      int(time.Now().Unix()),
			},
		}
		require.NoError(t, l.procEvents(update))
	}

	require.Eventually(t, func() bool {
		return processed.Load() == int64(numMessages)
	}, 10*time.Second, 50*time.Millisecond, "all %d messages should be processed", numMessages)

	assert.Equal(t, int64(numMessages), processed.Load())
}
