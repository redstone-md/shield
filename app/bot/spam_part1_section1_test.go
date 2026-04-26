package bot

import (
	"github.com/stretchr/testify/assert"
	"github.com/umputun/tg-spam/app/bot/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"testing"
)

type spamFilterOnMessageCase struct {
	name         string
	message      Message
	checkOnly    bool
	dry          bool
	wantResponse Response
	wantRequest  spamcheck.Request
}

func runSpamFilterOnMessageCase(t *testing.T, tc spamFilterOnMessageCase) {
	det := &mocks.MessageCheckerMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			if tc.wantRequest != (spamcheck.Request{}) {
				assert.Equal(t, tc.wantRequest, req)
			}
			switch tc.message.Text {
			case "good message":
				return false, []spamcheck.Response{{Name: "test", Spam: false, Details: "ham"}}
			case "openai verdict":
				return true, []spamcheck.Response{{Name: "openai", Spam: true, Details: "job scam, confidence: 95%"}}
			case "gemini verdict":
				return true, []spamcheck.Response{{Name: "gemini", Spam: true, Details: "crypto solicitation, confidence: 91%"}}
			default:
				return true, []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}}
			}
		},
	}

	s := NewSpamFilter(det, SpamConfig{
		SpamMsg:    "detected",
		SpamDryMsg: "detected dry",
		Dry:        tc.dry,
	})

	got := s.OnMessage(tc.message, tc.checkOnly)
	assert.Equal(t, tc.wantResponse, got)

	if tc.message.From.ID == 0 {
		assert.Empty(t, det.CheckCalls())
	} else {
		assert.Len(t, det.CheckCalls(), 1)
	}

}

func TestSpamFilter_OnMessage_SpamDetected(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam detected",
		message: Message{
			Text:  "spam message",
			From:  User{ID: 1, Username: "user1"},
			Image: &Image{FileID: "123"},
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
			Meta:     spamcheck.MetaData{Images: 1},
		},
	})
}

func TestSpamFilter_OnMessage_SpamWithBothVideoAndForward(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam with both video and forward",
		message: Message{
			Text:        "spam message",
			From:        User{ID: 1, Username: "user1"},
			WithVideo:   true,
			WithForward: true,
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
			Meta:     spamcheck.MetaData{HasVideo: true, HasForward: true},
		},
	})
}

func TestSpamFilter_OnMessage_SpamWithVideoNote(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam with video note",
		message: Message{
			Text:          "spam message",
			From:          User{ID: 1, Username: "user1"},
			WithVideoNote: true,
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
			Meta:     spamcheck.MetaData{HasVideo: true},
		},
	})
}

func TestSpamFilter_OnMessage_SpamDetectedDryMode(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam detected dry mode",
		message: Message{
			Text: "spam message",
			From: User{ID: 1, Username: "user1"},
		},
		dry: true,
		wantResponse: Response{
			Text:          `detected dry: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
	})
}

func TestSpamFilter_OnMessage_SpamDetectedWithOpenaiVerdict(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam detected with openai verdict",
		message: Message{
			Text: "openai verdict",
			From: User{ID: 1, Username: "user1"},
		},
		wantResponse: Response{
			Text:          `job scam: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "openai", Spam: true, Details: "job scam, confidence: 95%"}},
		},
	})
}

func TestSpamFilter_OnMessage_SpamDetectedWithGeminiVerdictInDryMode(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "spam detected with gemini verdict in dry mode",
		message: Message{
			Text: "gemini verdict",
			From: User{ID: 1, Username: "user1"},
		},
		dry: true,
		wantResponse: Response{
			Text:          `crypto solicitation: "user1" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1"},
			CheckResults:  []spamcheck.Response{{Name: "gemini", Spam: true, Details: "crypto solicitation, confidence: 91%"}},
		},
	})
}

func TestSpamFilter_OnMessage_HamDetected(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "ham detected",
		message: Message{
			Text: "good message",
			From: User{ID: 1, Username: "user1"},
		},
		wantResponse: Response{
			CheckResults: []spamcheck.Response{{Name: "test", Spam: false, Details: "ham"}},
		},
	})
}

func TestSpamFilter_OnMessage_SystemMessageWithoutContent(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "system message without content",
		message: Message{
			Text: "system",
			From: User{ID: 0},
		},
		wantResponse: Response{},
	})
}

func TestSpamFilter_OnMessage_SystemMessageWithContent(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "system message with content",
		message: Message{
			Text:        "system message",
			From:        User{ID: 0},
			WithForward: true,
			Image:       &Image{FileID: "123"},
		},
		wantResponse: Response{},
	})
}

func TestSpamFilter_OnMessage_WithComplexLinks(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with complex links",
		message: Message{
			Text: "message https://example.com/path?param=1 http://test.com http://test.com/another",
			From: User{ID: 1, Username: "user1"},
			Entities: &[]Entity{
				{Type: "url", Offset: 8, Length: 32},
				{Type: "url", Offset: 41, Length: 15},
				{Type: "url", Offset: 57, Length: 23},
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
			Msg:      "message https://example.com/path?param=1 http://test.com http://test.com/another",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Links: 3},
		},
	})
}

func TestSpamFilter_OnMessage_WithDisplayName(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with display name",
		message: Message{
			Text: "spam message",
			From: User{ID: 1, Username: "user1", DisplayName: "User One"},
		},
		wantResponse: Response{
			Text:          `detected: "User One" (1)`,
			Send:          true,
			BanInterval:   PermanentBanDuration,
			DeleteReplyTo: true,
			User:          User{ID: 1, Username: "user1", DisplayName: "User One"},
			CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
		},
		wantRequest: spamcheck.Request{
			Msg:      "spam message",
			UserID:   "1",
			UserName: "user1",
		},
	})
}

func TestSpamFilter_OnMessage_WithTextMentionEntities(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with text mention entities",
		message: Message{
			Text: "spam message @someone",
			From: User{ID: 1, Username: "user1"},
			Entities: &[]Entity{
				{Type: "mention", Offset: 13, Length: 8},
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
			Msg:      "spam message @someone",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Mentions: 1},
		},
	})
}

func TestSpamFilter_OnMessage_WithImageCaptionMentionEntities(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with image caption mention entities",
		message: Message{
			Text: "Пишите - @zhanna_live23",
			From: User{ID: 1, Username: "user1"},
			Image: &Image{
				FileID:  "123",
				Caption: "Пишите - @zhanna_live23",
				Entities: &[]Entity{
					{Type: "mention", Offset: 9, Length: 14},
				},
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
			Msg:      "Пишите - @zhanna_live23",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Images: 1, Mentions: 1},
		},
	})
}

func TestSpamFilter_OnMessage_WithBothTextAndImageCaptionMentions(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with both text and image caption mentions",
		message: Message{
			Text: "@user1 check this",
			From: User{ID: 1, Username: "user1"},
			Entities: &[]Entity{
				{Type: "mention", Offset: 0, Length: 6},
			},
			Image: &Image{
				FileID:  "123",
				Caption: "contact @someone",
				Entities: &[]Entity{
					{Type: "mention", Offset: 8, Length: 8},
				},
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
			Msg:      "@user1 check this",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Images: 1, Mentions: 2},
		},
	})
}

func TestSpamFilter_OnMessage_WithUrlEntityInText(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with url entity in text",
		message: Message{
			Text: "check example.com for details",
			From: User{ID: 1, Username: "user1"},
			Entities: &[]Entity{
				{Type: "url", Offset: 6, Length: 11},
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
			Msg:      "check example.com for details",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Links: 1},
		},
	})
}

func TestSpamFilter_OnMessage_WithTextLinkEntityInImageCaption(t *testing.T) {
	runSpamFilterOnMessageCase(t, spamFilterOnMessageCase{
		name: "with text_link entity in image caption",
		message: Message{
			Text: "Click here for details",
			From: User{ID: 1, Username: "user1"},
			Image: &Image{
				FileID:  "123",
				Caption: "Click here for details",
				Entities: &[]Entity{
					{Type: "text_link", Offset: 0, Length: 10, URL: "https://example.com"},
				},
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
			Msg:      "Click here for details",
			UserID:   "1",
			UserName: "user1",
			Meta:     spamcheck.MetaData{Images: 1, Links: 1},
		},
	})
}
