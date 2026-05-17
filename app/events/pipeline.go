package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/observability"
)

type incomingEventProcessor interface {
	Process(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error
}

type pendingIncomingEvent struct {
	update tbapi.Update
	result chan error
}

type listenerPipeline struct {
	worker   sync.WaitGroup
	pending  map[string]pendingIncomingEvent
	mu       sync.Mutex
	eventID  atomic.Uint64
	running  bool
	ownQueue bool
}

type listenerEventProcessor struct {
	listener *TelegramListener
}

func (p listenerEventProcessor) Process(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	return p.listener.processQueuedEvent(ctx, event, update)
}

func (l *TelegramListener) procEvents(update tbapi.Update) error {
	return l.procEventsWithContext(context.Background(), update)
}

func (l *TelegramListener) procEventsWithContext(ctx context.Context, update tbapi.Update) error {
	if update.Message == nil {
		return nil
	}

	// intercept private (DM) messages before any other processing.
	// stores the sender info for the admin UI and silently drops the message.
	if update.Message.Chat.Type == "private" {
		if update.Message.From == nil {
			return nil
		}
		from := update.Message.From
		displayName := strings.TrimSpace(from.FirstName + " " + from.LastName)
		l.dmUsers.Add(DMUser{
			UserID:      from.ID,
			UserName:    from.UserName,
			DisplayName: displayName,
			Timestamp:   time.Now(),
		})
		return nil
	}

	fromChat := update.Message.Chat.ID
	if !l.isChatAllowed(fromChat) {
		return nil
	}

	if update.Message.From != nil && update.Message.From.IsBot && update.Message.SenderChat == nil {
		log.Printf("[DEBUG] bot-msg received: from=%q (%d) IsBot=true delete-guest-bots=%t",
			update.Message.From.UserName, update.Message.From.ID, l.DeleteGuestBots)
		if l.DeleteGuestBots && !l.isAllowedBot(update.Message.From) {
			l.deleteGuestBotMessage(ctx, update.Message)
			return nil
		}
	}

	msg := transform(update.Message)
	if strings.TrimSpace(msg.Text) == "" && msg.Image == nil && !msg.WithVideoNote && !msg.WithVideo && !msg.WithForward &&
		!msg.WithSticker && msg.Animation == nil && msg.CustomEmojiID == "" {
		return nil
	}

	event := l.makeIncomingEvent(update, msg)
	return l.enqueueIncomingEvent(ctx, event, update)
}

func (l *TelegramListener) ensurePipeline() {
	l.pipeline.mu.Lock()
	defer l.pipeline.mu.Unlock()
	if l.pipeline.running {
		return
	}
	if l.Queue == nil {
		l.Queue = moderation.NewInMemoryQueue(100)
		l.pipeline.ownQueue = true
	} else {
		l.pipeline.ownQueue = false
	}
	if l.processor == nil {
		l.processor = listenerEventProcessor{listener: l}
	}
	if l.ActionExecutor == nil {
		exec := newTelegramActionExecutor(l.TbAPI, l.Dry, l.TrainingMode, l.SuperUsers, l.ModerationActions)
		l.ActionExecutor = exec
	}
	if l.PolicyEngine == nil {
		l.PolicyEngine = newProfilePolicyEngine(l.PolicyProfileName)
	}
	if l.AuditWriter == nil {
		l.AuditWriter = defaultAuditWriter{spamLogger: l.SpamLogger, locator: l.Locator}
	}
	l.pipeline.pending = make(map[string]pendingIncomingEvent)
	l.pipeline.running = true
	l.pipeline.worker.Add(1)
	go l.runQueueWorker()
}

func (l *TelegramListener) enqueueIncomingEvent(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	ctx = observability.WithModerationMetadata(ctx, event.EventID, event.CorrelationID, event.IdempotencyKey)
	if l.IncomingEvents != nil {
		replay, err := l.IncomingEvents.Reserve(ctx, event)
		if err != nil {
			return fmt.Errorf("reserve incoming event %s: %w", event.EventID, err)
		}
		if replay.Processed {
			observability.Logf(ctx, "[INFO] replay moderation decision for key %s action=%s applied=%t",
				event.IdempotencyKey, replay.Decision.Action, replay.ActionResult.Applied)
			return nil
		}
		if !replay.Recorded {
			observability.Logf(ctx, "[INFO] duplicate incoming event already in progress for key %s", event.IdempotencyKey)
			return nil
		}
	}

	l.ensurePipeline()

	resultCh := make(chan error, 1)
	l.pipeline.mu.Lock()
	l.pipeline.pending[event.EventID] = pendingIncomingEvent{update: update, result: resultCh}
	l.pipeline.mu.Unlock()

	if err := l.Queue.Publish(ctx, event); err != nil {
		l.pipeline.mu.Lock()
		delete(l.pipeline.pending, event.EventID)
		l.pipeline.mu.Unlock()
		return fmt.Errorf("publish incoming event %s: %w", event.EventID, err)
	}

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *TelegramListener) runQueueWorker() {
	defer l.pipeline.worker.Done()

	for event := range l.Queue.Consume() {
		l.pipeline.mu.Lock()
		pending, ok := l.pipeline.pending[event.EventID]
		if ok {
			delete(l.pipeline.pending, event.EventID)
		}
		l.pipeline.mu.Unlock()
		if !ok {
			log.Printf("[WARN] dropped moderation event %s without pending telegram update", event.EventID)
			continue
		}

		err := l.processor.Process(context.Background(), event, pending.update)
		pending.result <- err
		close(pending.result)
	}
}

func (l *TelegramListener) shutdownPipeline() {
	l.pipeline.mu.Lock()
	if !l.pipeline.running || l.Queue == nil {
		l.pipeline.mu.Unlock()
		return
	}
	queue := l.Queue
	ownQueue := l.pipeline.ownQueue
	l.pipeline.running = false
	l.pipeline.pending = nil
	l.pipeline.mu.Unlock()

	queue.Close()
	l.pipeline.worker.Wait()
	if ownQueue {
		l.Queue = nil
	}
}

type pipelineContext struct {
	event       moderation.IncomingEvent
	msg         *bot.Message
	resp        bot.Response
	fromChat    int64
	spamUserID  int64
	banUserStr  string
	outcome     PolicyOutcome
	strikeCount int
	incidentID  int64
}

func (l *TelegramListener) processQueuedEvent(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	ctx = observability.WithModerationMetadata(ctx, event.EventID, event.CorrelationID, event.IdempotencyKey)

	msgJSON, errJSON := json.Marshal(update.Message)
	if errJSON != nil {
		return fmt.Errorf("failed to marshal update.Message to json: %w", errJSON)
	}

	fromChat := update.Message.Chat.ID
	observability.Logf(ctx, "[DEBUG] %s", string(msgJSON))
	msg := transform(update.Message)

	observability.Logf(ctx, "[DEBUG] incoming msg: %+v", strings.ReplaceAll(msg.Text, "\n", " "))
	observability.Logf(ctx, "[DEBUG] incoming msg details: %+v", msg)

	l.locateMessage(ctx, msg, fromChat)

	if msg.SenderChat.ID != 0 && (msg.SenderChat.ID == fromChat || msg.SenderChat.ID == l.linkedChannelID) {
		observability.Logf(ctx, "[DEBUG] skipping spam check for anonymous admin post from group itself or linked channel")
		return nil
	}

	checkStart := time.Now()
	resp := l.botOnMessage(ctx, *msg, false)
	l.observeLatency("fast_path_latency", time.Since(checkStart))
	l.meter(ctx, "spam_checks")
	if resp.Send && resp.BanInterval > 0 {
		l.meter(ctx, "spam_detected")
	}

	resp = applyMediaSlowPath(ctx, l.mediaSlowPathConfig(), event, msg, resp)

	spamUserID := msg.From.ID
	if msg.SenderChat.ID != 0 {
		spamUserID = msg.SenderChat.ID
	}

	strikeCount := l.getStrikeCount(ctx, spamUserID)
	outcome, policyErr := l.PolicyEngine.Decide(ctx, PolicyRequest{
		Event:         event,
		Response:      resp,
		Message:       msg,
		SpamUserID:    spamUserID,
		StrikeCount:   strikeCount,
		UseEscalation: spamUserID != 0 && l.DetectedSpamCounter != nil,
		SoftBanMode:   l.SoftBanMode,
		Moderation:    l.ModerationConfig,
		IsSuperUser:   l.SuperUsers.IsSuper(msg.From.Username, msg.From.ID),
	})
	if policyErr != nil {
		return fmt.Errorf("policy decision failed: %w", policyErr)
	}
	l.meter(ctx, "policy_"+string(outcome.Decision.Action))

	pc := pipelineContext{
		event: event, msg: msg, resp: resp, fromChat: fromChat,
		spamUserID: spamUserID, banUserStr: l.getBanUsername(resp, update), outcome: outcome,
		strikeCount: strikeCount,
	}

	switch outcome.Decision.Action {
	case moderation.ActionAllow:
		return l.processAllow(ctx, pc)
	case moderation.ActionWarn:
		return l.processWarn(ctx, pc)
	case moderation.ActionDelete:
		return l.processEnforce(ctx, pc)
	case moderation.ActionRestrict, moderation.ActionBan:
		return l.processEnforce(ctx, pc)
	default:
		return l.processAllow(ctx, pc)
	}
}

func (l *TelegramListener) locateMessage(ctx context.Context, msg *bot.Message, fromChat int64) {
	locatorUserID := msg.From.ID
	locatorUserName := msg.From.Username
	if msg.SenderChat.ID != 0 {
		locatorUserID = msg.SenderChat.ID
		locatorUserName = msg.SenderChat.UserName
	}
	if err := l.Locator.AddMessage(ctx, msg.Text, fromChat, locatorUserID, locatorUserName, msg.ID); err != nil {
		observability.Logf(ctx, "[WARN] failed to add message to locator: %v", err)
	}
}

func (l *TelegramListener) getStrikeCount(ctx context.Context, spamUserID int64) int {
	if spamUserID == 0 || l.DetectedSpamCounter == nil {
		return 0
	}
	count, err := l.DetectedSpamCounter.CountByUserID(ctx, spamUserID)
	if err != nil {
		observability.Logf(ctx, "[WARN] failed to count spam strikes for user %d: %v", spamUserID, err)
		return 0
	}
	return count
}

func (l *TelegramListener) processAllow(ctx context.Context, pc pipelineContext) error {
	errs := new(multierror.Error)
	if pc.resp.Send && pc.resp.BanInterval > 0 {
		actionResult := l.makeActionResult(pc.event, pc.outcome.Decision.Action, false)
		if err := l.AuditWriter.Write(ctx, AuditRecord{
			Event: pc.event, Message: pc.msg, Response: pc.resp,
			Decision: pc.outcome.Decision, ActionResult: actionResult,
			RuleSetVersion: l.RuleSetVersion, ChatID: pc.fromChat, SpamUserID: pc.spamUserID,
		}); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("audit write failed: %w", err))
		}
		if err := l.completeIncomingEvent(ctx, pc.event, pc.outcome.Decision, actionResult); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	if pc.resp.Send && l.SuperUsers.IsSuper(pc.msg.From.Username, pc.msg.From.ID) && l.TrainingMode && pc.resp.BanInterval > 0 {
		l.adminHandler.ReportBan(pc.banUserStr, pc.msg, pc.resp.BanInterval, l.SoftBanMode)
	}
	if !pc.resp.Send || pc.resp.BanInterval <= 0 {
		actionResult := l.makeActionResult(pc.event, pc.outcome.Decision.Action, false)
		if err := l.completeIncomingEvent(ctx, pc.event, pc.outcome.Decision, actionResult); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("processing events failed: %w", err)
	}
	return nil
}

func (l *TelegramListener) processEnforce(ctx context.Context, pc pipelineContext) error {
	errs := new(multierror.Error)

	if !pc.resp.Send {
		return l.cleanupAfterAction(ctx, pc, errs)
	}

	observability.Logf(ctx, "[DEBUG] policy action=%s reason=%q score=%.2f",
		pc.outcome.Decision.Action, pc.outcome.Decision.Reason, pc.outcome.Decision.Score)

	actionResult := l.makeActionResult(pc.event, pc.outcome.Decision.Action,
		pc.outcome.Decision.Action == moderation.ActionDelete)

	switch pc.outcome.Decision.Action {
	case moderation.ActionRestrict, moderation.ActionBan:
		l.applyBanAction(ctx, pc, &actionResult, errs)
	case moderation.ActionDelete:
		l.applyDeleteAction(ctx, pc, &actionResult, errs)
	}

	if actionResult.Error == "" {
		if err := l.AuditWriter.Write(ctx, AuditRecord{
			Event: pc.event, Message: pc.msg, Response: pc.resp,
			Decision: pc.outcome.Decision, ActionResult: actionResult,
			RuleSetVersion: l.RuleSetVersion, ChatID: pc.fromChat, SpamUserID: pc.spamUserID,
		}); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("audit write failed: %w", err))
		}
	}

	if l.CandidateGenerator != nil && pc.msg.Text != "" {
		l.CandidateGenerator.GenerateCandidates(ctx, pc.msg.Text)
	}

	if err := l.completeIncomingEvent(ctx, pc.event, pc.outcome.Decision, actionResult); err != nil {
		errs = multierror.Append(errs, err)
	}

	return l.cleanupAfterAction(ctx, pc, errs)
}

// ensureIncident creates the audit incident for a warn/ban before the group
// message is posted, so the incident id can be embedded in the appeal button.
// It is idempotent: the later AuditWriter.Write call resolves to the same
// incident. Returns 0 when incidents are disabled or the response is not
// actionable, in which case no appeal button is attached.
func (l *TelegramListener) ensureIncident(ctx context.Context, pc pipelineContext) int64 {
	if l.IncidentCreator == nil || !pc.resp.Send || pc.resp.BanInterval <= 0 {
		return 0
	}
	userName := ""
	if pc.msg != nil && pc.msg.From.ID != 0 {
		userName = pc.msg.From.DisplayName
	}
	msgText := ""
	if pc.msg != nil {
		msgText = pc.msg.Text
	}
	id, err := l.IncidentCreator.CreateIncident(ctx, pc.event.IdempotencyKey, pc.fromChat,
		l.RuleSetVersion, pc.spamUserID, userName, msgText, pc.resp.CheckResults, nil)
	if err != nil {
		observability.Logf(ctx, "[WARN] early incident creation failed: %v", err)
		return 0
	}
	return id
}

func (l *TelegramListener) processWarn(ctx context.Context, pc pipelineContext) error {
	errs := new(multierror.Error)

	if !pc.resp.Send {
		return l.cleanupAfterAction(ctx, pc, errs)
	}

	pc.incidentID = l.ensureIncident(ctx, pc)
	warnNum := pc.strikeCount + 1
	warnTotal := l.ModerationConfig.WarnStrikes
	if warnTotal <= 0 {
		warnTotal = 3
	}

	warnText := buildWarningText(warnNum, warnTotal, pc.msg.From, pc.spamUserID, l.WarnMsg, slowpathReason(pc.resp.CheckResults))

	actionResult := l.makeActionResult(pc.event, moderation.ActionWarn, false)

	warnReq := warnRequest{
		chatID:      pc.fromChat,
		subjectID:   pc.spamUserID,
		messageID:   pc.msg.ID,
		text:        warnText,
		warnDelTime: l.ModerationConfig.WarnDeleteDuration,
		incidentID:  pc.incidentID,
		botUsername: l.BotUsername,
	}
	if err := l.ActionExecutor.WarnUser(ctx, warnReq); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to send warning: %w", err))
		actionResult.Error = err.Error()
	}

	if l.adminHandler != nil {
		l.adminHandler.ReportWarn(pc.banUserStr, pc.msg, warnNum, warnTotal, slowpathReason(pc.resp.CheckResults))
	}

	l.forwardImageToAdminBeforeDelete(ctx, pc)
	if err := l.ActionExecutor.DeleteMessage(ctx, pc.fromChat, pc.msg.ID); err != nil {
		observability.Logf(ctx, "[WARN] failed to delete spam message %d in warn mode: %v", pc.msg.ID, err)
	}

	if actionResult.Error == "" {
		actionResult.Applied = true
		if err := l.AuditWriter.Write(ctx, AuditRecord{
			Event: pc.event, Message: pc.msg, Response: pc.resp,
			Decision: pc.outcome.Decision, ActionResult: actionResult,
			RuleSetVersion: l.RuleSetVersion, ChatID: pc.fromChat, SpamUserID: pc.spamUserID,
		}); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("audit write failed: %w", err))
		}
	}

	if l.CandidateGenerator != nil && pc.msg.Text != "" {
		l.CandidateGenerator.GenerateCandidates(ctx, pc.msg.Text)
	}

	if err := l.completeIncomingEvent(ctx, pc.event, pc.outcome.Decision, actionResult); err != nil {
		errs = multierror.Append(errs, err)
	}

	return l.cleanupAfterAction(ctx, pc, errs)
}

const (
	banGroupMessageText      = "🚫 Пользователь забанен за спам"
	restrictGroupMessageText = "🔇 Пользователь ограничен за спам"
)

// postBanGroupMessage posts the self-deleting ban or restriction notice with
// the appeal button to the group chat. It is skipped for channel bans and
// dry/training runs where no real enforcement happened.
func (l *TelegramListener) postBanGroupMessage(ctx context.Context, pc pipelineContext, incidentID int64) {
	if l.Dry || l.TrainingMode || pc.resp.ChannelID != 0 {
		return
	}
	text := banGroupMessageText
	if pc.outcome.Restrict {
		text = restrictGroupMessageText
	}
	if err := l.ActionExecutor.PostBanMessage(ctx, banMessageRequest{
		chatID:      pc.fromChat,
		text:        text,
		incidentID:  incidentID,
		botUsername: l.BotUsername,
		delTime:     l.ModerationConfig.WarnDeleteDuration,
	}); err != nil {
		observability.Logf(ctx, "[WARN] failed to post ban group message: %v", err)
	}
}

func (l *TelegramListener) applyBanAction(ctx context.Context, pc pipelineContext,
	actionResult *moderation.ModerationActionResult, errs *multierror.Error) {
	incidentID := l.ensureIncident(ctx, pc)
	banReq := banRequest{
		duration:  pc.outcome.Duration,
		userID:    pc.resp.User.ID,
		channelID: pc.resp.ChannelID,
		userName:  pc.banUserStr,
		chatID:    pc.fromChat,
		dry:       l.Dry,
		training:  l.TrainingMode,
		restrict:  pc.outcome.Restrict,
	}
	banStart := time.Now()
	if err := l.ActionExecutor.ApplyBan(ctx, banReq); err != nil {
		l.observeLatency("ban_latency", time.Since(banStart))
		l.incMetric("ban_errors")
		actionResult.Applied = false
		actionResult.Error = err.Error()
		multierror.Append(errs, fmt.Errorf("failed to apply %s for %s: %w",
			pc.outcome.Decision.Action, pc.banUserStr, err))
		return
	}
	actionResult.Applied = true
	if l.adminChatID != 0 && pc.msg.From.ID != 0 {
		l.adminHandler.ReportBan(pc.banUserStr, pc.msg, pc.outcome.Duration, pc.outcome.Restrict,
			slowpathReason(pc.resp.CheckResults))
	}
	l.postBanGroupMessage(ctx, pc, incidentID)
}

func (l *TelegramListener) applyDeleteAction(ctx context.Context, pc pipelineContext,
	actionResult *moderation.ModerationActionResult, errs *multierror.Error) {
	if l.Dry {
		observability.Logf(ctx, "[INFO] dry run: delete message %d", pc.msg.ID)
		actionResult.Applied = true
		if l.adminChatID != 0 && pc.msg.From.ID != 0 {
			l.adminHandler.ReportBan(pc.banUserStr, pc.msg, 0, false, slowpathReason(pc.resp.CheckResults))
		}
		return
	}
	if l.TrainingMode {
		observability.Logf(ctx, "[INFO] training mode: delete message %d", pc.msg.ID)
		actionResult.Applied = true
		return
	}
	l.forwardImageToAdminBeforeDelete(ctx, pc)
	if err := l.ActionExecutor.DeleteMessage(ctx, pc.fromChat, pc.msg.ID); err != nil {
		actionResult.Applied = false
		actionResult.Error = err.Error()
		multierror.Append(errs, fmt.Errorf("failed to delete message %d: %w", pc.msg.ID, err))
		return
	}
	actionResult.Applied = true
	if l.adminChatID != 0 && pc.msg.From.ID != 0 {
		l.adminHandler.ReportBan(pc.banUserStr, pc.msg, 0, false, slowpathReason(pc.resp.CheckResults))
	}
}

func (l *TelegramListener) forwardImageToAdminBeforeDelete(ctx context.Context, pc pipelineContext) {
	if pc.msg.Image == nil || l.adminChatID == 0 || l.ActionExecutor == nil {
		return
	}
	if err := l.ActionExecutor.ForwardMessage(ctx, pc.fromChat, l.adminChatID, pc.msg.ID); err != nil {
		observability.Logf(ctx, "[WARN] failed to forward image message %d before deletion: %v", pc.msg.ID, err)
	}
}

func (l *TelegramListener) cleanupAfterAction(ctx context.Context, pc pipelineContext, errs *multierror.Error) error {
	if err := l.ActionExecutor.DeleteExtraMessages(ctx, pc.resp.CheckResults,
		pc.msg.From.ID, pc.msg.From.Username, pc.fromChat); err != nil {
		errs = multierror.Append(errs, err)
	}

	canDelete := pc.resp.DeleteReplyTo && pc.resp.ReplyTo != 0 && !l.Dry &&
		!l.SuperUsers.IsSuper(pc.msg.From.Username, pc.msg.From.ID) && !l.TrainingMode
	if canDelete {
		if err := l.ActionExecutor.DeleteMessage(ctx, l.chatID, pc.resp.ReplyTo); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("processing events failed: %w", err)
	}
	return nil
}

func (l *TelegramListener) makeActionResult(event moderation.IncomingEvent,
	action moderation.Action, applied bool) moderation.ModerationActionResult {
	return moderation.ModerationActionResult{
		EventID:       event.EventID,
		CorrelationID: event.CorrelationID,
		Action:        action,
		Applied:       applied,
		Provider:      "telegram",
		AppliedAt:     time.Now().UTC(),
	}
}

func (l *TelegramListener) meter(ctx context.Context, meterType string) {
	if l.UsageMeter == nil {
		return
	}
	if err := l.UsageMeter.Increment(ctx, meterType); err != nil {
		observability.Logf(ctx, "[WARN] usage meter increment failed: %v", err)
	}
}

func (l *TelegramListener) observeLatency(name string, d time.Duration) {
	if l.MetricsRecorder == nil {
		return
	}
	l.MetricsRecorder.Observe(name, d)
}

func (l *TelegramListener) incMetric(name string) {
	if l.MetricsRecorder == nil {
		return
	}
	l.MetricsRecorder.Inc(name)
}

func stickerDownloadFileID(s *bot.StickerInfo) string {
	if s.ThumbFileID != "" {
		return s.ThumbFileID
	}
	if s.FileID != "" {
		return s.FileID
	}
	return s.ThumbFileID
}
