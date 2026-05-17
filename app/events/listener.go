// Package events provide event handlers for telegram bot and all the high-level event handlers.
// It parses messages, sends them to the spam detector and handles the results. It can also ban users
// and send messages to the admin.
//
// In addition to that, it provides support for admin chat handling allowing to unban users via the web service and
// update the list of spam samples.
package events

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/policy"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/slowpath"
)

type UsageMeter interface {
	Increment(ctx context.Context, meterType string) error
}

type MetricsRecorder interface {
	Inc(name string)
	Observe(name string, duration time.Duration)
}

type SlowPathChecker interface {
	Check(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error)
}

type SlowPathChatChecker interface {
	Reply(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error)
}

var chatThoughtRe = regexp.MustCompile(`(?is)<thought>.*?</thought>`)

// TelegramListener listens to tg update, forward to bots and send back responses
// Not thread safe
type TelegramListener struct {
	TbAPI                   TbAPI         // telegram bot API
	SpamLogger              SpamLogger    // logger to save spam to files and db
	Bot                     Bot           // bot to handle messages
	BotUsername             string        // telegram bot username (without "@" prefix)
	TenantID                string        // storage tenant id
	Group                   string        // can be int64 or public group username (without "@" prefix)
	AdminGroup              string        // can be int64 or public group username (without "@" prefix)
	IdleDuration            time.Duration // idle timeout to send "idle" message to bots
	SuperUsers              SuperUsers    // list of superusers, can ban and report spam, can't be banned
	TestingIDs              []int64       // list of chat IDs to test the bot
	StartupMsg              string        // message to send on startup to the primary chat
	WarnMsg                 string        // message to send on warning
	NoSpamReply             bool          // do not reply on spam messages in the primary chat
	SuppressJoinMessage     bool          // delete join message when kick out user
	DeleteJoinMessages      bool          // delete join messages immediately
	DeleteLeaveMessages     bool          // delete leave messages immediately
	TrainingMode            bool          // do not ban users, just report and train spam detector
	SoftBanMode             bool          // do not ban users, but restrict their actions
	Locator                 Locator       // message locator to get info about messages
	IncomingEvents          IncomingEvents
	ModerationActions       ModerationActions
	DetectedSpamCounter     DetectedSpamCounter
	RuleSetVersion          int
	ModerationConfig        ModerationConfig
	ReportConfig            ReportConfig // user spam reporting configuration
	DisableAdminSpamForward bool         // disable forwarding spam reports to admin chat support
	Dry                     bool         // dry run, do not ban or send messages
	AggressiveCleanup       bool         // delete all messages from user when banned via /spam command
	AggressiveCleanupLimit  int          // max messages to delete in aggressive cleanup mode
	DeleteGuestBots         bool         // auto-delete messages from non-admin, non-whitelisted bot accounts
	BotWhitelist            []string     // allowed bot usernames or numeric user IDs (raw strings from config)
	Queue                   moderation.Queue
	ActionExecutor          ActionExecutor
	PolicyEngine            PolicyEngine
	PolicyProfileName       string
	AuditWriter             AuditWriter
	IncidentCreator         IncidentCreator
	AppealService           appealService
	UsageMeter              UsageMeter
	MetricsRecorder         MetricsRecorder
	SlowPathEnabled         bool
	SlowPathEngine          SlowPathChecker
	SlowPathChatEngine      SlowPathChatChecker
	CandidateGenerator      CandidateGenerator
	AutoLearner             AutoLearner

	adminHandler    *admin
	reportsHandler  *userReports
	appealHandler   *appealHandler
	processor       incomingEventProcessor
	pipeline        listenerPipeline
	dmUsers         dmUsers // recent DM senders, stored in memory for admin UI
	chatID          int64
	adminChatID     int64
	linkedChannelID int64 // channel linked to the discussion group, resolved at startup

	msgs struct {
		once sync.Once
		ch   chan bot.Response
	}
	chatLimiter *chatRateLimiter
}

// GetDMUsers returns the list of recent DM senders
func (l *TelegramListener) GetDMUsers() []DMUser {
	return l.dmUsers.List()
}

// ApplyRuleSet updates the listener's runtime config from a new RuleSet.
// It updates the listener fields and propagates changes to the admin and reports sub-handlers.
func (l *TelegramListener) ApplyRuleSet(rs rules.RuleSet) {
	l.RuleSetVersion = rs.Version
	l.ModerationConfig = ModerationConfig{
		FirstStrike:        rs.Moderation.FirstStrike,
		SecondStrike:       rs.Moderation.SecondStrike,
		WarnStrikes:        rs.Moderation.WarnStrikes,
		WarnDeleteDuration: rs.Moderation.WarnDeleteDuration,
	}
	l.ReportConfig = ReportConfig{
		Storage:          l.ReportConfig.Storage,
		Enabled:          rs.Reports.Enabled,
		Threshold:        rs.Reports.Threshold,
		AutoBanThreshold: rs.Reports.AutoBanThreshold,
		RateLimit:        rs.Reports.RateLimit,
		RatePeriod:       rs.Reports.RatePeriod,
	}
	l.SoftBanMode = rs.Moderation.SoftBan
	l.Dry = rs.Moderation.DryRun

	if l.adminHandler != nil {
		l.adminHandler.softBan = rs.Moderation.SoftBan
		l.adminHandler.dry = rs.Moderation.DryRun
		l.adminHandler.moderation = l.ModerationConfig
		l.adminHandler.warnDeleteDuration = rs.Moderation.WarnDeleteDuration
	}
	if l.reportsHandler != nil {
		l.reportsHandler.ReportConfig = l.ReportConfig
		l.reportsHandler.moderation = l.ModerationConfig
		l.reportsHandler.softBanMode = rs.Moderation.SoftBan
		l.reportsHandler.dry = rs.Moderation.DryRun
	}

	profileName := rs.PolicyProfile
	if profileName == "" {
		profileName = "balanced"
	}
	profile := policy.ResolveProfile(profileName)
	l.PolicyEngine = defaultPolicyEngine{eng: policy.NewEngine(profile)}

	l.SlowPathEnabled = rs.SlowPathEnabled

	log.Printf("[INFO] listener config updated from rule set: version=%d, soft_ban=%v, dry=%v, policy=%s, slow_path=%v",
		rs.Version, rs.Moderation.SoftBan, rs.Moderation.DryRun, profileName, rs.SlowPathEnabled)
}

// Do process all events, blocked call
func (l *TelegramListener) Do(ctx context.Context) error {
	log.Printf("[INFO] start telegram listener for %q", l.Group)

	if l.TrainingMode {
		log.Printf("[WARN] training mode, no bans")
	}

	if l.SoftBanMode {
		log.Printf("[INFO] soft ban mode, no bans but restrictions")
	}

	if l.DeleteGuestBots {
		log.Printf("[INFO] delete-guest-bots enabled, whitelist=%v", l.BotWhitelist)
	} else {
		log.Printf("[DEBUG] delete-guest-bots disabled")
	}

	var getChatErr error
	if l.chatID, getChatErr = l.getChatID(l.Group); getChatErr != nil {
		return fmt.Errorf("failed to get chat ID for group %q: %w", l.Group, getChatErr)
	}
	log.Printf("[INFO] primary chat ID: %d", l.chatID)

	chatInfo, err := l.TbAPI.GetChat(tbapi.ChatInfoConfig{ChatConfig: tbapi.ChatConfig{ChatID: l.chatID}})
	if err != nil {
		log.Printf("[WARN] failed to get chat info for linked channel resolution: %v", err)
	} else if chatInfo.LinkedChatID != 0 {
		l.linkedChannelID = chatInfo.LinkedChatID
		log.Printf("[INFO] linked channel ID: %d", l.linkedChannelID)
	}

	if err := l.updateSupers(); err != nil {
		log.Printf("[WARN] failed to update superusers: %v", err)
	}

	if l.AdminGroup != "" {
		if l.adminChatID, getChatErr = l.getChatID(l.AdminGroup); getChatErr != nil {
			return fmt.Errorf("failed to get chat ID for admin group %q: %w", l.AdminGroup, getChatErr)
		}
		log.Printf("[INFO] admin chat ID: %d", l.adminChatID)
	}

	l.msgs.once.Do(func() {
		l.msgs.ch = make(chan bot.Response, 100)
		if l.IdleDuration == 0 {
			l.IdleDuration = 30 * time.Second
		}
	})

	if l.StartupMsg != "" && !l.TrainingMode && !l.Dry {
		if err := l.sendBotResponse(bot.Response{Send: true, Text: l.StartupMsg}, l.chatID, NotificationSilent); err != nil {
			log.Printf("[WARN] failed to send startup message, %v", err)
		} else {
			log.Printf("[DEBUG] startup message sent")
		}
	}

	l.ensurePipeline()
	l.initHandlers()

	adminForwardStatus := "enabled"
	if l.DisableAdminSpamForward {
		adminForwardStatus = "disabled"
	}
	log.Printf("[DEBUG] admin handler created, spam forwarding %s:\n%s",
		adminForwardStatus, observability.FormatFields(l.adminHandler.logConfig()))

	if l.AggressiveCleanup {
		log.Printf("[INFO] aggressive cleanup enabled, messages from user will be deleted on ban, limit %d",
			l.AggressiveCleanupLimit)
	}

	return l.eventLoop(ctx)
}

func (l *TelegramListener) initHandlers() {
	l.adminHandler = &admin{
		tbAPI: l.TbAPI, bot: l.Bot, locator: l.Locator, superUsers: l.SuperUsers, actions: l.ActionExecutor,
		autoLearner: l.AutoLearner, detectedSpam: l.DetectedSpamCounter,
		mediaSlowPath: l.mediaSlowPathConfig(),
		primChatID:    l.chatID, adminChatID: l.adminChatID,
		trainingMode: l.TrainingMode, softBan: l.SoftBanMode, dry: l.Dry, warnMsg: l.WarnMsg,
		moderation: l.ModerationConfig, warnDeleteDuration: l.ModerationConfig.WarnDeleteDuration,
		aggressiveCleanup: l.AggressiveCleanup, aggressiveCleanupLimit: l.AggressiveCleanupLimit,
	}

	if l.AppealService != nil {
		l.adminHandler.appeals = l.AppealService
	}

	l.reportsHandler = &userReports{
		ReportConfig: l.ReportConfig, tbAPI: l.TbAPI, bot: l.Bot, locator: l.Locator, superUsers: l.SuperUsers,
		actions: l.ActionExecutor, autoLearner: l.AutoLearner,
		detectedSpam: l.DetectedSpamCounter, tenantID: l.TenantID, moderation: l.ModerationConfig,
		primChatID: l.chatID, adminChatID: l.adminChatID,
		trainingMode: l.TrainingMode, softBanMode: l.SoftBanMode, dry: l.Dry,
	}

	if l.AppealService != nil {
		l.appealHandler = newAppealHandler(l.TbAPI, l.AppealService, l.adminChatID)
	}
}

func (l *TelegramListener) eventLoop(ctx context.Context) error {
	u := tbapi.NewUpdate(0)
	u.Timeout = 60

	updates := l.TbAPI.GetUpdatesChan(u)
	log.Printf("[DEBUG] start listening for updates")
	for {
		select {
		case <-ctx.Done():
			l.shutdownPipeline()
			return fmt.Errorf("listener context canceled: %w", ctx.Err())

		case update, ok := <-updates:
			if !ok {
				l.shutdownPipeline()
				return fmt.Errorf("telegram update chan closed")
			}
			if err := l.handleUpdate(ctx, update); err != nil {
				return err
			}

		case <-time.After(l.IdleDuration):
			resp := l.Bot.OnMessage(bot.Message{Text: "idle"}, false)
			if err := l.sendBotResponse(resp, l.chatID, NotificationSilent); err != nil {
				log.Printf("[WARN] failed to respond on idle, %v", err)
			}
		}
	}
}

func (l *TelegramListener) handleUpdate(ctx context.Context, update tbapi.Update) error {
	if update.Message != nil && l.isAdminChat(update.Message.Chat.ID, update.Message.From.UserName, update.Message.From.ID) {
		l.incMetric("admin_messages")
		if update.Message.ReplyToMessage != nil && l.SuperUsers.IsSuper(update.Message.From.UserName, update.Message.From.ID) {
			if l.procSuperReply(ctx, update) {
				return nil
			}
		}
		if err := l.adminHandler.MsgHandler(ctx, update); err != nil {
			l.incMetric("admin_errors")
			log.Printf("[WARN] failed to process admin chat message: %v", err)
			errResp := l.sendBotResponse(bot.Response{Send: true, Text: "error: " + err.Error()}, l.adminChatID, NotificationDefault)
			if errResp != nil {
				log.Printf("[WARN] failed to respond on error, %v", errResp)
			}
		}
		return nil
	}

	if update.CallbackQuery != nil {
		l.handleCallback(ctx, update.CallbackQuery)
		return nil
	}

	if update.EditedMessage != nil {
		l.incMetric("edited_messages")
		log.Printf("[INFO] processing edited message, id: %d", update.EditedMessage.MessageID)
		editedUpdate := tbapi.Update{
			UpdateID:      update.UpdateID,
			Message:       update.EditedMessage,
			EditedMessage: update.EditedMessage,
		}
		if err := l.procEventsWithContext(ctx, editedUpdate); err != nil {
			log.Printf("[WARN] failed to process edited message update: %v", err)
		}
		return nil
	}

	if update.Message == nil {
		return nil
	}

	if update.Message.Chat.Type == "private" && l.procAppealStart(ctx, update) {
		return nil
	}

	if update.Message.NewChatMembers != nil {
		if l.DeleteJoinMessages {
			l.deleteSystemMessage(update.Message.MessageID, update.Message.Chat.ID, "join")
		} else {
			if err := l.procNewChatMemberMessage(ctx, update); err != nil {
				log.Printf("[WARN] failed to process new chat member: %v", err)
			}
		}
		return nil
	}

	if update.Message.LeftChatMember != nil {
		if l.SuppressJoinMessage {
			if err := l.procLeftChatMemberMessage(ctx, update); err != nil {
				log.Printf("[WARN] failed to process left chat member: %v", err)
			}
		}
		if l.DeleteLeaveMessages {
			l.deleteSystemMessage(update.Message.MessageID, update.Message.Chat.ID, "leave")
		}
		return nil
	}

	fromSuper := l.SuperUsers.IsSuper(update.Message.From.UserName, update.Message.From.ID) ||
		l.isLinkedChannel(update.Message)
	if fromSuper && l.procSuperCommand(ctx, update) {
		return nil
	}

	if update.Message.ReplyToMessage != nil && fromSuper {
		if l.procSuperReply(ctx, update) {
			return nil
		}
	}

	if !fromSuper && l.isReportCommand(update.Message.Text) && update.Message.ReplyToMessage == nil {
		log.Printf("[DEBUG] deleting orphaned /report command from %s (%d)", update.Message.From.UserName, update.Message.From.ID)
		_, err := l.TbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
			MessageID: update.Message.MessageID, ChatConfig: tbapi.ChatConfig{ChatID: update.Message.Chat.ID},
		}})
		if err != nil {
			log.Printf("[WARN] failed to delete orphaned /report message %d: %v", update.Message.MessageID, err)
		}
		return nil
	}

	if l.procUserReply(ctx, update) {
		return nil
	}

	if err := l.procEventsWithContext(ctx, update); err != nil {
		log.Printf("[WARN] failed to process update: %v", err)
	}
	return nil
}

func (l *TelegramListener) handleCallback(ctx context.Context, cb *tbapi.CallbackQuery) {
	callbackData := cb.Data
	l.incMetric("callbacks_total")
	if len(callbackData) >= 3 && callbackData[:1] == "R" {
		if err := l.reportsHandler.HandleReportCallback(ctx, cb); err != nil {
			l.incMetric("callback_errors")
			log.Printf("[WARN] failed to process report callback: %v", err)
			errResp := l.sendBotResponse(bot.Response{Send: true, Text: "error: " + err.Error()}, l.adminChatID, NotificationDefault)
			if errResp != nil {
				log.Printf("[WARN] failed to respond on error, %v", errResp)
			}
		}
	} else {
		if err := l.adminHandler.InlineCallbackHandler(ctx, cb); err != nil {
			l.incMetric("callback_errors")
			log.Printf("[WARN] failed to process callback: %v", err)
			errResp := l.sendBotResponse(bot.Response{Send: true, Text: "error: " + err.Error()}, l.adminChatID, NotificationDefault)
			if errResp != nil {
				log.Printf("[WARN] failed to respond on error, %v", errResp)
			}
		}
	}
}

// procAppealStart routes a private-chat "/start <incidentID>" deep link to the
// appeal handler. It returns false for a bare /start or any non-/start text so
// the message continues through normal processing.
func (l *TelegramListener) procAppealStart(ctx context.Context, update tbapi.Update) (handled bool) {
	if l.appealHandler == nil || update.Message == nil {
		return false
	}
	const prefix = "/start "
	text := strings.TrimSpace(update.Message.Text)
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if payload == "" {
		return false
	}
	if err := l.appealHandler.Handle(ctx, update.Message, payload); err != nil {
		log.Printf("[WARN] failed to handle appeal /start: %v", err)
	}
	return true
}

func (l *TelegramListener) procSuperCommand(ctx context.Context, update tbapi.Update) (handled bool) {
	if update.Message == nil || update.Message.ReplyToMessage != nil {
		return false
	}
	cmd, arg := splitCommand(update.Message.Text)
	if !isBanCommand(cmd) || arg == "" {
		return false
	}
	log.Printf("[DEBUG] superuser %s requested ban for %q", update.Message.From.UserName, arg)
	if err := l.adminHandler.DirectBanTarget(ctx, update, arg); err != nil {
		log.Printf("[WARN] failed to process direct ban target: %v", err)
	}
	return true
}

// procSuperReply processes superuser reply commands: /spam, /ban, /warn, /unwarn.
func (l *TelegramListener) procSuperReply(ctx context.Context, update tbapi.Update) (handled bool) {
	switch {
	case strings.EqualFold(update.Message.Text, "/spam") ||
		strings.EqualFold(update.Message.Text, "spam") ||
		l.isReportCommand(update.Message.Text):
		log.Printf("[DEBUG] superuser %s reported spam", update.Message.From.UserName)
		if err := l.adminHandler.DirectSpamReport(ctx, update); err != nil {
			log.Printf("[WARN] failed to process direct spam report: %v", err)
		}
		return true
	case isBanCommand(strings.TrimSpace(update.Message.Text)):
		log.Printf("[DEBUG] superuser %s requested ban", update.Message.From.UserName)
		if err := l.adminHandler.DirectBanReport(ctx, update); err != nil {
			log.Printf("[WARN] failed to process direct ban request: %v", err)
		}
		return true
	case strings.EqualFold(update.Message.Text, "/warn") || strings.EqualFold(update.Message.Text, "warn"):
		log.Printf("[DEBUG] superuser %s requested warning", update.Message.From.UserName)
		if err := l.adminHandler.DirectWarnReport(update); err != nil {
			log.Printf("[WARN] failed to process direct warning request: %v", err)
		}
		return true
	case strings.EqualFold(update.Message.Text, "/del") || strings.EqualFold(update.Message.Text, "del"):
		if !l.SuperUsers.IsSuper(update.Message.From.UserName, update.Message.From.ID) {
			log.Printf("[WARN] non-superuser %s attempted direct delete", update.Message.From.UserName)
			return false
		}
		log.Printf("[DEBUG] superuser %s requested message deletion", update.Message.From.UserName)
		if err := l.adminHandler.DirectDeleteReply(ctx, update); err != nil {
			log.Printf("[WARN] failed to process direct delete request: %v", err)
		}
		return true
	case strings.EqualFold(update.Message.Text, "/unwarn") || strings.EqualFold(update.Message.Text, "unwarn"):
		log.Printf("[DEBUG] superuser %s requested warning removal", update.Message.From.UserName)
		if err := l.adminHandler.DirectUnwarnReport(update); err != nil {
			log.Printf("[WARN] failed to process direct unwarn request: %v", err)
		}
		return true
	}
	return false
}

func splitCommand(text string) (cmd, arg string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", ""
	}
	cmd = fields[0]
	if len(fields) > 1 {
		arg = fields[1]
	}
	return cmd, arg
}

func isBanCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	return strings.EqualFold(cmd, "/ban") || strings.EqualFold(cmd, "ban")
}

// isReportCommand checks if message text is a /report command variant
func (l *TelegramListener) isReportCommand(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "report" || text == "/report" {
		return true
	}
	if strings.HasPrefix(text, "/report@") {
		afterAt := text[8:]
		fields := strings.Fields(afterAt)
		if len(fields) == 0 {
			return false
		}
		username := fields[0]
		if l.BotUsername == "" {
			return false
		}
		return strings.EqualFold(username, l.BotUsername)
	}
	return false
}

func (l *TelegramListener) isBotMention(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" || l.BotUsername == "" {
		return false
	}
	needle := "@" + strings.ToLower(l.BotUsername)
	for field := range strings.FieldsSeq(text) {
		trimmed := strings.Trim(field, ".,:;!?()[]{}<>\"'")
		if trimmed == needle {
			return true
		}
	}
	return false
}

const (
	chatReplyLimitPerMinute = 5
	chatLimitWindow         = time.Minute
	chatLimitWarningText    = "бот может отвечать вам только 5 раз в минуту"
)

type chatRateLimiter struct {
	mu      sync.Mutex
	entries map[int64][]time.Time
}

func (r *chatRateLimiter) allow(userID int64, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries == nil {
		r.entries = make(map[int64][]time.Time)
	}
	cutoff := now.Add(-chatLimitWindow)
	entries := r.entries[userID][:0]
	for _, ts := range r.entries[userID] {
		if ts.After(cutoff) {
			entries = append(entries, ts)
		}
	}
	if len(entries) >= chatReplyLimitPerMinute {
		r.entries[userID] = entries
		return false
	}
	r.entries[userID] = append(entries, now)
	return true
}

// procUserReply processes regular user commands (reply) /report.
func (l *TelegramListener) procUserReply(ctx context.Context, update tbapi.Update) (handled bool) {
	if update.Message == nil {
		return false
	}
	switch {
	case l.isReportCommand(update.Message.Text):
		if !l.ReportConfig.Enabled {
			log.Printf("[DEBUG] user spam reporting disabled, ignoring /report from %s (%d)",
				update.Message.From.UserName, update.Message.From.ID)
			return true
		}
		log.Printf("[DEBUG] user %s (%d) reported spam", update.Message.From.UserName, update.Message.From.ID)
		if err := l.reportsHandler.DirectUserReport(ctx, update); err != nil {
			log.Printf("[WARN] failed to process user spam report: %v", err)
		}
		return true
	case l.isBotMention(update.Message.Text):
		return l.handleChatReply(ctx, update)
	case l.isChatReplyTrigger(update):
		return l.handleChatReply(ctx, update)
	}
	return false
}

func (l *TelegramListener) isChatReplyTrigger(update tbapi.Update) bool {
	if update.Message == nil {
		return false
	}
	if update.Message.ReplyToMessage != nil && l.isReplyToBot(update.Message.ReplyToMessage) {
		return true
	}
	return strings.Contains(strings.ToLower(update.Message.Text), "железяка")
}

// isAllowedBot returns true if a bot user is permitted to post in the primary chat:
// it is our own bot, a chat administrator, or explicitly whitelisted by id/username.
// Used by the guest-bot auto-delete path; unknown bot accounts return false.
func (l *TelegramListener) isAllowedBot(user *tbapi.User) bool {
	if user == nil {
		return true
	}
	if l.BotUsername != "" && strings.EqualFold(user.UserName, l.BotUsername) {
		return true
	}
	if user.ID == 136817688 { // @Channel_Bot — proxy for channel/anonymous-admin posts, handled elsewhere
		return true
	}
	for _, entry := range l.BotWhitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.EqualFold(strings.TrimPrefix(entry, "@"), user.UserName) {
			return true
		}
		if id, err := strconv.ParseInt(entry, 10, 64); err == nil && id == user.ID {
			return true
		}
	}
	// admin status check via Telegram API. errors treat the bot as not allowed so that we delete on the safe side.
	member, err := l.TbAPI.GetChatMember(tbapi.GetChatMemberConfig{
		ChatConfigWithUser: tbapi.ChatConfigWithUser{
			ChatConfig: tbapi.ChatConfig{ChatID: l.chatID},
			UserID:     user.ID,
		},
	})
	if err != nil {
		log.Printf("[WARN] guest-bot check: GetChatMember failed for %d: %v", user.ID, err)
		return false
	}
	return member.Status == "administrator" || member.Status == "creator"
}

// deleteGuestBotMessage removes a single message from a non-admin bot in the primary chat and logs the outcome.
func (l *TelegramListener) deleteGuestBotMessage(ctx context.Context, msg *tbapi.Message) {
	if l.Dry {
		log.Printf("[INFO] dry run: would delete guest bot message %d from %q (%d)",
			msg.MessageID, msg.From.UserName, msg.From.ID)
		return
	}
	_, err := l.TbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msg.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: msg.Chat.ID},
	}})
	if err != nil {
		log.Printf("[WARN] failed to delete guest bot message %d from %q (%d): %v",
			msg.MessageID, msg.From.UserName, msg.From.ID, err)
		return
	}
	log.Printf("[INFO] deleted guest bot message %d from %q (%d) chat=%d",
		msg.MessageID, msg.From.UserName, msg.From.ID, msg.Chat.ID)
	_ = ctx
}

func (l *TelegramListener) isReplyToBot(msg *tbapi.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	if l.BotUsername != "" && strings.EqualFold(msg.From.UserName, l.BotUsername) {
		return true
	}
	return msg.From.IsBot
}

func (l *TelegramListener) handleChatReply(ctx context.Context, update tbapi.Update) bool {
	if update.Message == nil || update.Message.From == nil {
		return false
	}
	if l.SlowPathChatEngine == nil {
		return false
	}
	if l.SuperUsers.IsSuper(update.Message.From.UserName, update.Message.From.ID) {
		l.sendChatReply(ctx, update)
		return true
	}
	if l.chatLimiter == nil {
		l.chatLimiter = &chatRateLimiter{}
	}
	if !l.chatLimiter.allow(update.Message.From.ID, time.Now().UTC()) {
		msg := tbapi.NewMessage(update.Message.Chat.ID, chatLimitWarningText)
		if sent, err := l.TbAPI.Send(msg); err == nil {
			_, _ = l.TbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
				ChatConfig: tbapi.ChatConfig{ChatID: update.Message.Chat.ID},
				MessageID:  sent.MessageID,
			}})
		} else {
			log.Printf("[WARN] failed to send rate-limit warning: %v", err)
		}
		return true
	}
	l.sendChatReply(ctx, update)
	return true
}

func (l *TelegramListener) sendChatReply(ctx context.Context, update tbapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	if l.SlowPathChatEngine == nil {
		log.Printf("[WARN] chat reply requested but slowpath chat engine is not configured")
		return
	}
	req := slowpath.ChatRequest{
		EventID:       fmt.Sprintf("chat-%d-%d", msg.Chat.ID, msg.MessageID),
		CorrelationID: fmt.Sprintf("chat-%d", msg.MessageID),
		TenantID:      l.TenantID,
		Message:       msg.Text,
	}
	if msg.ReplyToMessage != nil {
		req.History = []slowpath.HistoryMessage{{
			UserName: msg.ReplyToMessage.From.UserName,
			Text:     msg.ReplyToMessage.Text,
		}}
	}
	result, err := replyChatWithRetry(ctx, l.SlowPathChatEngine, req, time.Sleep)
	if err != nil {
		log.Printf("[WARN] failed to generate chat reply: %v", err)
		return
	}
	if result == nil {
		return
	}
	text := strings.TrimSpace(stripChatThoughtTags(result.Text))
	if text == "" {
		return
	}
	resp := bot.Response{Send: true, Text: text, ReplyTo: msg.MessageID}
	if err := l.sendBotResponse(resp, msg.Chat.ID, NotificationDefault); err != nil {
		log.Printf("[WARN] failed to send chat reply: %v", err)
	}
}

func stripChatThoughtTags(text string) string {
	return chatThoughtRe.ReplaceAllString(text, "")
}
