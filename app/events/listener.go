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
	"strings"
	"sync"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/policy"
	"github.com/umputun/tg-spam/app/rules"
)

type UsageMeter interface {
	Increment(ctx context.Context, meterType string) error
}

type MetricsRecorder interface {
	Inc(name string)
	Observe(name string, duration time.Duration)
}

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
	Queue                   moderation.Queue
	ActionExecutor          ActionExecutor
	PolicyEngine            PolicyEngine
	PolicyProfileName       string
	AuditWriter             AuditWriter
	UsageMeter              UsageMeter
	MetricsRecorder         MetricsRecorder
	SlowPathEnabled         bool

	adminHandler    *admin
	reportsHandler  *userReports
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
		FirstStrike:  rs.Moderation.FirstStrike,
		SecondStrike: rs.Moderation.SecondStrike,
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
	log.Printf("[DEBUG] admin handler created, spam forwarding %s, %+v", adminForwardStatus, l.adminHandler)

	if l.AggressiveCleanup {
		log.Printf("[INFO] aggressive cleanup enabled, messages from user will be deleted on ban, limit %d",
			l.AggressiveCleanupLimit)
	}

	return l.eventLoop(ctx)
}

func (l *TelegramListener) initHandlers() {
	l.adminHandler = &admin{
		tbAPI: l.TbAPI, bot: l.Bot, locator: l.Locator, superUsers: l.SuperUsers, actions: l.ActionExecutor,
		primChatID: l.chatID, adminChatID: l.adminChatID,
		trainingMode: l.TrainingMode, softBan: l.SoftBanMode, dry: l.Dry, warnMsg: l.WarnMsg,
		aggressiveCleanup: l.AggressiveCleanup, aggressiveCleanupLimit: l.AggressiveCleanupLimit,
	}

	l.reportsHandler = &userReports{
		ReportConfig: l.ReportConfig, tbAPI: l.TbAPI, bot: l.Bot, locator: l.Locator, superUsers: l.SuperUsers,
		actions:      l.ActionExecutor,
		detectedSpam: l.DetectedSpamCounter, tenantID: l.TenantID, moderation: l.ModerationConfig,
		primChatID: l.chatID, adminChatID: l.adminChatID,
		trainingMode: l.TrainingMode, softBanMode: l.SoftBanMode, dry: l.Dry,
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
		if l.DisableAdminSpamForward {
			return nil
		}
		l.incMetric("admin_messages")
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

	if update.Message.ReplyToMessage != nil && !fromSuper {
		if l.procUserReply(ctx, update) {
			return nil
		}
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

// procSuperReply processes superuser commands (reply) /spam, /ban, /warn
func (l *TelegramListener) procSuperReply(ctx context.Context, update tbapi.Update) (handled bool) {
	switch {
	case strings.EqualFold(update.Message.Text, "/spam") || strings.EqualFold(update.Message.Text, "spam"):
		log.Printf("[DEBUG] superuser %s reported spam", update.Message.From.UserName)
		if err := l.adminHandler.DirectSpamReport(ctx, update); err != nil {
			log.Printf("[WARN] failed to process direct spam report: %v", err)
		}
		return true
	case strings.EqualFold(update.Message.Text, "/ban") || strings.EqualFold(update.Message.Text, "ban"):
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
	}
	return false
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
	for _, field := range strings.Fields(text) {
		trimmed := strings.Trim(field, ".,:;!?()[]{}<>\"'")
		if trimmed == needle {
			return true
		}
	}
	return false
}

// procUserReply processes regular user commands (reply) /report.
func (l *TelegramListener) procUserReply(ctx context.Context, update tbapi.Update) (handled bool) {
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
		if !l.ReportConfig.Enabled {
			log.Printf("[DEBUG] user bot-mention reporting disabled, ignoring mention from %s (%d)",
				update.Message.From.UserName, update.Message.From.ID)
			return true
		}
		log.Printf("[DEBUG] user %s (%d) requested LLM review by bot mention", update.Message.From.UserName, update.Message.From.ID)
		if err := l.reportsHandler.DirectUserReport(ctx, update); err != nil {
			log.Printf("[WARN] failed to process user bot-mention report: %v", err)
		}
		return true
	}
	return false
}
