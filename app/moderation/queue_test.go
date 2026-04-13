package moderation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryQueuePublishAndConsume(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	defer q.Close()

	event := IncomingEvent{
		EventID:       "evt-1",
		CorrelationID: "corr-1",
		Source:        "telegram.webhook",
		ReceivedAt:    time.Now().UTC(),
	}

	err := q.Publish(context.Background(), event)
	require.NoError(t, err)

	select {
	case got := <-q.Consume():
		assert.Equal(t, event, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestInMemoryQueuePublishCanceledContext(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(0)
	defer q.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Publish(ctx, IncomingEvent{EventID: "evt-2"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestInMemoryQueuePublishAfterClose(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	q.Close()

	err := q.Publish(context.Background(), IncomingEvent{EventID: "evt-3"})
	require.ErrorIs(t, err, ErrQueueClosed)
}

func TestInMemoryQueueCloseClosesConsumerChannel(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	ch := q.Consume()

	q.Close()

	select {
	case _, ok := <-ch:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queue close")
	}
}
