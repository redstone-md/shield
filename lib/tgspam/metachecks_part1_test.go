package tgspam

import (
	"github.com/stretchr/testify/assert"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"testing"
)

func TestLinksCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		limit    int
		expected spamcheck.Response
	}{
		{
			name: "No links",
			req: spamcheck.Request{
				Msg: "This is a message without links.",
			},
			limit:    2,
			expected: spamcheck.Response{Name: "links", Spam: false, Details: "links 0/2"},
		},
		{
			name: "Below limit with meta",
			req: spamcheck.Request{
				Msg: "Check out this link: http://example.com",
				Meta: spamcheck.MetaData{
					Links: 1,
				},
			},
			limit:    2,
			expected: spamcheck.Response{Name: "links", Spam: false, Details: "links 1/2"},
		},
		{
			name: "Above limit with meta",
			req: spamcheck.Request{
				Msg: "Too many links here: http://example.com and https://example.org",
				Meta: spamcheck.MetaData{
					Links: 3,
				},
			},
			limit: 2,
			expected: spamcheck.Response{
				Name:    "links",
				Spam:    true,
				Details: "too many links 3/2",
				RuleID:  "links",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "Above limit by counting in message",
			req: spamcheck.Request{
				Msg: "Too many links here: http://example.com and https://example.org",
			},
			limit: 1,
			expected: spamcheck.Response{
				Name:    "links",
				Spam:    true,
				Details: "too many links 2/1",
				RuleID:  "links",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := LinksCheck(tt.limit)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestLinkOnlyCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		expected spamcheck.Response
	}{
		{
			name: "with no links",
			req: spamcheck.Request{
				Msg: "This is a message without links.",
			},
			expected: spamcheck.Response{Name: "link-only", Spam: false, Details: "message contains text"},
		},
		{
			name: "with only links",
			req: spamcheck.Request{
				Msg: "http://example.com https://example.org",
			},
			expected: spamcheck.Response{
				Name:    "link-only",
				Spam:    true,
				Details: "message contains links only",
				RuleID:  "link-only",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "with a single link, no text",
			req: spamcheck.Request{
				Msg: " https://example.org ",
			},
			expected: spamcheck.Response{
				Name:    "link-only",
				Spam:    true,
				Details: "message contains links only",
				RuleID:  "link-only",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "with text and links",
			req: spamcheck.Request{
				Msg: "Check out this link: http://example.com",
			},
			expected: spamcheck.Response{Name: "link-only", Spam: false, Details: "message contains text"},
		},
		{
			name: "Empty message",
			req: spamcheck.Request{
				Msg: "",
			},
			expected: spamcheck.Response{Name: "link-only", Spam: false, Details: "empty message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := LinkOnlyCheck()
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestImagesCheck(t *testing.T) {
	tests := []struct {
		name       string
		minTextLen int
		req        spamcheck.Request
		expected   spamcheck.Response
	}{
		{
			name: "no images and text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text.", Meta: spamcheck.MetaData{Images: 0}},
			expected: spamcheck.Response{Name: "images", Spam: false, Details: "text or no images"},
		},
		{
			name: "images with long text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text and an image.", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{Name: "images", Spam: false, Details: "text or no images"},
		},
		{
			name: "images without text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{
				Name:    "images",
				Spam:    true,
				Details: "image without text",
				RuleID:  "images",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "multiple images without text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{Images: 3}},
			expected: spamcheck.Response{
				Name:    "images",
				Spam:    true,
				Details: "image without text",
				RuleID:  "images",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "image with short text below threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "@Angelina_crypto717", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{
				Name:    "images",
				Spam:    true,
				Details: "image with short text (19 chars)",
				RuleID:  "images",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "image with text exactly at threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is exactly fifty characters long, I promise!!", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{Name: "images", Spam: false, Details: "text or no images"},
		},
		{
			name: "image with text above threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is a longer message that exceeds the minimum text length threshold", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{Name: "images", Spam: false, Details: "text or no images"},
		},
		{
			name: "no images with short text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "short", Meta: spamcheck.MetaData{Images: 0}},
			expected: spamcheck.Response{Name: "images", Spam: false, Details: "text or no images"},
		},
		{
			name: "image with cyrillic short text", minTextLen: 50,
			req:      spamcheck.Request{Msg: "Менеджер - @test123", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{
				Name:    "images",
				Spam:    true,
				Details: "image with short text (19 chars)",
				RuleID:  "images",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "image with empty text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{Images: 1}},
			expected: spamcheck.Response{
				Name:    "images",
				Spam:    true,
				Details: "image without text",
				RuleID:  "images",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := ImagesCheck(tt.minTextLen)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestVideosCheck(t *testing.T) {
	tests := []struct {
		name       string
		minTextLen int
		req        spamcheck.Request
		expected   spamcheck.Response
	}{
		{
			name: "no video and text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text.", Meta: spamcheck.MetaData{HasVideo: false}},
			expected: spamcheck.Response{Name: "videos", Spam: false, Details: "text or no video"},
		},
		{
			name: "video with long text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text and a video.", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{Name: "videos", Spam: false, Details: "text or no video"},
		},
		{
			name: "video without text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{
				Name:    "videos",
				Spam:    true,
				Details: "video without text",
				RuleID:  "videos",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "video with short text below threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "@spam_channel", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{
				Name:    "videos",
				Spam:    true,
				Details: "video with short text (13 chars)",
				RuleID:  "videos",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "video with text at threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is exactly fifty characters long, I promise!!", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{Name: "videos", Spam: false, Details: "text or no video"},
		},
		{
			name: "video with text above threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is a longer message that exceeds the minimum text length threshold", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{Name: "videos", Spam: false, Details: "text or no video"},
		},
		{
			name: "no video with short text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "short", Meta: spamcheck.MetaData{HasVideo: false}},
			expected: spamcheck.Response{Name: "videos", Spam: false, Details: "text or no video"},
		},
		{
			name: "video with empty text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{HasVideo: true}},
			expected: spamcheck.Response{
				Name:    "videos",
				Spam:    true,
				Details: "video without text",
				RuleID:  "videos",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := VideosCheck(tt.minTextLen)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestForwardedCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		expected spamcheck.Response
	}{
		{
			name: "message is forwarded",
			req: spamcheck.Request{
				Meta: spamcheck.MetaData{
					HasForward: true,
				},
			},
			expected: spamcheck.Response{
				Name:    "forward",
				Spam:    true,
				Details: "forwarded message",
				RuleID:  "forward",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "message is not forwarded",
			req: spamcheck.Request{
				Meta: spamcheck.MetaData{
					HasForward: false,
				},
			},
			expected: spamcheck.Response{
				Name:    "forward",
				Spam:    false,
				Details: "not a forwarded message",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := ForwardedCheck()
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestAudioCheck(t *testing.T) {
	tests := []struct {
		name       string
		minTextLen int
		req        spamcheck.Request
		expected   spamcheck.Response
	}{
		{
			name: "no audio and text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text.", Meta: spamcheck.MetaData{HasAudio: false}},
			expected: spamcheck.Response{Name: "audio", Spam: false, Details: "text or no audio"},
		},
		{
			name: "audio with long text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "This is a message with text and an audio.", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{Name: "audio", Spam: false, Details: "text or no audio"},
		},
		{
			name: "audio without text, minTextLen=0", minTextLen: 0,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{
				Name:    "audio",
				Spam:    true,
				Details: "audio without text",
				RuleID:  "audio",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "audio with short text below threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "@spam_channel", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{
				Name:    "audio",
				Spam:    true,
				Details: "audio with short text (13 chars)",
				RuleID:  "audio",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "audio with text at threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is exactly fifty characters long, I promise!!", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{Name: "audio", Spam: false, Details: "text or no audio"},
		},
		{
			name: "audio with text above threshold", minTextLen: 50,
			req:      spamcheck.Request{Msg: "This is a longer message that exceeds the minimum text length threshold", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{Name: "audio", Spam: false, Details: "text or no audio"},
		},
		{
			name: "no audio with short text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "short", Meta: spamcheck.MetaData{HasAudio: false}},
			expected: spamcheck.Response{Name: "audio", Spam: false, Details: "text or no audio"},
		},
		{
			name: "audio with empty text, minTextLen=50", minTextLen: 50,
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{HasAudio: true}},
			expected: spamcheck.Response{
				Name:    "audio",
				Spam:    true,
				Details: "audio without text",
				RuleID:  "audio",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := AudioCheck(tt.minTextLen)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestContactCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		expected spamcheck.Response
	}{
		{
			name:     "no contact and text",
			req:      spamcheck.Request{Msg: "This is a message with text.", Meta: spamcheck.MetaData{HasContact: false}},
			expected: spamcheck.Response{Name: "contact", Spam: false, Details: "no contact without text"},
		},
		{
			name:     "contact with text",
			req:      spamcheck.Request{Msg: "This is a message with text and a contact.", Meta: spamcheck.MetaData{HasContact: true}},
			expected: spamcheck.Response{Name: "contact", Spam: false, Details: "no contact without text"},
		},
		{
			name:     "contact without text",
			req:      spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{HasContact: true}},
			expected: spamcheck.Response{
				Name:    "contact",
				Spam:    true,
				Details: "contact without text",
				RuleID:  "contact",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := ContactCheck()
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestKeyboardCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		expected spamcheck.Response
	}{
		{
			name: "No keyboard",
			req: spamcheck.Request{
				Msg: "This is a message with text.",
				Meta: spamcheck.MetaData{
					HasKeyboard: false,
				},
			},
			expected: spamcheck.Response{Name: "keyboard", Spam: false, Details: "no keyboard"},
		},
		{
			name: "Message with keyboard",
			req: spamcheck.Request{
				Msg: "This is a message with text and buttons.",
				Meta: spamcheck.MetaData{
					HasKeyboard: true,
				},
			},
			expected: spamcheck.Response{
				Name:    "keyboard",
				Spam:    true,
				Details: "message with keyboard",
				RuleID:  "keyboard",
				Score:   1.0,
				Weight:  1.0,
			},
		},
		{
			name: "Empty message with keyboard",
			req: spamcheck.Request{
				Msg: "",
				Meta: spamcheck.MetaData{
					HasKeyboard: true,
				},
			},
			expected: spamcheck.Response{
				Name:    "keyboard",
				Spam:    true,
				Details: "message with keyboard",
				RuleID:  "keyboard",
				Score:   1.0,
				Weight:  1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := KeyboardCheck()
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}
