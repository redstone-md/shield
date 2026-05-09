package events

import (
	"errors"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSpamLoggerFunc_Save(t *testing.T) {

	msg := &bot.Message{
		ID:     123,
		ChatID: 456,
		Text:   "test message",
		From: bot.User{
			ID:          789,
			Username:    "testuser",
			DisplayName: "Test User",
		},
	}

	response := &bot.Response{
		Text:        "test response",
		Send:        true,
		BanInterval: time.Minute,
		User: bot.User{
			ID:          789,
			Username:    "testuser",
			DisplayName: "Test User",
		},
	}

	counter := 0

	loggerFunc := SpamLoggerFunc(func(m *bot.Message, r *bot.Response) {
		counter++
		assert.Equal(t, msg, m)
		assert.Equal(t, response, r)
	})

	loggerFunc.Save(msg, response)

	assert.Equal(t, 1, counter)
}

func TestEvents_escapeMarkDownV1Text(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Test with all markdown symbols",
			input:    "_*`[",
			expected: "\\_\\*\\`\\[",
		},
		{
			name:     "Test with no markdown symbols",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "Test with mixed content",
			input:    "Hello_World*`[",
			expected: "Hello\\_World\\*\\`\\[",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMarkDownV1Text(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEvents_markdownUserLink(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		userID   int64
		expected string
	}{
		{
			name:     "with id",
			userName: "admin",
			userID:   111,
			expected: "[admin](tg://user?id=111)",
		},
		{
			name:     "escapes underscore",
			userName: "verka_87",
			userID:   8651562434,
			expected: "[verka\\_87](tg://user?id=8651562434)",
		},
		{
			name:     "username fallback",
			userName: "@nevermorelove",
			expected: "[nevermorelove](https://t.me/nevermorelove)",
		},
		{
			name:     "id fallback",
			userID:   222,
			expected: "[222](tg://user?id=222)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, markdownUserLink(tt.userName, tt.userID))
		})
	}
}

func TestEvents_truncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		suffix   string
		expected string
	}{
		{
			name:     "short string not truncated",
			input:    "hello",
			maxRunes: 10,
			suffix:   "...",
			expected: "hello",
		},
		{
			name:     "exact length not truncated",
			input:    "hello",
			maxRunes: 5,
			suffix:   "...",
			expected: "hello",
		},
		{
			name:     "long ASCII string truncated",
			input:    "hello world this is a test",
			maxRunes: 10,
			suffix:   "...",
			expected: "hello worl...",
		},
		{
			name:     "emoji truncation",
			input:    "😀😁😂😃😄😅😆😇😈😉",
			maxRunes: 5,
			suffix:   "...",
			expected: "😀😁😂😃😄...",
		},
		{
			name:     "cyrillic truncation",
			input:    "Привет мир это тест",
			maxRunes: 10,
			suffix:   "...",
			expected: "Привет мир...",
		},
		{
			name:     "mixed multibyte truncation",
			input:    "Hello мир 😀 test",
			maxRunes: 10,
			suffix:   "...",
			expected: "Hello мир ...",
		},
		{
			name:     "arabic truncation",
			input:    "مرحبا بك في العالم",
			maxRunes: 8,
			suffix:   "...",
			expected: "مرحبا بك...",
		},
		{
			name:     "empty suffix",
			input:    "hello world",
			maxRunes: 5,
			suffix:   "",
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxRunes, tt.suffix)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}

			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8: %q", result)
			}
		})
	}
}

func TestEvents_send(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				if mc.Text == "badmd" && mc.ParseMode == "Markdown" {
					return tbapi.Message{}, errors.New("bad markdown")
				}
			}
			return tbapi.Message{}, nil
		},
	}

	t.Run("send with markdown passed", func(t *testing.T) {
		mockAPI.ResetCalls()
		err := send(tbapi.NewMessage(123, "test"), mockAPI)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1)
		assert.Equal(t, int64(123), mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ChatID)
		assert.Equal(t, "test", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
		assert.Equal(t, "Markdown", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ParseMode)
	})

	t.Run("send markdown link without fallback", func(t *testing.T) {
		mockAPI.ResetCalls()
		err := send(tbapi.NewMessage(123, "пользователь [123](tg://user?id=123) забанен"), mockAPI)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Equal(t, "Markdown", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ParseMode)
	})

	t.Run("send with markdown failed", func(t *testing.T) {
		mockAPI.ResetCalls()
		err := send(tbapi.NewMessage(123, "badmd"), mockAPI)
		require.NoError(t, err)

		assert.Len(t, mockAPI.SendCalls(), 2)

		assert.Equal(t, int64(123), mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ChatID)
		assert.Equal(t, "badmd", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
		assert.Equal(t, "Markdown", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ParseMode)

		assert.Equal(t, int64(123), mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).ChatID)
		assert.Equal(t, "badmd", mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text)
		assert.Empty(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).ParseMode)
	})

}

func TestTelegramListener_transformTextMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    *tbapi.Message
		expected *bot.Message
	}{
		{
			name: "Basic text message",
			input: &tbapi.Message{
				Chat: tbapi.Chat{
					ID: 123456,
				},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID: 30,
				Date:      1578627415,
				Text:      "Message",
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:   time.Unix(1578627415, 0),
				Text:   "Message",
				ChatID: 123456,
			},
		},
		{
			name: "Text message with nil values",
			input: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 123456},
				MessageID: 31,
				Date:      1579627415,
				Text:      "",
			},
			expected: &bot.Message{
				ID:     31,
				Sent:   time.Unix(1579627415, 0),
				Text:   "",
				ChatID: 123456,
			},
		},
		{
			name: "Text message with sender chat",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				SenderChat: &tbapi.Chat{
					ID:       654321,
					UserName: "channelname",
				},
				MessageID: 32,
				Date:      1579627416,
				Text:      "Channel Message",
			},
			expected: &bot.Message{
				ID:     32,
				Sent:   time.Unix(1579627416, 0),
				Text:   "Channel Message",
				ChatID: 123456,
				SenderChat: bot.SenderChat{
					ID:       654321,
					UserName: "channelname",
				},
			},
		},
		{
			name: "Message with forward",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID:     30,
				Date:          1578627415,
				Text:          "Forwarded message",
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Unix(1578627415, 0).Unix()},
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:        time.Unix(1578627415, 0),
				Text:        "Forwarded message",
				ChatID:      123456,
				WithForward: true,
			},
		},
		{
			name: "Message with reply",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID: 30,
				Date:      1578627415,
				Text:      "Reply to message",
				ReplyToMessage: &tbapi.Message{
					MessageID: 29,
					Date:      1578627400,
					Text:      "Original message",
					From: &tbapi.User{
						ID:        100000002,
						UserName:  "original_user",
						FirstName: "Original",
						LastName:  "User",
					},
				},
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:   time.Unix(1578627415, 0),
				Text:   "Reply to message",
				ChatID: 123456,
				ReplyTo: struct {
					From       bot.User
					Text       string `json:",omitempty"`
					Sent       time.Time
					SenderChat bot.SenderChat `json:"sender_chat,omitzero"`
				}{
					Sent: time.Unix(1578627400, 0),
					Text: "Original message",
					From: bot.User{
						ID:          100000002,
						Username:    "original_user",
						DisplayName: "Original User",
						FirstName:   "Original",
						LastName:    "User",
					},
				},
			},
		},
		{
			name: "Message with story",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID: 30,
				Date:      1578627415,
				Text:      "Message with story",
				Story:     &tbapi.Story{},
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:      time.Unix(1578627415, 0),
				Text:      "Message with story",
				ChatID:    123456,
				WithVideo: true,
			},
		},
		{
			name: "Message with audio",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID: 30,
				Date:      1578627415,
				Audio:     &tbapi.Audio{},
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:      time.Unix(1578627415, 0),
				ChatID:    123456,
				WithAudio: true,
			},
		},
		{
			name: "Message with giveaway",
			input: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123456},
				From: &tbapi.User{
					ID:        100000001,
					UserName:  "username",
					FirstName: "First",
					LastName:  "Last",
				},
				MessageID: 30,
				Date:      1578627415,
				Giveaway:  &tbapi.Giveaway{},
			},
			expected: &bot.Message{
				ID: 30,
				From: bot.User{
					ID:          100000001,
					Username:    "username",
					DisplayName: "First Last",
					FirstName:   "First",
					LastName:    "Last",
				},
				Sent:         time.Unix(1578627415, 0),
				ChatID:       123456,
				WithGiveaway: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, transform(tt.input))
		})
	}
}
