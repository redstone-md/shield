package events

import (
	"context"
	"errors"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/events/mocks"
	"testing"
)

func TestTelegramListener_isChatAllowed(t *testing.T) {
	testCases := []struct {
		name       string
		fromChat   int64
		chatID     int64
		testingIDs []int64
		expect     bool
	}{
		{
			name:       "Chat is allowed - fromChat equals chatID",
			fromChat:   123,
			chatID:     123,
			testingIDs: []int64{},
			expect:     true,
		},
		{
			name:       "Chat is allowed - fromChat in testingIDs",
			fromChat:   456,
			chatID:     123,
			testingIDs: []int64{456},
			expect:     true,
		},
		{
			name:       "Chat is not allowed",
			fromChat:   789,
			chatID:     123,
			testingIDs: []int64{456},
			expect:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			listener := TelegramListener{
				chatID:     tc.chatID,
				TestingIDs: tc.testingIDs,
			}
			result := listener.isChatAllowed(tc.fromChat)
			assert.Equal(t, tc.expect, result)
		})
	}
}

func TestTelegramListener_isAdminChat(t *testing.T) {
	testCases := []struct {
		name     string
		fromChat int64
		fromUser string
		fromID   int64
		chatID   int64
		expect   bool
	}{
		{
			name:     "allowed, fromUser is superuser and fromChat equals chatID",
			fromChat: 123,
			chatID:   123,
			fromUser: "umputun",
			expect:   true,
		},
		{
			name:     "not allowed, fromUser is superuser and fromChat is not chatID",
			fromChat: 456,
			chatID:   123777,
			fromUser: "umputun",
			expect:   false,
		},
		{
			name:     "not allowed, fromUser is not superuser and fromChat is chatID",
			fromChat: 456,
			chatID:   123,
			fromUser: "user",
			expect:   false,
		},
		{
			name:     "not allowed, fromUser is not superuser but fromChat is chatID",
			fromChat: 123,
			chatID:   123,
			fromUser: "user",
			expect:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			listener := TelegramListener{
				adminChatID: tc.chatID,
				SuperUsers:  SuperUsers{"umputun"},
			}
			result := listener.isAdminChat(tc.fromChat, tc.fromUser, tc.fromID)
			assert.Equal(t, tc.expect, result)
		})
	}
}

func TestSuperUser_IsSuper(t *testing.T) {
	tests := []struct {
		name     string
		super    SuperUsers
		userName string
		userID   int64
		want     bool
	}{
		{
			name:     "User is a super user",
			super:    SuperUsers{"Alice", "Bob"},
			userName: "Alice",
			want:     true,
		},
		{
			name:     "User is a super user by ID",
			super:    SuperUsers{"Alice", "Bob", "123"},
			userName: "blah",
			userID:   123,
			want:     true,
		},
		{
			name:     "User is not a super user",
			super:    SuperUsers{"Alice", "Bob"},
			userName: "Charlie",
			want:     false,
		},
		{
			name:   "User is not a super user ny ID",
			super:  SuperUsers{"Alice", "Bob", "123"},
			userID: 789,
			want:   false,
		},
		{
			name:     "User is a super user with slash prefix",
			super:    SuperUsers{"/Alice", "Bob"},
			userName: "Alice",
			want:     true,
		},
		{
			name:     "User is not a super user with slash prefix",
			super:    SuperUsers{"/Alice", "Bob"},
			userName: "Charlie",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.super.IsSuper(tt.userName, tt.userID))
		})
	}
}

func TestUpdateSupers(t *testing.T) {
	tests := []struct {
		name            string
		superUsers      SuperUsers
		chatAdmins      []tbapi.ChatMember
		adminFetchError error
		expectedResult  []string
		expectedErr     bool
	}{
		{
			name:           "empty admins",
			chatAdmins:     make([]tbapi.ChatMember, 0),
			expectedResult: make([]string, 0),
			expectedErr:    false,
		},
		{
			name:           "non-empty admin usernames",
			chatAdmins:     []tbapi.ChatMember{{User: &tbapi.User{UserName: "", ID: 1}}, {User: &tbapi.User{UserName: "admin2", ID: 2}}},
			expectedResult: []string{"1", "2"},
			expectedErr:    false,
		},
		{
			name: "non-empty admin user ids",
			chatAdmins: []tbapi.ChatMember{
				{User: &tbapi.User{UserName: "admin1", ID: 21}},
				{User: &tbapi.User{UserName: "admin2", ID: 22}}},
			expectedResult: []string{"21", "22"},
			expectedErr:    false,
		},
		{
			name:       "non-empty admin usernames, existing supers",
			superUsers: SuperUsers{"super1"},
			chatAdmins: []tbapi.ChatMember{
				{User: &tbapi.User{UserName: "admin1", ID: 1}},
				{User: &tbapi.User{UserName: "admin2", ID: 2}}},
			expectedResult: []string{"super1", "1", "2"},
			expectedErr:    false,
		},
		{
			name:       "non-empty admin usernames, existing supers with duplicate",
			superUsers: SuperUsers{"admin1"},
			chatAdmins: []tbapi.ChatMember{
				{User: &tbapi.User{UserName: "admin1", ID: 1}},
				{User: &tbapi.User{UserName: "admin2", ID: 2}}},
			expectedResult: []string{"admin1", "2"},
			expectedErr:    false,
		},
		{
			name: "admin usernames with empty string",
			chatAdmins: []tbapi.ChatMember{
				{User: &tbapi.User{UserName: "admin1", ID: 1}},
				{User: &tbapi.User{UserName: ""}},
				{User: &tbapi.User{UserName: "admin2", ID: 2}}},
			expectedResult: []string{"1", "2"},
			expectedErr:    false,
		},
		{
			name:            "fetching admins returns error",
			chatAdmins:      []tbapi.ChatMember{},
			adminFetchError: errors.New("fetch error"),
			expectedResult:  []string{},
			expectedErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &TelegramListener{
				TbAPI: &mocks.TbAPIMock{
					GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
						return tt.chatAdmins, tt.adminFetchError
					},
				},
				SuperUsers: tt.superUsers,
			}

			err := l.updateSupers()
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedResult, l.SuperUsers, "Expected: %v, got: %v", tt.expectedResult, l.SuperUsers)
			}
		})
	}
}

func TestProcNewChatMemberMessage(t *testing.T) {
	type addMessageArgs struct {
		Msg      string
		ChatID   int64
		UserID   int64
		UserName string
		MsgID    int
	}

	tests := []struct {
		name                   string
		update                 tbapi.Update
		expectedError          bool
		expectedAddMessageArgs []addMessageArgs
		AddMessageMockReturn   error
	}{
		{
			name: "new chat member added by admin successfully",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat: tbapi.Chat{ID: 123},
					From: &tbapi.User{UserName: "superuser1", ID: 77},
					NewChatMembers: []tbapi.User{
						{ID: 88, UserName: "user1"},
					},
					MessageID: 22,
				},
			},
			expectedError: false,
			expectedAddMessageArgs: []addMessageArgs{
				{
					Msg:      "new_123_88",
					ChatID:   123,
					UserID:   88,
					UserName: "",
					MsgID:    22,
				},
			},
		},
		{
			name: "new chat member self joined successfully",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat: tbapi.Chat{ID: 123},
					From: &tbapi.User{UserName: "user1", ID: 88},
					NewChatMembers: []tbapi.User{
						{ID: 88, UserName: "user1"},
					},
					MessageID: 22,
				},
			},
			expectedError: false,
			expectedAddMessageArgs: []addMessageArgs{
				{
					Msg:      "new_123_88",
					ChatID:   123,
					UserID:   88,
					UserName: "",
					MsgID:    22,
				},
			},
		},
		{
			name: "2 new chat member joined successfully",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat: tbapi.Chat{ID: 123},
					From: &tbapi.User{UserName: "superuser1", ID: 77},
					NewChatMembers: []tbapi.User{
						{ID: 88, UserName: "user1"},
						{ID: 99, UserName: "user1"},
					},
					MessageID: 22,
				},
			},
			expectedError:          false,
			expectedAddMessageArgs: []addMessageArgs{},
		},
		{
			name: "empty chat members in the message",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 123},
					NewChatMembers: []tbapi.User{},
				},
			},
			expectedError:          false,
			expectedAddMessageArgs: []addMessageArgs{},
		},
		{
			name: "message from unauthorized chat",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat: tbapi.Chat{ID: 999},
					NewChatMembers: []tbapi.User{
						{ID: 88, UserName: "user1"},
					},
				},
			},
			expectedError:          false,
			expectedAddMessageArgs: []addMessageArgs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locator, teardown := prepTestLocator(t)
			defer teardown()

			l := &TelegramListener{
				Locator: locator,
				chatID:  123,
			}

			err := l.procNewChatMemberMessage(context.Background(), tt.update)
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			for _, args := range tt.expectedAddMessageArgs {
				msgMeta, found := l.Locator.Message(context.Background(), args.Msg)
				assert.True(t, found)
				assert.Equal(t, args.ChatID, msgMeta.ChatID)
				assert.Equal(t, args.UserID, msgMeta.UserID)
				assert.Equal(t, args.UserName, msgMeta.UserName)
				assert.Equal(t, args.MsgID, msgMeta.MsgID)
			}
		})
	}
}
