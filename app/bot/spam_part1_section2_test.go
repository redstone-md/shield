package bot

import (
	"github.com/umputun/tg-spam/lib/spamcheck"
	"testing"
	"time"
)

func TestSpamFilter_OnMessage_WithMultipleLinkTypes(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with multiple link types",
		message: Message{
			Text: "visit https://site.com or click here",
			From: User{ID: 1, Username: "user1"},
			Entities: &[]Entity{
				{Type: "url", Offset: 6, Length: 16},
				{Type: "text_link", Offset: 26, Length: 10, URL: "https://other.com"},
			},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "visit https://site.com or click here",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Links: 2},
		},
	})
}

func TestSpamFilter_OnMessage_SpamInQuotedReplyToTextFromExternalChannel(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam in quoted/reply-to text from external channel",
		message: Message{
			Text: "Есть в наличии",
			From: User{ID: 1, Username: "user1"},
			ReplyTo: struct {
				From       User
				Text       string `json:",omitempty"`
				Sent       time.Time
				SenderChat SenderChat `json:"sender_chat,omitzero"`
			}{
				Text: "Мефедрон VHQ Кристалл 1г",
				From: User{ID: 999, Username: "spammer_channel"},
			},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "Есть в наличии\nМефедрон VHQ Кристалл 1г",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_MessageWithEmptyReplyToText(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "message with empty reply-to text",
		message: Message{
			Text: "spam message",
			From: User{ID: 1, Username: "user1"},
			ReplyTo: struct {
				From       User
				Text       string `json:",omitempty"`
				Sent       time.Time
				SenderChat SenderChat `json:"sender_chat,omitzero"`
			}{
				Text: "",
				From: User{ID: 999, Username: "other_user"},
			},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "spam message",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_EmptyMainTextWithSpamInReplyTo(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "empty main text with spam in reply-to",
		message: Message{
			Text: "",
			From: User{ID: 1, Username: "user1"},
			ReplyTo: struct {
				From       User
				Text       string `json:",omitempty"`
				Sent       time.Time
				SenderChat SenderChat `json:"sender_chat,omitzero"`
			}{
				Text: "spam in quoted message",
				From: User{ID: 999, Username: "spammer_channel"},
			},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "\nspam in quoted message",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_SpamInQuoteFieldTelegramTextquote(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam in Quote field (telegram TextQuote)",
		message: Message{
			Text:  "Мяу в наличии!",
			From:  User{ID: 1, Username: "user1"},
			Quote: "Мефедрон VHQ Кристалл 1г",
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "Мяу в наличии!\nМефедрон VHQ Кристалл 1г",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_SpamWithUserFirstLastNameAndPremium(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam with user first/last name and premium",
		message: Message{
			Text: "spam message",
			From: User{ID: 1, Username: "user1", DisplayName: "John Doe",
				FirstName: "John", LastName: "Doe", IsPremium: true},
		},
		wantResponse: Response{
			Text:          `detected: "John Doe" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1", DisplayName: "John Doe"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg: "spam message", UserID: "1", UserName: "user1",
			FirstName: "John", LastName: "Doe", IsPremium: true,
		},
	})
}

func TestSpamFilter_OnMessage_SpamDetectedFromChannelMessage(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam detected from channel message",
		message: Message{
			Text:       "spam message",
			From:       User{ID: 136817688, Username: "Channel_Bot", FirstName: "Channel_Bot"},
			SenderChat: SenderChat{ID: 12345, UserName: "spam_channel"},
		},
		wantResponse: Response{
			Text:          `detected: "Channel_Bot" (136817688)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 136817688, Username: "Channel_Bot"},
			ChannelID:     12345,
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{Msg: "spam message", UserID: "12345", UserName: "spam_channel"},
	})
}

func TestSpamFilter_OnMessage_BothQuoteAndReplytoTextPresentQuoteTakesPrecedence(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "both Quote and ReplyTo.Text present - Quote takes precedence",
		message: Message{
			Text:  "check this",
			From:  User{ID: 1, Username: "user1"},
			Quote: "spam quote text",
			ReplyTo: struct {
				From       User
				Text       string `json:",omitempty"`
				Sent       time.Time
				SenderChat SenderChat `json:"sender_chat,omitzero"`
			}{
				Text: "full reply text",
			},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "check this\nspam quote text",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_WithSticker(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with sticker",
		message: Message{
			Text:        "spam message",
			From:        User{ID: 1, Username: "user1"},
			WithSticker: true,
			Sticker:     &StickerInfo{FileID: "stk123", SetName: "Cats"},
		},
		wantResponse: Response{
			Text:          `detected: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "spam message",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{HasSticker: true},
		},
	})
}
