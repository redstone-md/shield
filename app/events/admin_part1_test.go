package events

import (
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"testing"
)

func TestAdmin_reportBan(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}

	adm := admin{
		tbAPI:       mockAPI,
		adminChatID: 123,
	}

	msg := &bot.Message{
		From: bot.User{
			ID: 456,
		},
		Text: "Test\n\n_message_",
	}

	t.Run("normal user name", func(t *testing.T) {
		mockAPI.ResetCalls()
		adm.ReportBan("testUser", msg, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		t.Logf("sent text: %+v", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
		assert.Equal(t, int64(123), mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ChatID)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "permanently banned [testUser](tg://user?id=456)")
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "Test  \\_message\\_")
		assert.NotNil(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup)
		assert.Equal(t, "⛔︎ change ban",
			mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup.(tbapi.InlineKeyboardMarkup).InlineKeyboard[0][0].Text)
	})

	t.Run("name with md chars", func(t *testing.T) {
		mockAPI.ResetCalls()
		adm.ReportBan("test_User", msg, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		t.Logf("sent text: %+v", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
		assert.Equal(t, int64(123), mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ChatID)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "permanently banned [test\\_User](tg://user?id=456)")
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "Test  \\_message\\_")
		assert.NotNil(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup)
		assert.Equal(t, "⛔︎ change ban",
			mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup.(tbapi.InlineKeyboardMarkup).InlineKeyboard[0][0].Text)
	})

	t.Run("message with quote", func(t *testing.T) {
		mockAPI.ResetCalls()
		msgWithQuote := &bot.Message{
			From:  bot.User{ID: 456},
			Text:  "Спасибо!!",
			Quote: "Бесплатный VPN для Telegram",
		}
		adm.ReportBan("spammer", msgWithQuote, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		t.Logf("sent text: %+v", sentText)
		assert.Contains(t, sentText, "Спасибо!!")
		assert.Contains(t, sentText, "Бесплатный VPN для Telegram")
	})

	t.Run("no username uses first name and id without profile link", func(t *testing.T) {
		mockAPI.ResetCalls()
		msgWithoutUsername := &bot.Message{
			From: bot.User{ID: 7187750383, FirstName: "Claude"},
			Text: "spam text",
		}
		adm.ReportBan("", msgWithoutUsername, bot.PermanentBanDuration, true)

		require.Len(t, mockAPI.SendCalls(), 1)
		sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		assert.Contains(t, sentText, "<b>restricted</b> Claude (7187750383)")
		assert.NotContains(t, sentText, "tg://user")
	})
}

func TestAdmin_reportWarnNoUsernameUsesFirstNameAndID(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}

	adm := admin{
		tbAPI:       mockAPI,
		adminChatID: 123,
	}
	msg := &bot.Message{
		From: bot.User{ID: 7187750383, FirstName: "Claude"},
		Text: "spam text",
	}

	adm.ReportWarn("", msg, 1, 3)

	require.Len(t, mockAPI.SendCalls(), 1)
	sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
	assert.Contains(t, sentText, "<b>⚠️ WARNING 1/3</b> Claude (7187750383)")
	assert.NotContains(t, sentText, "tg://user")
}

func TestAdmin_reportWarnUsernameUsesFirstNameAsLinkText(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}

	adm := admin{
		tbAPI:       mockAPI,
		adminChatID: 123,
	}
	msg := &bot.Message{
		From: bot.User{ID: 7187750383, Username: "baz_02l_wss", FirstName: "Firstname"},
		Text: "spam text",
	}

	adm.ReportWarn("baz_02l_wss", msg, 3, 3)

	require.Len(t, mockAPI.SendCalls(), 1)
	sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
	assert.Contains(t, sentText, `<b>⚠️ WARNING 3/3</b> <a href="https://t.me/baz_02l_wss">Firstname</a>`)
	assert.NotContains(t, sentText, `>baz_02l_wss</a>`)
}

func TestAdmin_getCleanMessage(t *testing.T) {
	a := &admin{}

	tests := []struct {
		name     string
		input    string
		expected string
		err      bool
	}{
		{
			name:     "with spam detection results",
			input:    "Line 1\n\nLine 2\nLine3\n\nspam detection results:\nLine 4",
			expected: "Line 2\nLine3",
			err:      false,
		},
		{
			name:     "without spam detection results",
			input:    "Line 1\n\nLine 2\nLine 3",
			expected: "Line 2\nLine 3",
			err:      false,
		},
		{
			name:     "without spam detection results, single line",
			input:    "Line 1\n\nLine 2",
			expected: "Line 2",
			err:      false,
		},
		{
			name:     "with spam detection results, single line",
			input:    "Line 1\n\nLine 2\n\nspam detection results:\nLine 4",
			expected: "Line 2",
			err:      false,
		},
		{
			name:     "only one line",
			input:    "Line 1",
			expected: "",
			err:      true,
		},
		{
			name:     "spam detection results immediately after header",
			input:    "Line 1\n\nspam detection results:\nLine 3",
			expected: "",
			err:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.getCleanMessage(tt.input)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAdmin_getCleanMessage2(t *testing.T) {
	msg := `permanently banned {157419590 new_Nikita Νικήτας}

и да, этим надо заниматься каждый день по несколько часов. За месяц увидишь ощутимый результат

**spam detection results**
- stopword: ham, not found
- emoji: ham, 0/2
- similarity: ham, 0.15/0.50
- classifier: spam, probability of spam: 71.70%
- cas: ham, record not found

_unbanned by umputun in 1m5s_`

	a := &admin{}
	result, err := a.getCleanMessage(msg)
	require.NoError(t, err)
	assert.Equal(t, "и да, этим надо заниматься каждый день по несколько часов. За месяц увидишь ощутимый результат", result)
}

func TestAdmin_extractUsername(t *testing.T) {
	tests := []struct {
		name           string
		banMessage     string
		expectedResult string
		expectError    bool
	}{
		{name: "markdown format", banMessage: "**permanently banned [John_Doe](tg://user?id=123456)** some text", expectedResult: "John_Doe"},
		{name: "plain format", banMessage: "permanently banned {200312168 umputun Umputun U} some text", expectedResult: "umputun"},
		{name: "t.me channel link", banMessage: "**permanently banned [spamchannel](https://t.me/spamchannel)**\n\nspam text", expectedResult: "spamchannel"},
		{name: "plain channel with ID", banMessage: "**permanently banned mychannel (-100999888)**\n\nspam text", expectedResult: "mychannel"},
		{name: "plain channel multi-word title", banMessage: "**permanently banned Spam News Channel (-100999888)**\n\ntext", expectedResult: "Spam News Channel"},
		{name: "invalid format", banMessage: "permanently banned John_Doe some message text", expectError: true},
	}

	a := admin{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username, err := a.extractUsername(test.banMessage)
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedResult, username)
			}
		})
	}
}

func TestAdmin_dryModeForwardMessage(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}
	adm := admin{
		tbAPI: mockAPI,
		dry:   true,
	}
	msg := &bot.Message{}

	adm.ReportBan("testUser", msg, bot.PermanentBanDuration, false)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "would have permanently banned [testUser]")
}

func TestAdmin_reportBanChannel(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
	}

	adm := admin{tbAPI: mockAPI, adminChatID: 123}

	t.Run("channel message uses SenderChat.ID in callback data", func(t *testing.T) {
		mockAPI.ResetCalls()
		msg := &bot.Message{
			From:       bot.User{ID: 136817688, Username: "Channel_Bot"},
			SenderChat: bot.SenderChat{ID: -100999888, UserName: "spamchannel"},
			Text:       "spam from channel",
		}
		adm.ReportBan("spamchannel", msg, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		markup := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup.(tbapi.InlineKeyboardMarkup)

		require.NotNil(t, markup.InlineKeyboard[0][0].CallbackData)
		assert.Contains(t, *markup.InlineKeyboard[0][0].CallbackData, "-100999888:")
		assert.NotContains(t, *markup.InlineKeyboard[0][0].CallbackData, "136817688")

		assert.Contains(t, sentText, "https://t.me/spamchannel")
		assert.NotContains(t, sentText, "tg://user?id=136817688")
	})

	t.Run("channel message without username uses plain text with ID", func(t *testing.T) {
		mockAPI.ResetCalls()
		msg := &bot.Message{
			From:       bot.User{ID: 136817688, Username: "Channel_Bot"},
			SenderChat: bot.SenderChat{ID: -100999888},
			Text:       "spam from channel",
		}
		adm.ReportBan("Some Channel", msg, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		assert.Contains(t, sentText, "permanently banned Some Channel (-100999888)")
		assert.NotContains(t, sentText, "tg://user")
	})

	t.Run("regular user message uses From.ID in callback data", func(t *testing.T) {
		mockAPI.ResetCalls()
		msg := &bot.Message{
			From: bot.User{ID: 456, Username: "spammer"},
			Text: "spam from user",
		}
		adm.ReportBan("spammer", msg, bot.PermanentBanDuration, false)

		require.Len(t, mockAPI.SendCalls(), 1)
		sentText := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		markup := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).ReplyMarkup.(tbapi.InlineKeyboardMarkup)
		require.NotNil(t, markup.InlineKeyboard[0][0].CallbackData)
		assert.Contains(t, *markup.InlineKeyboard[0][0].CallbackData, "456:")

		assert.Contains(t, sentText, "tg://user?id=456")
	})
}
