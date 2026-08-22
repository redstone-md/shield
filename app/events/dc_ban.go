package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/lib/fileid"
)

// errNoProfilePhoto is returned by classifyUserDC when the user has no profile
// photo, which means their datacenter cannot be derived from file_id. The join
// gate treats this as a skip (not an error worth alarming on).
var errNoProfilePhoto = errors.New("user has no profile photo")

// dcSet is an immutable set of banned datacenter ids. It is swapped atomically
// on rule-set reload so the event loop can read it without locking.
type dcSet struct{ m map[int]struct{} }

func (s *dcSet) has(dc int) bool {
	if s == nil {
		return false
	}
	_, ok := s.m[dc]
	return ok
}

// newDCSet builds an immutable dcSet from a slice. Duplicate ids collapse.
func newDCSet(dcs []int) *dcSet {
	m := make(map[int]struct{}, len(dcs))
	for _, d := range dcs {
		m[d] = struct{}{}
	}
	return &dcSet{m: m}
}

// procChatMemberUpdated handles chat_member updates: when a user joins a
// monitored chat and their profile-photo datacenter is in the banned set, the
// user is banned preemptively across all primary chats. Pre-existing members are
// not checked (no join event fires for them).
func (l *TelegramListener) procChatMemberUpdated(ctx context.Context, u tbapi.ChatMemberUpdated) error {
	if !l.isChatAllowed(u.Chat.ID) {
		return nil
	}
	if !isJoinTransition(u.OldChatMember.Status, u.NewChatMember.Status) {
		return nil
	}
	member := u.NewChatMember
	if member.User == nil {
		return nil
	}
	user := member.User

	// exemptions: bots, chat admins/creators, super-users, approved users.
	if user.IsBot {
		return nil
	}
	if protectedJoinStatus(member.Status) {
		return nil
	}
	if l.SuperUsers.IsSuper(user.UserName, user.ID) {
		return nil
	}
	if l.Bot != nil && l.Bot.IsApprovedUser(user.ID) {
		log.Printf("[DEBUG] dc-ban: user %d is approved, skipping", user.ID)
		return nil
	}

	set := l.bannedDCs.Load()
	if set == nil || len(set.m) == 0 {
		return nil // gate disabled (empty banned set)
	}

	dc, err := l.classifyUserDC(ctx, user.ID)
	if err != nil {
		if errors.Is(err, errNoProfilePhoto) {
			log.Printf("[DEBUG] dc-ban: user %d has no profile photo, skipping", user.ID)
		} else {
			log.Printf("[WARN] dc-ban: classify user %d failed: %v", user.ID, err)
		}
		return nil
	}
	if !set.has(dc) {
		return nil
	}

	if l.ActionExecutor == nil {
		log.Printf("[WARN] dc-ban: action executor is nil, cannot ban user %d (dc %d)", user.ID, dc)
		return nil
	}

	req := banRequest{
		userID:   user.ID,
		userName: user.UserName,
		duration: bot.PermanentBanDuration,
		dry:      l.Dry,
		training: l.TrainingMode,
	}
	var errs *multierror.Error
	for _, chatID := range l.primChatIDs {
		r := req
		r.chatID = chatID
		if err := l.ActionExecutor.ApplyBan(ctx, r); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("chat %d: %w", chatID, err))
		}
	}
	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("dc-ban user %d (dc %d): %w", user.ID, dc, err)
	}

	l.incMetric("dc_ban_total")
	l.notifyDCBan(u, user, dc)
	return nil
}

// classifyUserDC resolves a user's datacenter, preferring a cached value in the
// locator and otherwise fetching the profile photo and decoding its file_id.
// The decoded DC is cached for rejoin de-duplication.
func (l *TelegramListener) classifyUserDC(ctx context.Context, userID int64) (int, error) {
	if l.Locator != nil {
		if dc, ok := l.Locator.GetUserDC(ctx, userID); ok {
			return dc, nil
		}
	}
	if l.TbAPI == nil {
		return 0, errors.New("telegram api is nil")
	}
	// the fork's GetUserProfilePhotos takes no context, so the call is bounded only
	// by the BotAPI client's HTTP timeout (configured at construction), not a
	// per-call deadline.
	photos, err := l.TbAPI.GetUserProfilePhotos(tbapi.UserProfilePhotosConfig{UserID: userID, Limit: 1})
	if err != nil {
		return 0, fmt.Errorf("get profile photos: %w", err)
	}
	if photos.TotalCount == 0 || len(photos.Photos) == 0 || len(photos.Photos[0]) == 0 {
		return 0, errNoProfilePhoto
	}
	dc, err := fileid.DecodeDC(photos.Photos[0][0].FileID)
	if err != nil {
		return 0, fmt.Errorf("decode file_id: %w", err)
	}
	if l.Locator != nil {
		if err := l.Locator.SetUserDC(ctx, userID, dc); err != nil {
			log.Printf("[WARN] dc-ban: failed to cache dc for user %d: %v", userID, err)
		}
	}
	return dc, nil
}

// notifyDCBan posts a short audit line with a two-step unban button to the
// admin chat, matching the messageless-ban precedent in DirectBanTarget.
func (l *TelegramListener) notifyDCBan(u tbapi.ChatMemberUpdated, user *tbapi.User, dc int) {
	if l.adminChatID == 0 {
		return
	}
	text := fmt.Sprintf("[DC GATE] %s забанен по DC %d при входе в чат %d",
		htmlBanTarget(user.UserName, user.ID), dc, u.Chat.ID)
	if l.Dry {
		text = "[DRY] " + text
	} else if l.TrainingMode {
		text = "[TRAINING] " + text
	}
	msg := tbapi.NewMessage(l.adminChatID, text)
	msg.ReplyMarkup = tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Разбанить", dcUnbanAskPrefix+strconv.FormatInt(user.ID, 10)),
		),
	)
	if err := send(msg, l.TbAPI); err != nil {
		log.Printf("[WARN] dc-ban: failed to notify admin chat: %v", err)
	}
}

// isJoinTransition reports whether a membership update represents a user
// (re)joining a chat: the prior status was left/kicked and the new status is an
// active member state. Promotions within the group (e.g. member -> admin) are
// excluded by requiring a non-member prior status.
func isJoinTransition(oldStatus, newStatus string) bool {
	switch newStatus {
	case "member", "administrator", "creator", "restricted":
	default:
		return false
	}
	return oldStatus == "left" || oldStatus == "kicked"
}

// protectedJoinStatus reports whether the joining user is a chat administrator
// or creator, who cannot be banned and should be skipped before classification.
func protectedJoinStatus(status string) bool {
	return status == "administrator" || status == "creator"
}
