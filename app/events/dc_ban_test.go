package events

import (
	"context"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/rules"
)

// realAvatarFileID is a real Bot API ProfilePhoto file_id that decodes to DC 2
// (from github.com/gotd/td/fileid test fixtures). Using real data keeps these
// tests honest about the file_id format without hand-rolling binary payloads.
const realAvatarFileID = "AQADAgAD7a8xG75QcEkACAMAA2jAIuIW____cd7THMWjNdIiBA"

func dcJoinUpdate(chatID, userID int64, userName, oldStatus, newStatus string, isBot bool) tbapi.ChatMemberUpdated {
	return tbapi.ChatMemberUpdated{
		Chat:          tbapi.Chat{ID: chatID},
		From:          tbapi.User{ID: userID, UserName: userName},
		OldChatMember: tbapi.ChatMember{Status: oldStatus},
		NewChatMember: tbapi.ChatMember{
			Status: newStatus,
			User:   &tbapi.User{ID: userID, UserName: userName, IsBot: isBot},
		},
	}
}

func newDCBanTestListener(t *testing.T, banned []int, photosResp tbapi.UserProfilePhotos) (
	*TelegramListener, *actionExecutorSpy, *mocks.TbAPIMock, *mocks.LocatorMock,
) {
	t.Helper()
	api := &mocks.TbAPIMock{
		GetUserProfilePhotosFunc: func(tbapi.UserProfilePhotosConfig) (tbapi.UserProfilePhotos, error) {
			return photosResp, nil
		},
		SendFunc: func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	loc := &mocks.LocatorMock{
		GetUserDCFunc: func(context.Context, int64) (int, bool) { return 0, false },
		SetUserDCFunc: func(context.Context, int64, int) error { return nil },
	}
	spy := &actionExecutorSpy{}
	l := &TelegramListener{
		TbAPI:          api,
		Locator:        loc,
		ActionExecutor: spy,
		SuperUsers:     SuperUsers{"super"},
		primChatIDs:    []int64{123},
		chatIDsSet:     map[int64]struct{}{123: {}},
		adminChatID:    999,
	}
	l.bannedDCs.Store(newDCSet(banned))
	return l, spy, api, loc
}

func TestProcChatMemberUpdated_BansByDC(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, spy, api, loc := newDCBanTestListener(t, []int{2}, photos)

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "spammer", "left", "member", false))
	require.NoError(t, err)

	require.Len(t, spy.banCalls, 1, "one ban per primary chat")
	assert.Equal(t, int64(42), spy.banCalls[0].userID)
	assert.Equal(t, int64(123), spy.banCalls[0].chatID)
	assert.Equal(t, bot.PermanentBanDuration, spy.banCalls[0].duration)
	assert.False(t, spy.banCalls[0].restrict)

	require.Len(t, api.GetUserProfilePhotosCalls(), 1, "avatar fetched on cache miss")
	assert.Equal(t, int64(42), api.GetUserProfilePhotosCalls()[0].Config.UserID)
	require.Len(t, loc.SetUserDCCalls(), 1, "decoded dc cached")
	assert.Equal(t, 2, loc.SetUserDCCalls()[0].Dc)
	require.Len(t, api.SendCalls(), 1, "admin chat notified")
}

func TestProcChatMemberUpdated_UsesCachedDC(t *testing.T) {
	l, spy, api, loc := newDCBanTestListener(t, []int{2}, tbapi.UserProfilePhotos{})
	loc.GetUserDCFunc = func(context.Context, int64) (int, bool) { return 2, true }

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "u", "left", "member", false))
	require.NoError(t, err)

	assert.Len(t, spy.banCalls, 1)
	assert.Empty(t, api.GetUserProfilePhotosCalls(), "cached dc must skip photo fetch")
	assert.Empty(t, loc.SetUserDCCalls(), "cache hit must not re-write")
}

func TestProcChatMemberUpdated_SkipCases(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	cases := []struct {
		name   string
		u      tbapi.ChatMemberUpdated
		banned []int
	}{
		{"non-monitored chat", dcJoinUpdate(777, 42, "u", "left", "member", false), []int{2}},
		{"not a join promotion", dcJoinUpdate(123, 42, "u", "member", "administrator", false), []int{2}},
		{"not a join left to left", dcJoinUpdate(123, 42, "u", "left", "left", false), []int{2}},
		{"bot account", dcJoinUpdate(123, 42, "u", "left", "member", true), []int{2}},
		{"administrator join", dcJoinUpdate(123, 42, "u", "left", "administrator", false), []int{2}},
		{"creator join", dcJoinUpdate(123, 42, "u", "left", "creator", false), []int{2}},
		{"super-user", dcJoinUpdate(123, 42, "super", "left", "member", false), []int{2}},
		{"gate disabled empty set", dcJoinUpdate(123, 42, "u", "left", "member", false), nil},
		{"dc not in banned set", dcJoinUpdate(123, 42, "u", "left", "member", false), []int{3, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, spy, _, _ := newDCBanTestListener(t, tc.banned, photos)
			err := l.procChatMemberUpdated(context.Background(), tc.u)
			require.NoError(t, err)
			assert.Empty(t, spy.banCalls, "must not ban")
		})
	}
}

func TestProcChatMemberUpdated_NoProfilePhotoSkips(t *testing.T) {
	l, spy, api, _ := newDCBanTestListener(t, []int{2}, tbapi.UserProfilePhotos{TotalCount: 0})

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "u", "left", "member", false))
	require.NoError(t, err)

	assert.Empty(t, spy.banCalls)
	require.Len(t, api.GetUserProfilePhotosCalls(), 1, "photo was fetched before skipping")
}

func TestProcChatMemberUpdated_DryModePropagates(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, spy, _, _ := newDCBanTestListener(t, []int{2}, photos)
	l.Dry = true

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "u", "left", "member", false))
	require.NoError(t, err)

	require.Len(t, spy.banCalls, 1)
	assert.True(t, spy.banCalls[0].dry, "dry flag must propagate to the ban request")
	assert.False(t, spy.banCalls[0].training)
}

func TestProcChatMemberUpdated_BanErrorBubblesUp(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, spy, api, _ := newDCBanTestListener(t, []int{2}, photos)
	spy.applyBan = func(context.Context, banRequest) error { return assertError("ban failed") }

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "u", "left", "member", false))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dc-ban user 42")
	assert.Empty(t, api.SendCalls(), "admin must not be notified when the ban itself failed")
}

func TestProcChatMemberUpdated_ApprovedUserSkips(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, spy, api, _ := newDCBanTestListener(t, []int{2}, photos)
	l.Bot = &mocks.BotMock{IsApprovedUserFunc: func(int64) bool { return true }}

	err := l.procChatMemberUpdated(context.Background(), dcJoinUpdate(123, 42, "legit_user", "left", "member", false))
	require.NoError(t, err)

	assert.Empty(t, spy.banCalls, "approved user must not be re-banned by the dc gate")
	assert.Empty(t, api.GetUserProfilePhotosCalls(), "approved user must be skipped before classification")
	assert.Empty(t, api.SendCalls(), "no admin notification for approved user")
}

func TestNotifyDCBan_HTMLAndUnbanButton(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, _, api, _ := newDCBanTestListener(t, []int{2}, photos)

	u := dcJoinUpdate(-1001420186506, 4242, "gpig_stepan", "left", "member", false)
	l.notifyDCBan(u, u.NewChatMember.User, 2)

	require.Len(t, api.SendCalls(), 1)
	msg := api.SendCalls()[0].C.(tbapi.MessageConfig)
	assert.Equal(t, tbapi.ModeHTML, msg.ParseMode)
	assert.Contains(t, msg.Text, `[DC GATE] <a href="tg://user?id=4242">gpig_stepan</a> (4242)`)
	assert.Contains(t, msg.Text, "забанен по DC 2 при входе в чат -1001420186506")
	assert.NotContains(t, msg.Text, `\_`, "no markdown escaping must leak into HTML mode")

	kb, ok := msg.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
	require.True(t, ok, "message must carry an inline keyboard")
	require.Len(t, kb.InlineKeyboard, 1)
	require.Len(t, kb.InlineKeyboard[0], 1)
	assert.Equal(t, "Разбанить", kb.InlineKeyboard[0][0].Text)
	assert.Equal(t, "DCA4242", *kb.InlineKeyboard[0][0].CallbackData)
}

func newDCGateAdmin(t *testing.T) (*mocks.TbAPIMock, *mocks.BotMock, *admin) {
	t.Helper()
	api := &mocks.TbAPIMock{
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
	}
	botMock := &mocks.BotMock{AddApprovedUserFunc: func(int64, string) error { return nil }}
	adm := &admin{
		tbAPI:       api,
		bot:         botMock,
		primChatIDs: []int64{123},
		adminChatID: 456,
	}
	return api, botMock, adm
}

func dcGateQuery(data, text string) *tbapi.CallbackQuery {
	return &tbapi.CallbackQuery{
		ID:   "q1",
		Data: data,
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			Text:      text,
		},
		From: &tbapi.User{UserName: "admin", ID: 111},
	}
}

func TestDCGateUnban_TwoStepFlow(t *testing.T) {
	const msgText = "[DC GATE] gpig_stepan (4242) забанен по DC 2 при входе в чат -1001420186506"

	t.Run("ask swaps keyboard for confirmation", func(t *testing.T) {
		api, _, adm := newDCGateAdmin(t)
		require.NoError(t, adm.callbackDCUnbanAsk(dcGateQuery("DCA4242", msgText)))

		require.Len(t, api.SendCalls(), 1)
		edit := api.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig)
		require.Len(t, edit.ReplyMarkup.InlineKeyboard, 1)
		require.Len(t, edit.ReplyMarkup.InlineKeyboard[0], 2)
		assert.Equal(t, "Подтвердить разбан", edit.ReplyMarkup.InlineKeyboard[0][0].Text)
		assert.Equal(t, "DCU4242", *edit.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
		assert.Equal(t, "Отмена", edit.ReplyMarkup.InlineKeyboard[0][1].Text)
		assert.Equal(t, "DCX4242", *edit.ReplyMarkup.InlineKeyboard[0][1].CallbackData)
	})

	t.Run("confirm unbans everywhere and approves", func(t *testing.T) {
		api, botMock, adm := newDCGateAdmin(t)
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", msgText)))

		var unbanCalls []tbapi.UnbanChatMemberConfig
		for _, call := range api.RequestCalls() {
			if cfg, ok := call.C.(tbapi.UnbanChatMemberConfig); ok {
				unbanCalls = append(unbanCalls, cfg)
			}
		}
		require.Len(t, unbanCalls, 1, "unban in every primary chat")
		assert.Equal(t, int64(4242), unbanCalls[0].UserID)
		assert.Equal(t, int64(123), unbanCalls[0].ChatID)
		assert.True(t, unbanCalls[0].OnlyIfBanned)

		require.Len(t, botMock.AddApprovedUserCalls(), 1, "user must be whitelisted against the gate")
		assert.Equal(t, int64(4242), botMock.AddApprovedUserCalls()[0].ID)
		assert.Equal(t, "gpig_stepan", botMock.AddApprovedUserCalls()[0].Name)

		require.Len(t, api.SendCalls(), 1)
		edit := api.SendCalls()[0].C.(tbapi.EditMessageTextConfig)
		assert.Contains(t, edit.Text, "разбанено администратором")
		assert.Contains(t, edit.Text, `<a href="tg://user?id=111">admin</a>`)
		assert.Empty(t, edit.ReplyMarkup.InlineKeyboard, "keyboard must be cleared")
	})

	t.Run("confirm in training mode skips telegram unban but still approves", func(t *testing.T) {
		api, botMock, adm := newDCGateAdmin(t)
		adm.trainingMode = true
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", msgText)))

		for _, call := range api.RequestCalls() {
			_, isUnban := call.C.(tbapi.UnbanChatMemberConfig)
			assert.False(t, isUnban, "training mode must not call telegram unban")
		}
		require.Len(t, botMock.AddApprovedUserCalls(), 1)
	})

	t.Run("router dispatches DC prefixes", func(t *testing.T) {
		for _, tc := range []struct {
			data      string
			wantEdit  bool // EditMessageReplyMarkupConfig via send()
			wantUnban bool
			wantApprv bool
		}{
			{"DCA4242", true, false, false},
			{"DCU4242", false, true, true},
			{"DCX4242", true, false, false},
		} {
			t.Run(tc.data, func(t *testing.T) {
				api, botMock, adm := newDCGateAdmin(t)
				require.NoError(t, adm.InlineCallbackHandler(context.Background(), dcGateQuery(tc.data, msgText)))

				var gotEdit bool
				for _, call := range api.SendCalls() {
					if _, ok := call.C.(tbapi.EditMessageReplyMarkupConfig); ok {
						gotEdit = true
					}
				}
				assert.Equal(t, tc.wantEdit, gotEdit, "keyboard edit")

				var unbans int
				for _, call := range api.RequestCalls() {
					if _, ok := call.C.(tbapi.UnbanChatMemberConfig); ok {
						unbans++
					}
				}
				if tc.wantUnban {
					assert.Equal(t, 1, unbans, "hard unban in every primary chat")
				} else {
					assert.Zero(t, unbans)
				}
				if tc.wantApprv {
					require.Len(t, botMock.AddApprovedUserCalls(), 1)
				} else {
					assert.Empty(t, botMock.AddApprovedUserCalls())
				}
			})
		}
	})

	t.Run("router does not fall through to legacy unban on malformed DC data", func(t *testing.T) {
		api, botMock, adm := newDCGateAdmin(t)
		require.Error(t, adm.InlineCallbackHandler(context.Background(), dcGateQuery("DCAnotanumber", msgText)))
		assert.Empty(t, botMock.AddApprovedUserCalls())
		assert.Empty(t, api.SendCalls(), "no edit on malformed data")
	})

	t.Run("dry mode skips telegram unban but still approves", func(t *testing.T) {
		api, botMock, adm := newDCGateAdmin(t)
		adm.dry = true
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", msgText)))

		for _, call := range api.RequestCalls() {
			_, isUnban := call.C.(tbapi.UnbanChatMemberConfig)
			assert.False(t, isUnban, "dry mode must not call telegram unban")
		}
		require.Len(t, botMock.AddApprovedUserCalls(), 1)
	})

	t.Run("second confirm does not append resolution twice", func(t *testing.T) {
		api, botMock, adm := newDCGateAdmin(t)
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", msgText)))

		edited := api.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", edited)))

		assert.Len(t, api.SendCalls(), 1, "already-resolved message must not be edited again")
		assert.Len(t, botMock.AddApprovedUserCalls(), 1, "approval must not run twice")

		require.NoError(t, adm.callbackDCUnbanCancel(dcGateQuery("DCX4242", edited)))
		assert.Len(t, api.SendCalls(), 1, "cancel on resolved message must not restore the button")
	})

	t.Run("soft-ban mode still lifts the hard kick", func(t *testing.T) {
		api, _, adm := newDCGateAdmin(t)
		adm.softBan = true
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(), dcGateQuery("DCU4242", msgText)))

		var unbans, restricts int
		for _, call := range api.RequestCalls() {
			switch call.C.(type) {
			case tbapi.UnbanChatMemberConfig:
				unbans++
			case tbapi.RestrictChatMemberConfig:
				restricts++
			}
		}
		assert.Equal(t, 1, unbans, "dc gate bans hard, so the unban must be hard too")
		assert.Zero(t, restricts, "soft-ban restrict-lifting must not be used for dc-gate unbans")
	})

	t.Run("cancel restores unban button", func(t *testing.T) {
		api, _, adm := newDCGateAdmin(t)
		require.NoError(t, adm.callbackDCUnbanCancel(dcGateQuery("DCX4242", msgText)))

		require.Len(t, api.SendCalls(), 1)
		edit := api.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig)
		require.Len(t, edit.ReplyMarkup.InlineKeyboard, 1)
		require.Len(t, edit.ReplyMarkup.InlineKeyboard[0], 1)
		assert.Equal(t, "Разбанить", edit.ReplyMarkup.InlineKeyboard[0][0].Text)
		assert.Equal(t, "DCA4242", *edit.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
	})

	t.Run("message without username approves with empty name", func(t *testing.T) {
		_, botMock, adm := newDCGateAdmin(t)
		require.NoError(t, adm.callbackDCUnbanConfirmed(context.Background(),
			dcGateQuery("DCU7303562081", "[DC GATE] 7303562081 забанен по DC 5 при входе в чат -1001420186506")))

		require.Len(t, botMock.AddApprovedUserCalls(), 1)
		assert.Empty(t, botMock.AddApprovedUserCalls()[0].Name)
	})
}

func TestDCGateUserName(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "plain entity-stripped text",
			text:     "[DC GATE] gpig_stepan (4242) забанен по DC 2 при входе в чат -1001420186506",
			expected: "gpig_stepan",
		},
		{
			name:     "plain-text fallback keeps raw anchor markup",
			text:     `[DC GATE] <a href="tg://user?id=4242">gpig_stepan</a> (4242) забанен по DC 2 при входе в чат -1001420186506`,
			expected: "gpig_stepan",
		},
		{
			name:     "dry marker prefix",
			text:     "[DRY] [DC GATE] gpig_stepan (4242) забанен по DC 2 при входе в чат -1001420186506",
			expected: "gpig_stepan",
		},
		{
			name:     "training marker prefix",
			text:     "[TRAINING] [DC GATE] gpig_stepan (4242) забанен по DC 2 при входе в чат -1001420186506",
			expected: "gpig_stepan",
		},
		{
			name:     "no username leaves no match",
			text:     "[DC GATE] 7303562081 забанен по DC 5 при входе в чат -1001420186506",
			expected: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, dcGateUserName(tc.text))
		})
	}
}

func TestNotifyDCBan_TrainingMarker(t *testing.T) {
	photos := tbapi.UserProfilePhotos{TotalCount: 1, Photos: [][]tbapi.PhotoSize{{{FileID: realAvatarFileID}}}}
	l, _, api, _ := newDCBanTestListener(t, []int{2}, photos)
	l.TrainingMode = true

	u := dcJoinUpdate(123, 42, "u", "left", "member", false)
	l.notifyDCBan(u, u.NewChatMember.User, 2)

	require.Len(t, api.SendCalls(), 1)
	assert.Contains(t, api.SendCalls()[0].C.(tbapi.MessageConfig).Text, "[TRAINING] [DC GATE]")
}

// assertError is a tiny error helper for the spy's applyBan hook.
type assertError string

func (a assertError) Error() string { return string(a) }

func TestIsJoinTransition(t *testing.T) {
	cases := []struct {
		old, newS string
		want      bool
	}{
		{"left", "member", true},
		{"kicked", "member", true},
		{"left", "restricted", true},
		{"kicked", "administrator", true},
		{"left", "creator", true},
		{"member", "administrator", false}, // promotion, not a join
		{"left", "left", false},
		{"kicked", "kicked", false},
		{"member", "left", false}, // leaving
		{"left", "banned", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isJoinTransition(tc.old, tc.newS), "old=%q new=%q", tc.old, tc.newS)
	}
}

func TestProtectedJoinStatus(t *testing.T) {
	assert.True(t, protectedJoinStatus("administrator"))
	assert.True(t, protectedJoinStatus("creator"))
	assert.False(t, protectedJoinStatus("member"))
	assert.False(t, protectedJoinStatus("restricted"))
}

func TestNewDCSet(t *testing.T) {
	s := newDCSet([]int{2, 4, 2})
	assert.True(t, s.has(2))
	assert.True(t, s.has(4))
	assert.False(t, s.has(1))
	assert.False(t, s.has(0))
	assert.False(t, (*dcSet)(nil).has(2), "nil set is safe and has nothing")

	empty := newDCSet(nil)
	assert.False(t, empty.has(2))
}

func TestApplyRuleSet_RebuildsBannedDCs(t *testing.T) {
	l := &TelegramListener{}
	l.bannedDCs.Store(newDCSet(nil))

	l.ApplyRuleSet(rules.RuleSet{JoinGate: rules.JoinGateRules{BannedDCs: []int{3, 5}}})

	require.NotNil(t, l.bannedDCs.Load())
	assert.True(t, l.bannedDCs.Load().has(3))
	assert.True(t, l.bannedDCs.Load().has(5))
	assert.False(t, l.bannedDCs.Load().has(2))
}
