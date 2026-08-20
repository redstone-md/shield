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
