package tgspam

import (
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMentionsCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		limit    int
		expected spamcheck.Response
	}{
		{
			name: "No mentions",
			req: spamcheck.Request{
				Msg: "This is a message without mentions.",
				Meta: spamcheck.MetaData{
					Mentions: 0,
				},
			},
			limit:    5,
			expected: spamcheck.Response{Name: "mentions", Spam: false, Details: "mentions 0/5"},
		},
		{
			name: "Below limit",
			req: spamcheck.Request{
				Msg: "This message mentions @user1 and @user2.",
				Meta: spamcheck.MetaData{
					Mentions: 2,
				},
			},
			limit:    5,
			expected: spamcheck.Response{Name: "mentions", Spam: false, Details: "mentions 2/5"},
		},
		{
			name: "At limit",
			req: spamcheck.Request{
				Msg: "This message mentions five users: @user1 @user2 @user3 @user4 @user5",
				Meta: spamcheck.MetaData{
					Mentions: 5,
				},
			},
			limit:    5,
			expected: spamcheck.Response{Name: "mentions", Spam: false, Details: "mentions 5/5"},
		},
		{
			name: "Above limit",
			req: spamcheck.Request{
				Msg: "Message with too many mentions: @user1 @user2 @user3 @user4 @user5 @user6",
				Meta: spamcheck.MetaData{
					Mentions: 6,
				},
			},
			limit: 5,
			expected: spamcheck.Response{
				Name:    "mentions",
				Spam:    true,
				Details: "too many mentions 6/5",
				RuleID:  "mentions", Score: 1.0, Weight: 1.0,
			},
		},
		{
			name: "Disabled check",
			req: spamcheck.Request{
				Msg: "Message with many mentions: @user1 @user2 @user3 @user4 @user5 @user6",
				Meta: spamcheck.MetaData{
					Mentions: 6,
				},
			},
			limit:    -1,
			expected: spamcheck.Response{Name: "mentions", Spam: false, Details: "check disabled"},
		},
		{
			name: "Zero limit, no mentions",
			req: spamcheck.Request{
				Msg: "Message with no mentions",
				Meta: spamcheck.MetaData{
					Mentions: 0,
				},
			},
			limit:    0,
			expected: spamcheck.Response{Name: "mentions", Spam: false, Details: "mentions 0/0"},
		},
		{
			name: "Zero limit, with mentions",
			req: spamcheck.Request{
				Msg: "Message with mentions: @user1",
				Meta: spamcheck.MetaData{
					Mentions: 1,
				},
			},
			limit: 0,
			expected: spamcheck.Response{
				Name:    "mentions",
				Spam:    true,
				Details: "too many mentions 1/0",
				RuleID:  "mentions", Score: 1.0, Weight: 1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := MentionsCheck(tt.limit)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestUsernameSymbolsCheck(t *testing.T) {
	tests := []struct {
		name              string
		req               spamcheck.Request
		prohibitedSymbols string
		expected          spamcheck.Response
	}{
		{
			name: "No username",
			req: spamcheck.Request{
				UserName: "",
			},
			prohibitedSymbols: "@",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    false,
				Details: "no username",
			},
		},
		{
			name: "Disabled check",
			req: spamcheck.Request{
				UserName: "user@name",
			},
			prohibitedSymbols: "",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    false,
				Details: "check disabled",
			},
		},
		{
			name: "Username contains prohibited symbol",
			req: spamcheck.Request{
				UserName: "user@name",
			},
			prohibitedSymbols: "@",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    true,
				Details: "username contains prohibited symbol '@'",
				RuleID:  "username-symbols", Score: 1.0, Weight: 1.0,
			},
		},
		{
			name: "Username contains one of multiple prohibited symbols",
			req: spamcheck.Request{
				UserName: "user#name",
			},
			prohibitedSymbols: "@#$",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    true,
				Details: "username contains prohibited symbol '#'",
				RuleID:  "username-symbols", Score: 1.0, Weight: 1.0,
			},
		},
		{
			name: "Username does not contain prohibited symbols",
			req: spamcheck.Request{
				UserName: "username",
			},
			prohibitedSymbols: "@#$",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    false,
				Details: "no prohibited symbols in username",
			},
		},
		{
			name: "Username with special characters but not prohibited",
			req: spamcheck.Request{
				UserName: "user-name_123",
			},
			prohibitedSymbols: "@#$",
			expected: spamcheck.Response{
				Name:    "username-symbols",
				Spam:    false,
				Details: "no prohibited symbols in username",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := UsernameSymbolsCheck(tt.prohibitedSymbols)
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}

func TestGiveawayCheck(t *testing.T) {
	tests := []struct {
		name     string
		req      spamcheck.Request
		expected spamcheck.Response
	}{
		{
			name:     "no giveaway",
			req:      spamcheck.Request{Meta: spamcheck.MetaData{HasGiveaway: false}},
			expected: spamcheck.Response{Name: "giveaway", Spam: false, Details: "no giveaway"},
		},
		{
			name:     "giveaway",
			req:      spamcheck.Request{Meta: spamcheck.MetaData{HasGiveaway: true}},
			expected: spamcheck.Response{Name: "giveaway", Spam: true, Details: "giveaway message", RuleID: "giveaway", Score: 1.0, Weight: 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := GiveawayCheck()
			assert.Equal(t, tt.expected, check(tt.req))
		})
	}
}
