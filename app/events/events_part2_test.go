package events

import (
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"testing"
	"time"
)

func TestTelegramListener_transformPhoto(t *testing.T) {
	assert.Equal(t,
		&bot.Message{
			Text: "caption",
			Sent: time.Unix(1578627415, 0),
			Image: &bot.Image{
				FileID:  "AgADAgADFKwxG8r0qUiQByxwp9Gi4s1qwQ8ABAEAAwIAA3kAA5K9AgABFgQ",
				Width:   1280,
				Height:  597,
				Caption: "caption",
				Entities: &[]bot.Entity{
					{
						Type:   "bold",
						Offset: 0,
						Length: 7,
					},
				},
			},
		},
		transform(
			&tbapi.Message{
				Date: 1578627415,
				Photo: []tbapi.PhotoSize{
					{
						FileID:   "AgADAgADFKwxG8r0qUiQByxwp9Gi4s1qwQ8ABAEAAwIAA20AA5C9AgABFgQ",
						Width:    320,
						Height:   149,
						FileSize: 6262,
					},
					{
						FileID:   "AgADAgADFKwxG8r0qUiQByxwp9Gi4s1qwQ8ABAEAAwIAA3gAA5G9AgABFgQ",
						Width:    800,
						Height:   373,
						FileSize: 30240,
					},
					{
						FileID:   "AgADAgADFKwxG8r0qUiQByxwp9Gi4s1qwQ8ABAEAAwIAA3kAA5K9AgABFgQ",
						Width:    1280,
						Height:   597,
						FileSize: 55267,
					},
				},
				Caption: "caption",
				CaptionEntities: []tbapi.MessageEntity{
					{
						Type:   "bold",
						Offset: 0,
						Length: 7,
					},
				},
			},
		),
	)
}

func TestTelegramListener_transformEntities(t *testing.T) {
	tests := []struct {
		name     string
		input    *tbapi.Message
		expected *bot.Message
	}{
		{
			name: "Message with mentions and italics",
			input: &tbapi.Message{
				Date: 1578627415,
				Text: "@username тебя слишком много, отдохни...",
				Entities: []tbapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 0,
						Length: 9,
					},
					{
						Type:   "italic",
						Offset: 10,
						Length: 30,
					},
				},
			},
			expected: &bot.Message{
				Sent: time.Unix(1578627415, 0),
				Text: "@username тебя слишком много, отдохни...",
				Entities: &[]bot.Entity{
					{
						Type:   "mention",
						Offset: 0,
						Length: 9,
					},
					{
						Type:   "italic",
						Offset: 10,
						Length: 30,
					},
				},
			},
		},
		{
			name: "Message with URL entity",
			input: &tbapi.Message{
				Date: 1578627416,
				Text: "Check this link",
				Entities: []tbapi.MessageEntity{
					{
						Type:   "url",
						Offset: 6,
						Length: 4,
						URL:    "https://example.com",
					},
				},
			},
			expected: &bot.Message{
				Sent: time.Unix(1578627416, 0),
				Text: "Check this link",
				Entities: &[]bot.Entity{
					{
						Type:   "url",
						Offset: 6,
						Length: 4,
						URL:    "https://example.com",
					},
				},
			},
		},
		{
			name: "Message with user entity",
			input: &tbapi.Message{
				Date: 1578627417,
				Text: "Message mentioning @user",
				Entities: []tbapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 18,
						Length: 5,
						User: &tbapi.User{
							ID:        100000002,
							UserName:  "user",
							FirstName: "First",
							LastName:  "User",
							IsPremium: true,
						},
					},
				},
			},
			expected: &bot.Message{
				Sent: time.Unix(1578627417, 0),
				Text: "Message mentioning @user",
				Entities: &[]bot.Entity{
					{
						Type:   "mention",
						Offset: 18,
						Length: 5,
						User: &bot.User{
							ID:          100000002,
							Username:    "user",
							DisplayName: "First User",
							FirstName:   "First",
							LastName:    "User",
							IsPremium:   true,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, transform(tt.input))
		})
	}
}

func TestTelegramListener_transformReplyTo(t *testing.T) {
	tbl := []struct {
		name string
		in   *tbapi.Message
		out  bot.Message
	}{
		{
			name: "reply with spaces in display name",
			in: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "reply message",
				From:      &tbapi.User{ID: 456, UserName: "user1"},
				ReplyToMessage: &tbapi.Message{
					Text: "original message",
					From: &tbapi.User{
						ID:        789,
						UserName:  "user2",
						FirstName: "  John  ",
						LastName:  " Doe ",
					},
				},
			},
			out: bot.Message{
				ID:     100,
				ChatID: 123,
				Text:   "reply message",
				From:   bot.User{ID: 456, Username: "user1"},
				ReplyTo: struct {
					From       bot.User
					Text       string `json:",omitempty"`
					Sent       time.Time
					SenderChat bot.SenderChat `json:"sender_chat,omitzero"`
				}{
					Text: "original message",
					From: bot.User{
						ID:          789,
						Username:    "user2",
						DisplayName: "John Doe",
						FirstName:   "John",
						LastName:    "Doe",
					},
				},
			},
		},
		{
			name: "reply with empty last name",
			in: &tbapi.Message{
				MessageID: 101,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "reply message",
				From:      &tbapi.User{ID: 456, UserName: "user1"},
				ReplyToMessage: &tbapi.Message{
					Text: "original message",
					From: &tbapi.User{
						ID:        789,
						UserName:  "user2",
						FirstName: "John",
						LastName:  "",
					},
				},
			},
			out: bot.Message{
				ID:     101,
				ChatID: 123,
				Text:   "reply message",
				From:   bot.User{ID: 456, Username: "user1"},
				ReplyTo: struct {
					From       bot.User
					Text       string `json:",omitempty"`
					Sent       time.Time
					SenderChat bot.SenderChat `json:"sender_chat,omitzero"`
				}{
					Text: "original message",
					From: bot.User{
						ID:          789,
						Username:    "user2",
						DisplayName: "John",
						FirstName:   "John",
					},
				},
			},
		},
	}

	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			res := transform(tt.in)
			assert.Equal(t, tt.out.ReplyTo.From, res.ReplyTo.From)
			assert.Equal(t, tt.out.ReplyTo.Text, res.ReplyTo.Text)
		})
	}
}

func TestTelegramListener_transformQuote(t *testing.T) {
	tbl := []struct {
		name  string
		in    *tbapi.Message
		quote string
	}{
		{
			name: "message with Quote (TextQuote)",
			in: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "user message",
				From:      &tbapi.User{ID: 456, UserName: "user1"},
				Quote: &tbapi.TextQuote{
					Text:     "quoted spam text",
					Position: 18,
				},
			},
			quote: "quoted spam text",
		},
		{
			name: "message without Quote",
			in: &tbapi.Message{
				MessageID: 101,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "user message",
				From:      &tbapi.User{ID: 456, UserName: "user1"},
			},
			quote: "",
		},
		{
			name: "message with empty Quote text",
			in: &tbapi.Message{
				MessageID: 102,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "user message",
				From:      &tbapi.User{ID: 456, UserName: "user1"},
				Quote:     &tbapi.TextQuote{Text: ""},
			},
			quote: "",
		},
	}

	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			res := transform(tt.in)
			assert.Equal(t, tt.quote, res.Quote)
		})
	}
}

func TestTelegramListener_transformForward(t *testing.T) {
	tbl := []struct {
		name string
		in   *tbapi.Message
		out  bot.Message
	}{
		{
			name: "forward from channel with ForwardOrigin",
			in: &tbapi.Message{
				MessageID:     1,
				From:          &tbapi.User{ID: 123, UserName: "user_name"},
				Chat:          tbapi.Chat{ID: 456},
				Text:          "text",
				ForwardOrigin: &tbapi.MessageOrigin{},
			},
			out: bot.Message{
				ID:          1,
				From:        bot.User{ID: 123, Username: "user_name"},
				ChatID:      456,
				Text:        "text",
				WithForward: true,
			},
		},
		{
			name: "real message with video and forward",
			in: &tbapi.Message{
				MessageID: 600627,
				From: &tbapi.User{
					ID:        2010123477,
					UserName:  "Zxcdaun",
					FirstName: "Yaroslav",
				},
				Chat: tbapi.Chat{
					ID:       -1001358715993,
					Type:     "supergroup",
					Title:    "radio-t chat",
					UserName: "radio_t_chat",
				},
				ForwardOrigin: &tbapi.MessageOrigin{
					Type: "channel",
					Chat: &tbapi.Chat{
						ID:       -1002160119872,
						Type:     "channel",
						Title:    "sdhjrt",
						UserName: "srdfjtj",
					},
					MessageID: 2,
				},
				Video: &tbapi.Video{
					FileID:       "BAACAgIAAx0CUPxcWQABCSozZ5_mO2Yf-5dx-_6m_kiz7-kcJ4IAAt5wAAKsCwFJz-NbffiygqI2BA",
					FileUniqueID: "AgAD3nAAAqwLAUk",
					Width:        464,
					Height:       848,
					Duration:     18,
				},
				Caption: "👁‍🗨Гᴧᴀɜ Бᴏᴦᴀ 3.0👁‍🗨\n✅ГОЛЫЕ ЖОПЫПЕР🍑\n✅СЛИВЫ ПРЕРЕПИСОКЛИС📨\n✅ИНТИМА💋 \n❗️И ЕЩЕ МНОГОЕ В ОБНОВЛЕННОМ ИНТИМ ПОИСКЕ❗️\n\n➡️t.me/glaz_Fahjhe_bot⬅️",
			},
			out: bot.Message{
				ID:          600627,
				From:        bot.User{ID: 2010123477, Username: "Zxcdaun", DisplayName: "Yaroslav", FirstName: "Yaroslav"},
				ChatID:      -1001358715993,
				Text:        "👁‍🗨Гᴧᴀɜ Бᴏᴦᴀ 3.0👁‍🗨\n✅ГОЛЫЕ ЖОПЫПЕР🍑\n✅СЛИВЫ ПРЕРЕПИСОКЛИС📨\n✅ИНТИМА💋 \n❗️И ЕЩЕ МНОГОЕ В ОБНОВЛЕННОМ ИНТИМ ПОИСКЕ❗️\n\n➡️t.me/glaz_Fahjhe_bot⬅️",
				WithVideo:   true,
				WithForward: true,
			},
		},
		{
			name: "no forward",
			in: &tbapi.Message{
				MessageID: 1,
				From:      &tbapi.User{ID: 123, UserName: "user_name"},
				Chat:      tbapi.Chat{ID: 456},
				Text:      "text",
			},
			out: bot.Message{
				ID:          1,
				From:        bot.User{ID: 123, Username: "user_name"},
				ChatID:      456,
				Text:        "text",
				WithForward: false,
			},
		},
	}

	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			res := transform(tt.in)
			assert.Equal(t, tt.out.ID, res.ID)
			assert.Equal(t, tt.out.From, res.From)
			assert.Equal(t, tt.out.ChatID, res.ChatID)
			assert.Equal(t, tt.out.Text, res.Text)
			assert.Equal(t, tt.out.WithForward, res.WithForward)
			assert.Equal(t, tt.out.WithVideo, res.WithVideo)
		})
	}
}

func Test_parseCallbackData(t *testing.T) {
	var tests = []struct {
		name       string
		data       string
		wantUserID int64
		wantMsgID  int
		wantErr    bool
	}{
		{"Valid data", "12345:678", 12345, 678, false},
		{"Data too short", "12", 0, 0, true},
		{"No colon separator", "12345678", 0, 0, true},
		{"Invalid userID", "abc:678", 0, 0, true},
		{"Invalid msgID", "12345:xyz", 0, 0, true},
		{"wrong prefix with valid data", "c12345:678", 0, 0, true},
		{"valid prefix+ with valid data", "+12345:678", 12345, 678, false},
		{"valid prefix! with valid data", "!12345:678", 12345, 678, false},
		{"valid prefix? with valid data", "?12345:678", 12345, 678, false},
		{"valid prefix R+ with valid data", "R+12345:678", 12345, 678, false},
		{"valid prefix R- with valid data", "R-12345:678", 12345, 678, false},
		{"valid prefix R? with valid data", "R?12345:678", 12345, 678, false},
		{"valid prefix R! with valid data", "R!12345:678", 12345, 678, false},
		{"valid prefix RX with valid data", "RX12345:678", 12345, 678, false},
		{"negative channel ID", "-100123456:678", -100123456, 678, false},
		{"negative channel ID with prefix", "?-100123456:678", -100123456, 678, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, gotMsgID, err := parseCallbackData(tt.data)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantUserID, gotUserID)
				assert.Equal(t, tt.wantMsgID, gotMsgID)
			}
		})
	}
}
